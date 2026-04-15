package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	webFetchDefaultTimeout = 30
	webFetchMaxBody        = 100000
)

type webFetchTool struct{}

// NewWebFetchTool creates a tool that fetches content from URLs via HTTP GET.
func NewWebFetchTool() Tool {
	return &webFetchTool{}
}

func (t *webFetchTool) Name() string { return "web_fetch" }

func (t *webFetchTool) Description() string {
	return "Fetch content from a URL via HTTP GET. Returns the response body as text. Useful for reading web pages, APIs, or downloading text content."
}

func (t *webFetchTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"url": {
				Type:        "string",
				Description: "The URL to fetch. Must be a fully-formed URL starting with http:// or https://.",
			},
			"headers": {
				Type:        "string",
				Description: "Optional HTTP headers as key:value pairs separated by newlines (e.g. \"Accept: application/json\\nAuthorization: Bearer token\").",
			},
			"timeout": {
				Type:        "number",
				Description: "Timeout in seconds. Defaults to 30.",
			},
		},
		Required: []string{"url"},
	}
}

func (t *webFetchTool) Execute(ctx context.Context, arguments string) Result {
	var params struct {
		URL     string `json:"url"`
		Headers string `json:"headers"`
		Timeout int    `json:"timeout"`
	}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	rawURL := strings.TrimSpace(params.URL)
	if rawURL == "" {
		return ErrorResult("url must not be empty")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return ErrorResult("url must start with http:// or https://")
	}

	timeout := params.Timeout
	if timeout <= 0 {
		timeout = webFetchDefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("create request: %s", err))
	}

	// Set a reasonable default User-Agent.
	req.Header.Set("User-Agent", "elementary-claw/1.0")

	// Parse optional headers.
	if headers := strings.TrimSpace(params.Headers); headers != "" {
		for _, line := range strings.Split(headers, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ErrorResult(fmt.Sprintf("request timed out after %d seconds", timeout))
		}
		return ErrorResult(fmt.Sprintf("fetch error: %s", err))
	}
	defer resp.Body.Close()

	// Read body with size limit.
	limitedReader := io.LimitReader(resp.Body, int64(webFetchMaxBody)+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return ErrorResult(fmt.Sprintf("read response body: %s", err))
	}

	text := string(body)
	truncated := false
	if len(body) > webFetchMaxBody {
		text = TruncateMiddle(text, webFetchMaxBody)
		truncated = true
	}

	// Build result with metadata header.
	var result strings.Builder
	result.WriteString(fmt.Sprintf("HTTP %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode)))
	result.WriteString(fmt.Sprintf("Content-Type: %s\n", resp.Header.Get("Content-Type")))
	if truncated {
		result.WriteString(fmt.Sprintf("(body truncated to %d bytes)\n", webFetchMaxBody))
	}
	result.WriteString("\n")
	result.WriteString(text)

	if resp.StatusCode >= 400 {
		return ErrorResult(result.String())
	}
	return TextResult(result.String())
}
