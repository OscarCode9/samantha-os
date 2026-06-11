package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type listCronJobsTool struct {
	baseURL string
	client  *http.Client
}

// NewListCronJobsTool creates a tool that lists jobs from the local cron API.
func NewListCronJobsTool(baseURL string) Tool {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		trimmed = defaultCronBaseURL
	}
	return &listCronJobsTool{
		baseURL: strings.TrimRight(trimmed, "/"),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *listCronJobsTool) Name() string { return "list_cron_jobs" }

func (t *listCronJobsTool) Description() string {
	return "List scheduled cron jobs configured in elementary-claw (not system crontab)."
}

func (t *listCronJobsTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"enabled_only": {
				Type:        "boolean",
				Description: "When true, return only enabled jobs.",
			},
		},
	}
}

func (t *listCronJobsTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		EnabledOnly bool `json:"enabled_only"`
	}{}
	if strings.TrimSpace(arguments) != "" {
		if err := ParseArgs(arguments, &params); err != nil {
			return ErrorResult(err.Error())
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/cron/jobs", nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create request: %v", err))
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to call cron API: %v", err))
	}
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ErrorResult(fmt.Sprintf("invalid cron API response: %v", err))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrorResult(fmt.Sprintf("cron API returned HTTP %d", resp.StatusCode))
	}

	if count, ok := body["count"].(float64); ok && count == 0 {
		return TextResult("No tienes jobs diarios configurados en claw cron.")
	}

	if params.EnabledOnly {
		jobs, ok := body["jobs"].([]any)
		if ok {
			filtered := make([]any, 0, len(jobs))
			for _, item := range jobs {
				entry, ok := item.(map[string]any)
				if !ok {
					continue
				}
				enabled, ok := entry["enabled"].(bool)
				if ok && enabled {
					filtered = append(filtered, entry)
				}
			}
			body["jobs"] = filtered
			body["count"] = len(filtered)
		}
	}

	return JSONResult(body)
}
