package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/providers/copilotproxy"
	"github.com/oscarcode/elementary-claw/internal/providers/githubcopilot"
	"github.com/oscarcode/elementary-claw/internal/providers/openai"
	"github.com/oscarcode/elementary-claw/internal/providers/openaicodex"
)

type upstreamTarget struct {
	BaseURL string
	Headers http.Header
	Source  string
}

func resolveUpstreamTarget(paths config.Paths, cfg config.FileConfig) (upstreamTarget, error) {
	provider := strings.TrimSpace(cfg.Agent.Provider)
	switch provider {
	case "github-copilot":
		token, err := githubcopilot.ResolveAPIToken(githubcopilot.ResolveParams{
			Paths:    paths,
			UseCache: true,
		})
		if err != nil {
			return upstreamTarget{}, err
		}

		headers := make(http.Header)
		headers.Set("Authorization", "Bearer "+token.Token)
		headers.Set("Copilot-Integration-Id", "vscode-chat")
		headers.Set("Editor-Version", "vscode/1.96.0")
		headers.Set("Editor-Plugin-Version", "copilot/1.0.0")
		return upstreamTarget{
			BaseURL: strings.TrimRight(token.BaseURL, "/"),
			Headers: headers,
			Source:  token.Source,
		}, nil
	case "copilot-proxy":
		baseURL := strings.TrimSpace(cfg.Agent.BaseURL)
		if baseURL == "" {
			baseURL = copilotproxy.DefaultBaseURL
		}
		return upstreamTarget{
			BaseURL: strings.TrimRight(baseURL, "/"),
			Headers: make(http.Header),
			Source:  "config",
		}, nil
	case "openai":
		apiKey, source, err := openai.ResolveAPIKey(paths, "")
		if err != nil {
			return upstreamTarget{}, err
		}

		baseURL := strings.TrimSpace(cfg.Agent.BaseURL)
		if baseURL == "" {
			baseURL = openai.DefaultAPIBaseURL
		}

		headers := make(http.Header)
		headers.Set("Authorization", "Bearer "+apiKey)
		return upstreamTarget{
			BaseURL: strings.TrimRight(baseURL, "/"),
			Headers: headers,
			Source:  source,
		}, nil
	case "openai-codex":
		credential, source, err := openaicodex.ResolveCredential(context.Background(), paths)
		if err != nil {
			return upstreamTarget{}, err
		}

		headers := make(http.Header)
		headers.Set("Authorization", "Bearer "+credential.AccessToken)
		if accountID := strings.TrimSpace(credential.AccountID); accountID != "" {
			headers.Set("chatgpt-account-id", accountID)
		}
		return upstreamTarget{
			BaseURL: strings.TrimRight(openaicodex.DefaultBaseURL, "/"),
			Headers: headers,
			Source:  source,
		}, nil
	default:
		return upstreamTarget{}, fmt.Errorf("unsupported provider %q", provider)
	}
}

func buildUpstreamURL(target upstreamTarget, requestPath string) string {
	parsed, err := url.Parse(target.BaseURL)
	if err != nil {
		trimmedBaseURL := strings.TrimRight(target.BaseURL, "/")
		trimmedPath := strings.TrimPrefix(requestPath, "/")
		return trimmedBaseURL + "/" + trimmedPath
	}

	basePath := strings.TrimRight(parsed.Path, "/")
	upstreamPath := requestPath

	// Always strip /v1 prefix from the incoming request path.
	// The base URL determines whether /v1 is present (e.g. copilot-proxy
	// includes /v1, while github-copilot does not).
	if requestPath == "/v1" {
		upstreamPath = ""
	} else if strings.HasPrefix(requestPath, "/v1/") {
		upstreamPath = strings.TrimPrefix(requestPath, "/v1")
	}

	parsed.Path = basePath + upstreamPath
	return parsed.String()
}
