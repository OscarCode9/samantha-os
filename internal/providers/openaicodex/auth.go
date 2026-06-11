package openaicodex

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oscarcode/elementary-claw/internal/config"
)

const (
	DefaultBaseURL   = "https://chatgpt.com/backend-api/codex"
	DefaultModel     = "gpt-5.4"
	oauthClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	oauthRedirectURI = "http://localhost:1455/auth/callback"
	oauthScopes      = "openid profile email offline_access"
)

var (
	AuthEndpoint    = "https://auth.openai.com/oauth/authorize"
	TokenEndpoint   = "https://auth.openai.com/oauth/token"
	ErrNoCredential = errors.New("no OpenAI Codex credential found")
)

type Credential struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	AccountID    string
}

type OAuthInit struct {
	AuthURL string `json:"authUrl"`
	State   string `json:"state"`
}

type oauthState struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type authStore struct {
	Version  int                    `json:"version,omitempty"`
	Profiles map[string]authProfile `json:"profiles"`
}

type authProfile struct {
	Provider     string      `json:"provider"`
	Mode         string      `json:"mode"`
	Type         string      `json:"type,omitempty"`
	Token        string      `json:"token"`
	RefreshToken string      `json:"refresh_token,omitempty"`
	ExpiresAt    interface{} `json:"expires_at,omitempty"`
	AccountID    string      `json:"account_id,omitempty"`
}

func (credential Credential) Valid() bool {
	if strings.TrimSpace(credential.AccessToken) == "" {
		return false
	}
	if credential.ExpiresAt.IsZero() {
		return true
	}
	return time.Now().Add(30 * time.Second).Before(credential.ExpiresAt)
}

func BeginOAuth(paths config.Paths) (OAuthInit, error) {
	state, err := randomString(43)
	if err != nil {
		return OAuthInit{}, err
	}
	codeVerifier, err := randomString(64)
	if err != nil {
		return OAuthInit{}, err
	}
	codeChallenge := codeChallengeS256(codeVerifier)

	pending := oauthState{
		State:        state,
		CodeVerifier: codeVerifier,
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	if err := saveOAuthState(paths, pending); err != nil {
		return OAuthInit{}, err
	}

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {oauthClientID},
		"redirect_uri":          {oauthRedirectURI},
		"scope":                 {oauthScopes},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}

	return OAuthInit{
		AuthURL: AuthEndpoint + "?" + params.Encode(),
		State:   state,
	}, nil
}

func ExchangeOAuth(ctx context.Context, paths config.Paths, rawURL string) (Credential, error) {
	code, state, err := parseRedirectURL(rawURL)
	if err != nil {
		return Credential{}, err
	}

	pending, err := loadOAuthState(paths)
	if err != nil {
		return Credential{}, err
	}
	if pending.State == "" || pending.CodeVerifier == "" {
		return Credential{}, errors.New("no hay una autorización pendiente de ChatGPT")
	}
	if pending.State != state {
		return Credential{}, errors.New("el estado OAuth no coincide; intenta conectar de nuevo")
	}
	if time.Now().After(pending.ExpiresAt) {
		_ = os.Remove(oauthStatePath(paths))
		return Credential{}, errors.New("el enlace de autorización expiró; vuelve a intentarlo")
	}

	credential, err := exchangeCode(ctx, code, pending.CodeVerifier)
	if err != nil {
		return Credential{}, err
	}
	if err := SaveCredential(paths, credential); err != nil {
		return Credential{}, err
	}
	_ = os.Remove(oauthStatePath(paths))
	return credential, nil
}

func ResolveCredential(ctx context.Context, paths config.Paths) (Credential, string, error) {
	credential, err := loadStoredCredential(paths)
	if err == nil {
		if credential.Valid() {
			if credential.AccountID == "" {
				credential.AccountID = extractAccountID(credential.AccessToken)
			}
			return credential, "auth-store:openai-codex:default", nil
		}
		if strings.TrimSpace(credential.RefreshToken) != "" {
			refreshed, refreshErr := refreshCredential(ctx, credential.RefreshToken)
			if refreshErr == nil {
				if err := SaveCredential(paths, refreshed); err != nil {
					return Credential{}, "", err
				}
				return refreshed, "auth-store:openai-codex:default:refreshed", nil
			}
		}
	}
	if err != nil && !errors.Is(err, ErrNoCredential) {
		return Credential{}, "", err
	}

	fileCredential, fileSource, fileErr := loadCodexCLIAuth(paths)
	if fileErr != nil {
		return Credential{}, "", fileErr
	}
	if !fileCredential.Valid() {
		return Credential{}, "", ErrNoCredential
	}
	return fileCredential, fileSource, nil
}

func SaveCredential(paths config.Paths, credential Credential) error {
	credential.AccessToken = strings.TrimSpace(credential.AccessToken)
	if credential.AccessToken == "" {
		return errors.New("OpenAI Codex access token must not be empty")
	}
	if credential.AccountID == "" {
		credential.AccountID = extractAccountID(credential.AccessToken)
	}

	store, err := readAuthStore(paths)
	if err != nil {
		return err
	}
	if store.Version == 0 {
		store.Version = 1
	}
	if store.Profiles == nil {
		store.Profiles = map[string]authProfile{}
	}

	profile := authProfile{
		Provider:     "openai-codex",
		Mode:         "active",
		Type:         "oauth",
		Token:        credential.AccessToken,
		RefreshToken: strings.TrimSpace(credential.RefreshToken),
		AccountID:    credential.AccountID,
	}
	if !credential.ExpiresAt.IsZero() {
		profile.ExpiresAt = credential.ExpiresAt.UnixMilli()
	}

	store.Profiles["openai-codex:default"] = profile
	return writeAuthStore(paths, store)
}

func loadStoredCredential(paths config.Paths) (Credential, error) {
	store, err := readAuthStore(paths)
	if err != nil {
		return Credential{}, err
	}
	profile, ok := store.Profiles["openai-codex:default"]
	if !ok {
		return Credential{}, ErrNoCredential
	}
	if strings.TrimSpace(profile.Token) == "" {
		return Credential{}, ErrNoCredential
	}
	expiresAt, err := parseExpiresAt(profile.ExpiresAt)
	if err != nil {
		return Credential{}, err
	}
	return Credential{
		AccessToken:  strings.TrimSpace(profile.Token),
		RefreshToken: strings.TrimSpace(profile.RefreshToken),
		ExpiresAt:    expiresAt,
		AccountID:    strings.TrimSpace(profile.AccountID),
	}, nil
}

func readAuthStore(paths config.Paths) (authStore, error) {
	data, err := os.ReadFile(paths.AuthPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return authStore{
				Version:  1,
				Profiles: map[string]authProfile{},
			}, nil
		}
		return authStore{}, fmt.Errorf("read auth store: %w", err)
	}

	var store authStore
	if err := json.Unmarshal(data, &store); err != nil {
		return authStore{}, fmt.Errorf("decode auth store: %w", err)
	}
	if store.Profiles == nil {
		store.Profiles = map[string]authProfile{}
	}
	if store.Version == 0 {
		store.Version = 1
	}
	return store, nil
}

func writeAuthStore(paths config.Paths, store authStore) error {
	if err := os.MkdirAll(filepath.Dir(paths.AuthPath), 0o700); err != nil {
		return fmt.Errorf("create auth directory: %w", err)
	}
	payload, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encode auth store: %w", err)
	}
	if err := os.WriteFile(paths.AuthPath, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("write auth store: %w", err)
	}
	return nil
}

func loadCodexCLIAuth(paths config.Paths) (Credential, string, error) {
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		codexHome = filepath.Join(paths.HomeDir, ".codex")
	}
	authPath := filepath.Join(codexHome, "auth.json")
	data, err := os.ReadFile(authPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credential{}, "", ErrNoCredential
		}
		return Credential{}, "", fmt.Errorf("read Codex auth.json: %w", err)
	}

	var payload struct {
		Tokens struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return Credential{}, "", fmt.Errorf("decode Codex auth.json: %w", err)
	}
	if strings.TrimSpace(payload.Tokens.AccessToken) == "" {
		return Credential{}, "", ErrNoCredential
	}

	credential := Credential{
		AccessToken:  strings.TrimSpace(payload.Tokens.AccessToken),
		RefreshToken: strings.TrimSpace(payload.Tokens.RefreshToken),
		ExpiresAt:    time.Now().Add(55 * time.Minute),
	}
	credential.AccountID = extractAccountID(credential.AccessToken)
	return credential, "codex-auth-file:" + authPath, nil
}

func refreshCredential(ctx context.Context, refreshToken string) (Credential, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {oauthClientID},
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Credential{}, fmt.Errorf("build OpenAI Codex refresh request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "elementary-claw/1.0")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Credential{}, fmt.Errorf("refresh OpenAI Codex token: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return Credential{}, fmt.Errorf("read OpenAI Codex refresh response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Credential{}, fmt.Errorf("OpenAI Codex refresh failed: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	return parseTokenResponse(body, refreshToken)
}

func exchangeCode(ctx context.Context, code string, codeVerifier string) (Credential, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {oauthClientID},
		"code":          {code},
		"code_verifier": {codeVerifier},
		"redirect_uri":  {oauthRedirectURI},
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Credential{}, fmt.Errorf("build OpenAI Codex token exchange request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "elementary-claw/1.0")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Credential{}, fmt.Errorf("exchange OpenAI Codex authorization code: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return Credential{}, fmt.Errorf("read OpenAI Codex token exchange response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Credential{}, fmt.Errorf("OpenAI Codex authorization failed: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	return parseTokenResponse(body, "")
}

func parseTokenResponse(body []byte, fallbackRefreshToken string) (Credential, error) {
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Credential{}, fmt.Errorf("decode OpenAI Codex token response: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return Credential{}, errors.New("OpenAI Codex token response missing access token")
	}

	refreshToken := strings.TrimSpace(payload.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(fallbackRefreshToken)
	}

	credential := Credential{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: refreshToken,
		AccountID:    extractAccountID(payload.AccessToken),
	}
	if payload.ExpiresIn > 0 {
		credential.ExpiresAt = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	}
	return credential, nil
}

func parseRedirectURL(rawURL string) (string, string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", "", errors.New("pega la URL completa de redirección de ChatGPT")
	}

	parsed, err := url.Parse(trimmed)
	if err == nil {
		code := strings.TrimSpace(parsed.Query().Get("code"))
		state := strings.TrimSpace(parsed.Query().Get("state"))
		if code != "" && state != "" {
			return code, state, nil
		}
	}

	if hashIndex := strings.Index(trimmed, "#"); hashIndex > 0 {
		code := strings.TrimSpace(trimmed[:hashIndex])
		state := strings.TrimSpace(trimmed[hashIndex+1:])
		if code != "" && state != "" {
			return code, state, nil
		}
	}

	return "", "", errors.New("formato de URL inválido; pega la URL completa de redirección")
}

func parseExpiresAt(raw interface{}) (time.Time, error) {
	switch value := raw.(type) {
	case nil:
		return time.Time{}, nil
	case float64:
		return unixOrMilliToTime(int64(value)), nil
	case int64:
		return unixOrMilliToTime(value), nil
	case json.Number:
		number, err := value.Int64()
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid expires_at value: %w", err)
		}
		return unixOrMilliToTime(number), nil
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return time.Time{}, nil
		}
		if parsed, err := time.Parse(time.RFC3339, text); err == nil {
			return parsed, nil
		}
		var number int64
		if _, err := fmt.Sscanf(text, "%d", &number); err == nil {
			return unixOrMilliToTime(number), nil
		}
		return time.Time{}, fmt.Errorf("invalid expires_at value %q", value)
	default:
		return time.Time{}, fmt.Errorf("unsupported expires_at type %T", raw)
	}
}

func unixOrMilliToTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value < 10_000_000_000 {
		return time.Unix(value, 0).UTC()
	}
	return time.UnixMilli(value).UTC()
}

func saveOAuthState(paths config.Paths, state oauthState) error {
	if err := os.MkdirAll(paths.CredentialsDir, 0o700); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode OpenAI Codex OAuth state: %w", err)
	}
	if err := os.WriteFile(oauthStatePath(paths), append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("write OpenAI Codex OAuth state: %w", err)
	}
	return nil
}

func loadOAuthState(paths config.Paths) (oauthState, error) {
	data, err := os.ReadFile(oauthStatePath(paths))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return oauthState{}, nil
		}
		return oauthState{}, fmt.Errorf("read OpenAI Codex OAuth state: %w", err)
	}
	var state oauthState
	if err := json.Unmarshal(data, &state); err != nil {
		return oauthState{}, fmt.Errorf("decode OpenAI Codex OAuth state: %w", err)
	}
	return state, nil
}

func oauthStatePath(paths config.Paths) string {
	return filepath.Join(paths.CredentialsDir, "openai-codex.oauth-state.json")
}

func randomString(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)[:length], nil
}

func codeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func extractAccountID(token string) string {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return ""
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return ""
	}
	authPayload, _ := payload["https://api.openai.com/auth"].(map[string]any)
	accountID, _ := authPayload["chatgpt_account_id"].(string)
	return strings.TrimSpace(accountID)
}
