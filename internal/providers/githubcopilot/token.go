package githubcopilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/oscarcode/elementary-claw/internal/config"
)

const (
	DefaultCopilotAPIBaseURL = "https://api.individual.githubcopilot.com"
	defaultCopilotTokenURL   = "https://api.github.com/copilot_internal/v2/token"
)

var proxyEndpointPattern = regexp.MustCompile(`(?:^|;)\s*proxy-ep=([^;\s]+)`)

type ResolveParams struct {
	Paths       config.Paths
	GitHubToken string
	UseCache    bool
	HTTPClient  *http.Client
	TokenURL    string
}

type APIToken struct {
	Token             string
	ExpiresAt         int64
	Source            string
	BaseURL           string
	GitHubTokenSource string
}

type cachedToken struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

type authStore struct {
	Profiles map[string]authProfile `json:"profiles"`
}

type authProfile struct {
	Provider string `json:"provider"`
	Mode     string `json:"mode"`
	Type     string `json:"type"`
	Token    string `json:"token"`
}

func ResolveAPIToken(params ResolveParams) (APIToken, error) {
	githubToken, githubTokenSource, err := resolveGitHubToken(params.Paths, params.GitHubToken)
	if err != nil {
		return APIToken{}, err
	}

	if params.UseCache {
		cached, ok, err := loadCachedToken(params.Paths.CopilotTokenCachePath)
		if err != nil {
			return APIToken{}, err
		}
		if ok && tokenUsable(cached, time.Now().UnixMilli()) {
			return APIToken{
				Token:             cached.Token,
				ExpiresAt:         cached.ExpiresAt,
				Source:            "cache:" + params.Paths.CopilotTokenCachePath,
				BaseURL:           DeriveAPIBaseURLFromToken(cached.Token),
				GitHubTokenSource: githubTokenSource,
			}, nil
		}
	}

	client := params.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	tokenURL := strings.TrimSpace(params.TokenURL)
	if tokenURL == "" {
		tokenURL = defaultCopilotTokenURL
	}

	request, err := http.NewRequest(http.MethodGet, tokenURL, nil)
	if err != nil {
		return APIToken{}, fmt.Errorf("build copilot token request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+githubToken)
	request.Header.Set("User-Agent", "elementary-claw/1.0")

	response, err := client.Do(request)
	if err != nil {
		return APIToken{}, fmt.Errorf("exchange github token for copilot token: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return APIToken{}, fmt.Errorf("copilot token exchange failed: HTTP %d", response.StatusCode)
	}

	var payload struct {
		Token     string      `json:"token"`
		ExpiresAt interface{} `json:"expires_at"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return APIToken{}, fmt.Errorf("decode copilot token response: %w", err)
	}

	expiresAt, err := parseExpiresAt(payload.ExpiresAt)
	if err != nil {
		return APIToken{}, err
	}
	if strings.TrimSpace(payload.Token) == "" {
		return APIToken{}, errors.New("copilot token response missing token")
	}

	if err := saveCachedToken(params.Paths.CopilotTokenCachePath, cachedToken{
		Token:     payload.Token,
		ExpiresAt: expiresAt,
		UpdatedAt: time.Now().UnixMilli(),
	}); err != nil {
		return APIToken{}, err
	}

	return APIToken{
		Token:             payload.Token,
		ExpiresAt:         expiresAt,
		Source:            "fetched:" + tokenURL,
		BaseURL:           DeriveAPIBaseURLFromToken(payload.Token),
		GitHubTokenSource: githubTokenSource,
	}, nil
}

func DeriveAPIBaseURLFromToken(token string) string {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return DefaultCopilotAPIBaseURL
	}

	match := proxyEndpointPattern.FindStringSubmatch(trimmed)
	if len(match) < 2 {
		return DefaultCopilotAPIBaseURL
	}

	host := resolveProxyHost(match[1])
	if host == "" {
		return DefaultCopilotAPIBaseURL
	}

	if strings.HasPrefix(host, "proxy.") {
		host = "api." + strings.TrimPrefix(host, "proxy.")
	}
	return "https://" + host
}

func resolveProxyHost(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	text := trimmed
	if !strings.HasPrefix(strings.ToLower(text), "http://") && !strings.HasPrefix(strings.ToLower(text), "https://") {
		text = "https://" + text
	}

	parsed, err := url.Parse(text)
	if err != nil {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func parseExpiresAt(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case float64:
		parsed := int64(typed)
		if parsed < 100_000_000_000 {
			return parsed * 1000, nil
		}
		return parsed, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, errors.New("copilot token response missing expires_at")
		}
		var parsed int64
		if _, err := fmt.Sscanf(trimmed, "%d", &parsed); err != nil {
			return 0, errors.New("copilot token response has invalid expires_at")
		}
		if parsed < 100_000_000_000 {
			return parsed * 1000, nil
		}
		return parsed, nil
	default:
		return 0, errors.New("copilot token response missing expires_at")
	}
}

func tokenUsable(token cachedToken, nowMs int64) bool {
	return token.ExpiresAt-nowMs > 5*60*1000
}

func loadCachedToken(path string) (cachedToken, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cachedToken{}, false, nil
		}
		return cachedToken{}, false, fmt.Errorf("read copilot token cache: %w", err)
	}

	var token cachedToken
	if err := json.Unmarshal(data, &token); err != nil {
		return cachedToken{}, false, fmt.Errorf("decode copilot token cache: %w", err)
	}
	if strings.TrimSpace(token.Token) == "" || token.ExpiresAt == 0 {
		return cachedToken{}, false, nil
	}
	return token, true, nil
}

func saveCachedToken(path string, token cachedToken) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create copilot credential directory: %w", err)
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal copilot token cache: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write copilot token cache: %w", err)
	}
	return nil
}

func resolveGitHubToken(paths config.Paths, explicit string) (string, string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, "flag", nil
	}

	for _, envName := range []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			return value, "env:" + envName, nil
		}
	}

	data, err := os.ReadFile(paths.AuthPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", errors.New("no GitHub token found in flags, environment, or auth store")
		}
		return "", "", fmt.Errorf("read auth store: %w", err)
	}

	var store authStore
	if err := json.Unmarshal(data, &store); err != nil {
		return "", "", fmt.Errorf("decode auth store: %w", err)
	}

	for profileID, profile := range store.Profiles {
		if profile.Provider != "github-copilot" {
			continue
		}
		if token := strings.TrimSpace(profile.Token); token != "" {
			return token, "auth-store:" + profileID, nil
		}
	}

	return "", "", errors.New("no GitHub token found in flags, environment, or auth store")
}

// SaveGitHubToken saves the GitHub token to the auth-profiles.json store.
func SaveGitHubToken(paths config.Paths, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("GitHub token must not be empty")
	}

	var store map[string]any
	data, err := os.ReadFile(paths.AuthPath)
	if err == nil {
		_ = json.Unmarshal(data, &store)
	}
	if store == nil {
		store = map[string]any{
			"version":  1,
			"profiles": map[string]any{},
		}
	}

	profiles, _ := store["profiles"].(map[string]any)
	if profiles == nil {
		profiles = map[string]any{}
	}

	profiles["github-copilot:default"] = map[string]any{
		"provider": "github-copilot",
		"mode":     "active",
		"type":     "oauth",
		"token":    token,
	}
	store["profiles"] = profiles

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
