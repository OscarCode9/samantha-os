package runtime

import (
	"net/http"
	"strings"
)

const (
	githubRateLimitErrorClass = "github-rate-limit"
	openAICodexOfferPrefix    = "__GITHUB_RATE_LIMIT__::"
)

func classifyUpstreamError(provider string, statusCode int, body string) string {
	if strings.TrimSpace(provider) != "github-copilot" {
		return ""
	}

	if statusCode == http.StatusTooManyRequests {
		return githubRateLimitErrorClass
	}

	normalized := normalizeProviderErrorText(body)
	switch {
	case strings.Contains(normalized, "session limit"),
		strings.Contains(normalized, "session limits"),
		strings.Contains(normalized, "chat limit"),
		strings.Contains(normalized, "chat limits"):
		return githubRateLimitErrorClass
	default:
		return ""
	}
}

func decorateGatewayErrorMessage(provider string, statusCode int, body string) string {
	text := strings.TrimSpace(body)
	if classifyUpstreamError(provider, statusCode, text) == githubRateLimitErrorClass {
		return openAICodexOfferPrefix + text
	}
	return text
}

func setProviderErrorHeaders(headers http.Header, provider string, statusCode int, body string) {
	if errorClass := classifyUpstreamError(provider, statusCode, body); errorClass != "" {
		headers.Set("X-Claw-Error-Class", errorClass)
	}
}

func normalizeProviderErrorText(body string) string {
	return strings.Join(strings.Fields(strings.ToLower(body)), " ")
}
