package runtime

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/providers/copilotproxy"
	"github.com/oscarcode/elementary-claw/internal/providers/githubcopilot"
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
	if strings.HasSuffix(strings.ToLower(basePath), "/v1") {
		if requestPath == "/v1" {
			upstreamPath = ""
		} else if strings.HasPrefix(requestPath, "/v1/") {
			upstreamPath = strings.TrimPrefix(requestPath, "/v1")
		}
	}

	parsed.Path = basePath + upstreamPath
	return parsed.String()
}
