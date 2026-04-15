package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Token holds an OAuth2 access/refresh token pair for an MCP server.
type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Scope        string    `json:"scope,omitempty"`
}

// Valid returns true when the token is present and not expired (with a 30s
// safety margin).
func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	if t.ExpiresAt.IsZero() {
		return true
	}
	return time.Now().Add(30 * time.Second).Before(t.ExpiresAt)
}

// LoadToken reads a cached token from the given file path.  Returns nil
// without an error when the file does not exist yet.
func LoadToken(path string) (*Token, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read token %s: %w", path, err)
	}
	var tok Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("parse token %s: %w", path, err)
	}
	return &tok, nil
}

// SaveToken writes a token to the given file path (mode 0600).
func SaveToken(path string, tok *Token) error {
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// RefreshAccessToken exchanges a refresh_token for a new access token using
// the tokenEndpoint URL and the given clientID/clientSecret.
func RefreshAccessToken(tokenEndpoint, clientID, clientSecret, refreshToken string) (*Token, error) {
	params := url.Values{}
	params.Set("grant_type", "refresh_token")
	params.Set("refresh_token", refreshToken)
	params.Set("client_id", clientID)
	params.Set("client_secret", clientSecret)

	resp, err := http.PostForm(tokenEndpoint, params)
	if err != nil {
		return nil, fmt.Errorf("token refresh request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh %s: %s", resp.Status, string(body))
	}

	return parseTokenResponse(body)
}

// pkce generates a random code_verifier and its S256 code_challenge.
func pkce() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}

// parseTokenResponse unmarshals a successful OAuth2 token endpoint response.
func parseTokenResponse(body []byte) (*Token, error) {
	var raw struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	tok := &Token{
		AccessToken:  raw.AccessToken,
		TokenType:    raw.TokenType,
		RefreshToken: raw.RefreshToken,
		Scope:        raw.Scope,
	}
	if raw.ExpiresIn > 0 {
		tok.ExpiresAt = time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second)
	}
	return tok, nil
}

// DiscoverMetadata fetches OAuth2 Authorization Server Metadata from the
// standard well-known URL relative to issuerBase.
func DiscoverMetadata(issuerBase string) (authEndpoint, tokenEndpoint string, err error) {
	wellKnown := strings.TrimRight(issuerBase, "/") + "/.well-known/oauth-authorization-server"
	resp, err := http.Get(wellKnown) //nolint:noctx
	if err != nil {
		return "", "", fmt.Errorf("fetch oauth metadata: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var meta struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return "", "", fmt.Errorf("parse oauth metadata: %w", err)
	}
	return meta.AuthorizationEndpoint, meta.TokenEndpoint, nil
}

// ExchangeCode exchanges an authorization code (with PKCE verifier) for a
// token at tokenEndpoint.
func ExchangeCode(tokenEndpoint, clientID, clientSecret, code, codeVerifier, redirectURI string) (*Token, error) {
	params := url.Values{}
	params.Set("grant_type", "authorization_code")
	params.Set("code", code)
	params.Set("redirect_uri", redirectURI)
	params.Set("client_id", clientID)
	params.Set("client_secret", clientSecret)
	params.Set("code_verifier", codeVerifier)

	resp, err := http.PostForm(tokenEndpoint, params)
	if err != nil {
		return nil, fmt.Errorf("code exchange request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("code exchange %s: %s", resp.Status, string(body))
	}

	return parseTokenResponse(body)
}
