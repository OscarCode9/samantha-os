package openaicodex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oscarcode/elementary-claw/internal/config"
)

func TestBeginOAuthWritesStateAndBuildsAuthURL(t *testing.T) {
	paths := makeOpenAICodexTestPaths(t.TempDir())

	init, err := BeginOAuth(paths)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(init.State) == "" {
		t.Fatal("expected OAuth state")
	}
	if !strings.Contains(init.AuthURL, "code_challenge=") {
		t.Fatalf("expected code challenge in auth URL: %s", init.AuthURL)
	}
	if !strings.Contains(init.AuthURL, "state="+init.State) {
		t.Fatalf("expected auth URL to include state %q: %s", init.State, init.AuthURL)
	}

	state, err := loadOAuthState(paths)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != init.State {
		t.Fatalf("unexpected saved state: got %q want %q", state.State, init.State)
	}
	if strings.TrimSpace(state.CodeVerifier) == "" {
		t.Fatal("expected code verifier to be saved")
	}
	if time.Until(state.ExpiresAt) <= 0 {
		t.Fatalf("expected future expiration, got %s", state.ExpiresAt)
	}
}

func TestResolveCredentialRefreshesExpiredStoredCredential(t *testing.T) {
	paths := makeOpenAICodexTestPaths(t.TempDir())
	if err := SaveCredential(paths, Credential{
		AccessToken:  "expired-token",
		RefreshToken: "refresh-old",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	originalClient := http.DefaultClient
	originalTokenEndpoint := TokenEndpoint
	t.Cleanup(func() {
		http.DefaultClient = originalClient
		TokenEndpoint = originalTokenEndpoint
	})

	TokenEndpoint = "https://auth.openai.test/oauth/token"
	refreshedToken := testJWT("acct_refresh_123")
	http.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", request.Method)
			}
			if request.URL.String() != TokenEndpoint {
				t.Fatalf("unexpected token endpoint: %s", request.URL.String())
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), "grant_type=refresh_token") {
				t.Fatalf("expected refresh grant, got: %s", string(body))
			}
			payload := `{"access_token":"` + refreshedToken + `","refresh_token":"refresh-new","expires_in":3600}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(payload)),
				Request:    request,
			}, nil
		}),
	}

	credential, source, err := ResolveCredential(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if source != "auth-store:openai-codex:default:refreshed" {
		t.Fatalf("unexpected source: %s", source)
	}
	if credential.AccessToken != refreshedToken {
		t.Fatalf("unexpected refreshed access token: %s", credential.AccessToken)
	}
	if credential.RefreshToken != "refresh-new" {
		t.Fatalf("unexpected refreshed refresh token: %s", credential.RefreshToken)
	}
	if credential.AccountID != "acct_refresh_123" {
		t.Fatalf("unexpected account id: %s", credential.AccountID)
	}

	stored, err := loadStoredCredential(paths)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AccessToken != refreshedToken {
		t.Fatalf("expected refreshed token to persist, got %s", stored.AccessToken)
	}
}

func TestResolveCredentialFallsBackToCodexCLIAuth(t *testing.T) {
	paths := makeOpenAICodexTestPaths(t.TempDir())
	cliToken := testJWT("acct_cli_456")
	authPath := filepath.Join(paths.HomeDir, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		t.Fatal(err)
	}
	payload := `{"tokens":{"access_token":"` + cliToken + `","refresh_token":"cli-refresh"}}`
	if err := os.WriteFile(authPath, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}

	credential, source, err := ResolveCredential(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessToken != cliToken {
		t.Fatalf("unexpected CLI access token: %s", credential.AccessToken)
	}
	if credential.AccountID != "acct_cli_456" {
		t.Fatalf("unexpected CLI account id: %s", credential.AccountID)
	}
	if !strings.Contains(source, filepath.Join(".codex", "auth.json")) {
		t.Fatalf("unexpected CLI source: %s", source)
	}
}

func makeOpenAICodexTestPaths(root string) config.Paths {
	stateDir := filepath.Join(root, ".samantha")
	workspaceDir := filepath.Join(stateDir, "workspace")
	credentialsDir := filepath.Join(stateDir, "state", "credentials")
	return config.Paths{
		HomeDir:        root,
		StateDir:       stateDir,
		WorkspaceDir:   workspaceDir,
		CredentialsDir: credentialsDir,
		SessionsDir:    filepath.Join(stateDir, "state", "sessions"),
		ConfigPath:     filepath.Join(stateDir, "samantha.json"),
		AuthPath:       filepath.Join(stateDir, "agents", "main", "agent", "auth-profiles.json"),
	}
}

func testJWT(accountID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadMap := map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": accountID,
		},
	}
	payloadBytes, _ := json.Marshal(payloadMap)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + ".signature"
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
