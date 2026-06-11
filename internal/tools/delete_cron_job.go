package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type deleteCronJobTool struct {
	baseURL string
	client  *http.Client
	run     commandRunner
}

// NewDeleteCronJobTool creates a tool that removes a cron job by ID.
func NewDeleteCronJobTool(baseURL string) Tool {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		trimmed = defaultCronBaseURL
	}
	return &deleteCronJobTool{
		baseURL: strings.TrimRight(trimmed, "/"),
		client:  &http.Client{Timeout: 10 * time.Second},
		run:     defaultCommandRunner,
	}
}

func (t *deleteCronJobTool) Name() string { return "delete_cron_job" }

func (t *deleteCronJobTool) Description() string {
	return "Delete an existing claw cron job by ID."
}

func (t *deleteCronJobTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"job_id": {
				Type:        "string",
				Description: "ID of the cron job to delete.",
			},
		},
		Required: []string{"job_id"},
	}
}

func (t *deleteCronJobTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		JobID string `json:"job_id"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	jobID := strings.TrimSpace(params.JobID)
	if jobID == "" {
		return ErrorResult("job_id must not be empty")
	}

	var calendarUID string
	if current, err := t.fetchJob(ctx, jobID); err == nil {
		calendarUID = extractCalendarUIDFromJob(current)
	}
	calendarDeleted := false
	calendarDeleteError := ""
	if calendarUID != "" {
		if err := deleteCalendarEvent(ctx, t.run, calendarUID); err != nil {
			calendarDeleteError = err.Error()
		} else {
			calendarDeleted = true
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, t.baseURL+"/cron/jobs/"+jobID, nil)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create request: %v", err))
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to call cron API: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrorResult(fmt.Sprintf("cron API returned HTTP %d", resp.StatusCode))
	}

	return JSONResult(map[string]any{
		"cronDeleted":           true,
		"jobID":                 jobID,
		"linkedCalendarUID":     calendarUID,
		"linkedCalendarDeleted": calendarDeleted,
		"calendarDeleteError":   calendarDeleteError,
	})
}

func (t *deleteCronJobTool) fetchJob(ctx context.Context, jobID string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/cron/jobs/"+jobID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cron API returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	parsed := map[string]any{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func extractCalendarUIDFromJob(job map[string]any) string {
	payload, ok := job["payload"].(map[string]any)
	if !ok {
		return ""
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return ""
	}
	uid, _ := data["calendar_event_uid"].(string)
	return strings.TrimSpace(uid)
}
