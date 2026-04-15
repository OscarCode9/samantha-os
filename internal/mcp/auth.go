package mcp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// AuthFlowParams holds the parameters needed to run an interactive OAuth2
// authorization code + PKCE flow for an MCP server.
type AuthFlowParams struct {
	ServerName    string
	ClientID      string
	ClientSecret  string
	AuthEndpoint  string
	TokenEndpoint string
	Scopes        string
	TokenPath     string
	// RedirectURI overrides the default loopback callback URI.
	// When set to a non-loopback URL the CLI uses the manual paste flow.
	RedirectURI string
}

// RunAuthFlow performs an interactive OAuth2 authorization code + PKCE flow.
// When RedirectURI is a loopback address (or empty) it starts a local HTTP
// server. Otherwise it prints the URL and prompts the user to paste the
// callback URL — compatible with URIs like https://claude.ai/api/mcp/auth_callback.
func RunAuthFlow(p AuthFlowParams) error {
	verifier, challenge, err := pkce()
	if err != nil {
		return fmt.Errorf("generate PKCE: %w", err)
	}

	if isLoopbackURI(p.RedirectURI) {
		return runLoopbackFlow(p, verifier, challenge)
	}
	return runManualFlow(p, verifier, challenge)
}

// isLoopbackURI returns true when the URI targets localhost / 127.0.0.1 or is
// empty (caller will pick a random loopback port).
func isLoopbackURI(u string) bool {
	if u == "" {
		return true
	}
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "http://localhost") ||
		strings.HasPrefix(l, "http://127.0.0.1")
}

// buildAuthURL constructs the authorization URL with PKCE and all required
// parameters.
func buildAuthURL(endpoint, clientID, redirectURI, scopes, challenge string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", scopes)
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", challenge[:8])
	return endpoint + "?" + params.Encode()
}

// runLoopbackFlow starts a local HTTP server on a random port, opens the
// browser, and waits for the authorization code to arrive via redirect.
func runLoopbackFlow(p AuthFlowParams, verifier, challenge string) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start callback server: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			msg := r.URL.Query().Get("error_description")
			if msg == "" {
				msg = r.URL.Query().Get("error")
			}
			errCh <- fmt.Errorf("OAuth callback error: %s", msg)
			fmt.Fprint(w, "<html><body><h2>Authorization failed.</h2><p>"+msg+"</p></body></html>")
			return
		}
		codeCh <- code
		fmt.Fprint(w, "<html><body><h2>Authorization successful!</h2><p>You can close this window.</p></body></html>")
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener) //nolint:errcheck

	authURL := buildAuthURL(p.AuthEndpoint, p.ClientID, redirectURI, p.Scopes, challenge)
	fmt.Printf("Opening browser for MCP server %q authorization...\n", p.ServerName)
	fmt.Printf("If the browser does not open, visit:\n  %s\n\n", authURL)
	openBrowser(authURL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for OAuth callback (2 min)")
	}

	return exchangeAndSave(p, code, redirectURI, verifier)
}

// runManualFlow prints the authorization URL and prompts the user to paste the
// callback URL (or just the code) after authorizing in their browser.
// This is used when the registered redirect_uri is a non-loopback URL such as
// https://claude.ai/api/mcp/auth_callback.
func runManualFlow(p AuthFlowParams, verifier, challenge string) error {
	redirectURI := p.RedirectURI
	authURL := buildAuthURL(p.AuthEndpoint, p.ClientID, redirectURI, p.Scopes, challenge)

	fmt.Printf("\n[MCP Auth] Server: %s\n", p.ServerName)
	fmt.Printf("\nOpen this URL in your browser to authorize:\n\n  %s\n\n", authURL)
	fmt.Printf("After authorizing you will be redirected to:\n  %s?code=...\n\n", redirectURI)
	fmt.Print("Paste the full redirect URL (or just the code value) here:\n> ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	input = strings.TrimSpace(input)

	// Accept either a full redirect URL or a bare authorization code.
	code := input
	if strings.Contains(input, "?") || strings.Contains(input, "code=") {
		parsed, err := url.Parse(input)
		if err != nil {
			return fmt.Errorf("parse redirect URL: %w", err)
		}
		code = parsed.Query().Get("code")
		if code == "" {
			return fmt.Errorf("no 'code' parameter found in pasted URL")
		}
	}
	if code == "" {
		return fmt.Errorf("no authorization code provided")
	}

	return exchangeAndSave(p, code, redirectURI, verifier)
}

func exchangeAndSave(p AuthFlowParams, code, redirectURI, verifier string) error {
	tok, err := ExchangeCode(p.TokenEndpoint, p.ClientID, p.ClientSecret, code, verifier, redirectURI)
	if err != nil {
		return err
	}

	if err := SaveToken(p.TokenPath, tok); err != nil {
		return err
	}

	fmt.Printf("Token saved to %s\n", p.TokenPath)
	if !tok.ExpiresAt.IsZero() {
		fmt.Printf("Expires at: %s\n", tok.ExpiresAt.Format(time.RFC3339))
	}
	return nil
}

// openBrowser tries to open a URL in the default system browser.
func openBrowser(u string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{u}
	case "linux":
		for _, bin := range []string{"xdg-open", "epiphany", "firefox", "chromium-browser"} {
			if _, err := exec.LookPath(bin); err == nil {
				cmd = bin
				args = []string{u}
				break
			}
		}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", strings.ReplaceAll(u, "&", "^&")}
	}

	if cmd == "" {
		return
	}
	exec.Command(cmd, args...).Start() //nolint:errcheck
}
