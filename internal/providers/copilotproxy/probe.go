package copilotproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "http://localhost:4000/openai/v1"

type ProbeResult struct {
	NormalizedBaseURL string
	ProbePath         string
	StatusCode        int
	Ready             bool
	Detail            string
}

func Probe(baseURL string) (ProbeResult, error) {
	normalized, err := NormalizeBaseURL(baseURL)
	if err != nil {
		return ProbeResult{}, err
	}

	client := &http.Client{Timeout: 10 * time.Second}

	modelsResult, err := probeEndpoint(client, normalized+"/models")
	if err == nil && modelsResult.Ready {
		modelsResult.NormalizedBaseURL = normalized
		return modelsResult, nil
	}

	chatResult, chatErr := probeChatCompletions(client, normalized+"/chat/completions")
	if chatErr == nil && chatResult.Ready {
		chatResult.NormalizedBaseURL = normalized
		return chatResult, nil
	}

	rootResult, rootErr := probeRoot(normalized, client)
	if rootErr == nil && rootResult.Ready {
		return ProbeResult{}, fmt.Errorf("%s", rootResult.Detail)
	}

	if err != nil {
		return ProbeResult{}, err
	}
	if chatErr != nil {
		return ProbeResult{}, chatErr
	}
	if rootErr != nil {
		return ProbeResult{}, rootErr
	}

	return ProbeResult{}, fmt.Errorf("proxy probe failed for %s", normalized)
}

func NormalizeBaseURL(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		trimmed = DefaultBaseURL
	}

	normalized := strings.TrimRight(trimmed, "/")
	if !strings.HasSuffix(strings.ToLower(normalized), "/v1") {
		normalized += "/v1"
	}

	request, err := http.NewRequest(http.MethodGet, normalized, nil)
	if err != nil {
		return "", fmt.Errorf("invalid proxy base URL: %w", err)
	}
	if request.URL.Scheme != "http" && request.URL.Scheme != "https" {
		return "", fmt.Errorf("invalid proxy URL scheme %q", request.URL.Scheme)
	}

	return normalized, nil
}

func probeEndpoint(client *http.Client, url string) (ProbeResult, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("build proxy probe request: %w", err)
	}

	response, err := client.Do(request)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("probe %s: %w", url, err)
	}
	defer response.Body.Close()

	ready := false
	detail := fmt.Sprintf("HTTP %d", response.StatusCode)
	switch response.StatusCode {
	case http.StatusOK, http.StatusUnauthorized, http.StatusForbidden, http.StatusMethodNotAllowed:
		ready = true
	}

	return ProbeResult{
		ProbePath:  url,
		StatusCode: response.StatusCode,
		Ready:      ready,
		Detail:     detail,
	}, nil
}

func probeChatCompletions(client *http.Client, url string) (ProbeResult, error) {
	payload := map[string]any{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("marshal proxy probe payload: %w", err)
	}

	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ProbeResult{}, fmt.Errorf("build proxy probe request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("probe %s: %w", url, err)
	}
	defer response.Body.Close()

	ready := false
	detail := fmt.Sprintf("HTTP %d", response.StatusCode)
	switch response.StatusCode {
	case http.StatusOK, http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusMethodNotAllowed, http.StatusUnprocessableEntity:
		ready = true
	}

	return ProbeResult{
		ProbePath:  url,
		StatusCode: response.StatusCode,
		Ready:      ready,
		Detail:     detail,
	}, nil
}

func probeRoot(normalizedBaseURL string, client *http.Client) (ProbeResult, error) {
	parsed, err := url.Parse(normalizedBaseURL)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("parse root probe URL: %w", err)
	}
	parsed.Path = "/"
	parsed.RawQuery = ""
	rootURL := parsed.String()
	request, err := http.NewRequest(http.MethodGet, rootURL, nil)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("build root probe request: %w", err)
	}

	response, err := client.Do(request)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("probe %s: %w", rootURL, err)
	}
	defer response.Body.Close()

	var payload struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return ProbeResult{}, fmt.Errorf("decode root probe response: %w", err)
	}

	message := strings.TrimSpace(payload.Message)
	if response.StatusCode == http.StatusOK && strings.Contains(strings.ToLower(message), "vscode lm api proxy") {
		return ProbeResult{
			ProbePath:  rootURL,
			StatusCode: response.StatusCode,
			Ready:      true,
			Detail:     fmt.Sprintf("VSCode LM API Proxy is running at %s, but it does not expose the expected OpenAI-compatible /v1 endpoints yet", rootURL),
		}, nil
	}

	return ProbeResult{
		ProbePath:  rootURL,
		StatusCode: response.StatusCode,
		Ready:      false,
		Detail:     fmt.Sprintf("HTTP %d", response.StatusCode),
	}, nil
}
