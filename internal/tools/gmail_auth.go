package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	gmailScope        = "https://www.googleapis.com/auth/gmail.modify"
	gmailAPIBase      = "https://gmail.googleapis.com/gmail/v1/users/me"
	gmailCallbackPort = 9877
	gmailAPISetupURL  = "https://console.cloud.google.com/apis/library/gmail.googleapis.com"
	gmailCredsSetupURL = "https://console.cloud.google.com/apis/credentials"
)

// ---------------------------------------------------------------------------
// Background OAuth callback server — started once, accepts one code.
// ---------------------------------------------------------------------------

var (
	errGmailNotConfigured = errors.New("gmail not configured")
	errGmailNotAuthorized = errors.New("gmail not authorized")

	callbackMu      sync.Mutex
	callbackRunning bool
	callbackTokenCh chan *oauth2.Token // non-nil while server is waiting
)

// startCallbackServer starts a one-shot HTTP server on gmailCallbackPort.
// It exchanges the code for a token using cfg and sends it on the returned channel.
// The server shuts itself down after receiving one successful callback or after timeout.
func startCallbackServer(cfg *oauth2.Config, timeout time.Duration) (<-chan *oauth2.Token, error) {
	callbackMu.Lock()
	defer callbackMu.Unlock()

	if callbackRunning {
		return nil, fmt.Errorf("an authorization flow is already in progress; complete it in your browser first")
	}

	tokenCh := make(chan *oauth2.Token, 1)
	callbackTokenCh = tokenCh

	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:         fmt.Sprintf("localhost:%d", gmailCallbackPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		authErr := r.URL.Query().Get("error")
		if authErr != "" {
			http.Error(w, "Authorization denied: "+authErr, http.StatusBadRequest)
			return
		}
		if code == "" {
			http.Error(w, "Missing code", http.StatusBadRequest)
			return
		}
		tok, err := cfg.Exchange(context.Background(), code)
		if err != nil {
			http.Error(w, "Token exchange failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body style='font-family:sans-serif;padding:40px'><h2>✓ ¡Gmail autorizado!</h2><p>Puedes cerrar esta pestaña y volver a Sam.</p></body></html>")
		tokenCh <- tok
		// Shut down server after a short delay to let the response flush.
		go func() {
			time.Sleep(500 * time.Millisecond)
			srv.Shutdown(context.Background()) //nolint:errcheck
		}()
	})

	ln, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", gmailCallbackPort))
	if err != nil {
		callbackTokenCh = nil
		return nil, fmt.Errorf("no se pudo abrir el puerto %d: %w", gmailCallbackPort, err)
	}

	callbackRunning = true
	go func() {
		srv.Serve(ln) //nolint:errcheck
		callbackMu.Lock()
		callbackRunning = false
		callbackTokenCh = nil
		callbackMu.Unlock()
	}()

	// Auto-shutdown after timeout.
	go func() {
		time.Sleep(timeout)
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()

	return tokenCh, nil
}

// GmailCredentials holds the OAuth2 client ID and secret.
type GmailCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func gmailCredsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".openclaw", "state", "credentials", "gmail-credentials.json"), nil
}

func gmailTokenPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".openclaw", "state", "credentials", "gmail-token.json"), nil
}

func loadGmailCredentials() (*GmailCredentials, error) {
	p, err := gmailCredsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w — pídele a Sam que te conecte a Gmail", errGmailNotConfigured)
		}
		return nil, err
	}
	var creds GmailCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("gmail credentials file is malformed: %w", err)
	}
	return &creds, nil
}

func saveGmailCredentials(creds GmailCredentials) error {
	p, err := gmailCredsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func loadGmailToken() (*oauth2.Token, error) {
	p, err := gmailTokenPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w — pídele a Sam que te conecte a Gmail primero", errGmailNotAuthorized)
		}
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("gmail token file is malformed: %w", err)
	}
	return &tok, nil
}

func saveGmailToken(tok *oauth2.Token) error {
	p, err := gmailTokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func gmailOAuth2Config(creds *GmailCredentials) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Scopes:       []string{gmailScope},
		Endpoint:     google.Endpoint,
		RedirectURL:  fmt.Sprintf("http://localhost:%d/callback", gmailCallbackPort),
	}
}

type connectGmailParams struct {
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret"`
	CredentialsJSON string `json:"credentials_json"`
	CredentialsPath string `json:"credentials_path"`
}

func parseGmailCredentialsJSON(raw string) (*GmailCredentials, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("credentials_json no puede estar vacío")
	}

	var payload struct {
		Installed *GmailCredentials `json:"installed"`
		Web       *GmailCredentials `json:"web"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("credentials_json no es un JSON válido: %w", err)
	}

	switch {
	case payload.Installed != nil && payload.Installed.ClientID != "" && payload.Installed.ClientSecret != "":
		creds := *payload.Installed
		return &creds, nil
	case payload.Web != nil && payload.Web.ClientID != "" && payload.Web.ClientSecret != "":
		creds := *payload.Web
		return &creds, nil
	default:
		return nil, fmt.Errorf("credentials_json no contiene client_id y client_secret de Google")
	}
}

func resolveGmailCredentialsPath(rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", fmt.Errorf("credentials_path no puede estar vacío")
	}

	if rawPath == "~" || strings.HasPrefix(rawPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("no pude resolver ~ en credentials_path: %w", err)
		}
		if rawPath == "~" {
			rawPath = home
		} else {
			rawPath = filepath.Join(home, strings.TrimPrefix(rawPath, "~/"))
		}
	}

	if filepath.IsAbs(rawPath) {
		return filepath.Clean(rawPath), nil
	}

	absPath, err := filepath.Abs(rawPath)
	if err != nil {
		return "", fmt.Errorf("no pude resolver credentials_path: %w", err)
	}
	return filepath.Clean(absPath), nil
}

func loadGmailCredentialsFromPath(rawPath string) (*GmailCredentials, string, error) {
	resolvedPath, err := resolveGmailCredentialsPath(rawPath)
	if err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, "", fmt.Errorf("no pude leer credentials_path: %w", err)
	}

	creds, err := parseGmailCredentialsJSON(string(data))
	if err != nil {
		return nil, "", fmt.Errorf("credentials_path no contiene un OAuth client JSON válido: %w", err)
	}
	return creds, resolvedPath, nil
}

func resolveConnectGmailCredentials(params connectGmailParams) (*GmailCredentials, string, error) {
	hasPair := strings.TrimSpace(params.ClientID) != "" || strings.TrimSpace(params.ClientSecret) != ""
	hasJSON := strings.TrimSpace(params.CredentialsJSON) != ""
	hasPath := strings.TrimSpace(params.CredentialsPath) != ""

	explicitSources := 0
	if hasPair {
		explicitSources++
	}
	if hasJSON {
		explicitSources++
	}
	if hasPath {
		explicitSources++
	}

	if explicitSources > 1 {
		return nil, "", fmt.Errorf("usa sólo una fuente de credenciales: client_id/client_secret, credentials_json o credentials_path")
	}

	if hasPair {
		if strings.TrimSpace(params.ClientID) == "" || strings.TrimSpace(params.ClientSecret) == "" {
			return nil, "", fmt.Errorf("si envías credenciales manuales, debes enviar client_id y client_secret juntos")
		}
		return &GmailCredentials{
			ClientID:     strings.TrimSpace(params.ClientID),
			ClientSecret: strings.TrimSpace(params.ClientSecret),
		}, "inline_client_pair", nil
	}

	if hasJSON {
		creds, err := parseGmailCredentialsJSON(params.CredentialsJSON)
		if err != nil {
			return nil, "", err
		}
		return creds, "inline_credentials_json", nil
	}

	if hasPath {
		creds, resolvedPath, err := loadGmailCredentialsFromPath(params.CredentialsPath)
		if err != nil {
			return nil, "", err
		}
		return creds, resolvedPath, nil
	}

	creds, err := loadGmailCredentials()
	if err != nil {
		return nil, "", err
	}
	return creds, "saved_credentials", nil
}

func tryOpenBrowserURLs(rawURLs []string) ([]string, string) {
	openedURLs := make([]string, 0, len(rawURLs))
	browserCommand := ""

	for _, rawURL := range rawURLs {
		opened, command := tryOpenBrowserURL(rawURL)
		if browserCommand == "" && command != "" {
			browserCommand = command
		}
		if opened {
			openedURLs = append(openedURLs, rawURL)
		}
	}

	return openedURLs, browserCommand
}

func gmailNeedsCredentialsResult() Result {
	setupURLs := []string{gmailAPISetupURL, gmailCredsSetupURL}
	openedURLs, browserCommand := tryOpenBrowserURLs(setupURLs)
	message := "Necesito crear la credencial OAuth de Google una sola vez para terminar de conectar Gmail. Si pude, ya abrí la consola correcta."
	if len(openedURLs) == 0 {
		message = "Necesito crear la credencial OAuth de Google una sola vez para terminar de conectar Gmail. Abre las setup_urls y luego vuelve a pedirme conectar Gmail."
	}

	return JSONResult(map[string]any{
		"ok":              false,
		"status":          "needs_credentials",
		"message":         message,
		"setup_urls":      setupURLs,
		"opened_urls":     openedURLs,
		"browser_opened":  len(openedURLs) > 0,
		"browser_command": browserCommand,
		"accepted_inputs": []string{"client_id + client_secret", "credentials_json", "credentials_path"},
		"instructions": []string{
			"1. En Google Cloud Console habilita Gmail API si todavía no está habilitada",
			"2. Crea un OAuth Client ID tipo Desktop app y descarga el JSON",
			"3. Vuelve a pedirme conectar Gmail pasándome credentials_json, credentials_path o client_id y client_secret",
			"4. En ese mismo paso Sam guardará las credenciales, abrirá Google OAuth y dejará Gmail conectado",
		},
	})
}

func startGmailAuthorizationFlow(creds *GmailCredentials, credentialsSource string) Result {
	if err := saveGmailCredentials(*creds); err != nil {
		return ErrorResult(fmt.Sprintf("error guardando credenciales: %s", err))
	}

	cfg := gmailOAuth2Config(creds)
	tokenCh, err := startCallbackServer(cfg, 10*time.Minute)
	if err != nil {
		return ErrorResult(err.Error())
	}

	authURL := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	// Write URL to file so it's accessible even if xdg-open fails.
	_ = os.WriteFile("/tmp/gmail-auth-url.txt", []byte(authURL), 0o644)

	browserOpened, browserCommand := tryOpenBrowserURL(authURL)

	// Wait in background for the callback and save the token.
	go func() {
		select {
		case tok := <-tokenCh:
			_ = saveGmailToken(tok)
		case <-time.After(10 * time.Minute):
			// Timed out — server already shut down.
		}
	}()

	message := "Abre esta URL en tu navegador local para autorizar Gmail. El servidor de callback está listo en el puerto 9877. Una vez que autorices, las herramientas de Gmail estarán disponibles."
	instructions := []string{
		"1. Abre la auth_url y acepta los permisos de Google",
		"2. Google te redirigirá a localhost:9877/callback",
		"3. Sam guardará el token automáticamente y Gmail quedará conectado",
	}
	if browserOpened {
		message = "Intenté abrir automáticamente la página de autorización de Gmail. Si no ves el navegador, abre la auth_url manualmente."
		instructions = []string{
			"1. Si el navegador ya abrió, acepta los permisos de Google",
			"2. Si no abrió, copia la auth_url y ábrela manualmente",
			"3. Google te redirigirá a localhost:9877/callback y Sam guardará el token",
		}
	}

	return JSONResult(map[string]any{
		"ok":                 true,
		"status":             "awaiting_authorization",
		"auth_url":           authURL,
		"browser_opened":     browserOpened,
		"browser_command":    browserCommand,
		"credentials_source": credentialsSource,
		"message":            message,
		"instructions":       instructions,
		"tunnel_hint":        "Si vas a abrir la URL desde tu Mac mientras Sam corre en la VM, primero corre: ssh -L 9877:localhost:9877 oscarcode91@192.168.64.5",
	})
}

func resolveBrowserOpenCommand() []string {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("open"); err == nil {
			return []string{"open"}
		}
	case "linux":
		if isSSHSession() && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return nil
		}
		if _, err := exec.LookPath("xdg-open"); err == nil {
			return []string{"xdg-open"}
		}
		if _, err := exec.LookPath("gio"); err == nil {
			return []string{"gio", "open"}
		}
	}
	return nil
}

func isSSHSession() bool {
	return os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != ""
}

func tryOpenBrowserURL(rawURL string) (bool, string) {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false, ""
	}

	argv := resolveBrowserOpenCommand()
	if len(argv) == 0 {
		return false, ""
	}

	cmd := exec.Command(argv[0], append(argv[1:], parsed.String())...)
	if err := cmd.Start(); err != nil {
		return false, strings.Join(argv, " ")
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return true, strings.Join(argv, " ")
}

// gmailHTTPClient returns an authenticated HTTP client, refreshing the token if needed.
func gmailHTTPClient(ctx context.Context) (*http.Client, error) {
	creds, err := loadGmailCredentials()
	if err != nil {
		return nil, err
	}
	tok, err := loadGmailToken()
	if err != nil {
		return nil, err
	}
	cfg := gmailOAuth2Config(creds)
	ts := cfg.TokenSource(ctx, tok)

	newTok, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("gmail token refresh failed: %w — prueba conectarte a Gmail de nuevo", err)
	}
	if newTok.AccessToken != tok.AccessToken {
		_ = saveGmailToken(newTok)
	}

	return oauth2.NewClient(ctx, ts), nil
}

// ---------------------------------------------------------------------------
// connect_gmail tool — single-command setup + OAuth orchestration.
// ---------------------------------------------------------------------------

type connectGmailTool struct {
	name string
}

func NewConnectGmailTool() Tool { return &connectGmailTool{name: "connect_gmail"} }

// NewGmailConnectTool is kept as a compatibility constructor for older call sites.
func NewGmailConnectTool() Tool { return &connectGmailTool{name: "gmail_connect"} }

func (t *connectGmailTool) Name() string { return t.name }

func (t *connectGmailTool) Description() string {
	return "Single-command Gmail connection flow for Sam. Use this whenever the user wants to connect Gmail. It reuses saved credentials, or accepts client_id/client_secret, a Google OAuth credentials JSON, or a path to that JSON. If credentials are missing it opens the Google Cloud Console pages and returns the exact next step. If credentials are available it saves them and opens Google OAuth automatically."
}

func (t *connectGmailTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"client_id": {
				Type:        "string",
				Description: "OAuth2 client ID from Google Cloud Console (APIs & Services → Credentials → Desktop app).",
			},
			"client_secret": {
				Type:        "string",
				Description: "OAuth2 client secret from Google Cloud Console.",
			},
			"credentials_json": {
				Type:        "string",
				Description: "Full contents of the Google OAuth desktop credentials JSON downloaded from Google Cloud Console.",
			},
			"credentials_path": {
				Type:        "string",
				Description: "Path to the Google OAuth desktop credentials JSON file downloaded from Google Cloud Console.",
			},
		},
	}
}

func (t *connectGmailTool) Execute(ctx context.Context, arguments string) Result {
	var params connectGmailParams
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	creds, credentialsSource, err := resolveConnectGmailCredentials(params)
	if err != nil {
		if errors.Is(err, errGmailNotConfigured) {
			return gmailNeedsCredentialsResult()
		}
		return ErrorResult(err.Error())
	}

	return startGmailAuthorizationFlow(creds, credentialsSource)
}

// ---------------------------------------------------------------------------
// gmail_setup tool — legacy blocking version kept for compatibility.
// Prefer gmail_connect.
// ---------------------------------------------------------------------------

type gmailSetupTool struct{}

func NewGmailSetupTool() Tool { return &gmailSetupTool{} }

func (t *gmailSetupTool) Name() string { return "gmail_setup" }

func (t *gmailSetupTool) Description() string {
	return "Save or replace Gmail OAuth2 credentials without starting the authorization flow. Only use this when the user explicitly wants to store or update client_id and client_secret without connecting yet. For normal 'connect Gmail' requests, use gmail_connect instead."
}

func (t *gmailSetupTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"client_id": {
				Type:        "string",
				Description: "OAuth2 client ID from Google Cloud Console.",
			},
			"client_secret": {
				Type:        "string",
				Description: "OAuth2 client secret from Google Cloud Console.",
			},
		},
		Required: []string{"client_id", "client_secret"},
	}
}

func (t *gmailSetupTool) Execute(ctx context.Context, arguments string) Result {
	var params connectGmailParams
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}
	creds, _, err := resolveConnectGmailCredentials(params)
	if err != nil {
		if errors.Is(err, errGmailNotConfigured) {
			return gmailNeedsCredentialsResult()
		}
		return ErrorResult(err.Error())
	}
	if err := saveGmailCredentials(*creds); err != nil {
		return ErrorResult(fmt.Sprintf("error guardando credenciales: %s", err))
	}
	return JSONResult(map[string]any{
		"ok":      true,
		"message": "Credenciales guardadas. Usa connect_gmail para abrir Google OAuth y conectar Gmail.",
	})
}

