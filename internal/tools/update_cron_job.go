package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type updateCronJobTool struct {
	baseURL string
	client  *http.Client
}

// NewUpdateCronJobTool creates a tool that updates an existing claw cron job.
func NewUpdateCronJobTool(baseURL string) Tool {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		trimmed = defaultCronBaseURL
	}
	return &updateCronJobTool{
		baseURL: strings.TrimRight(trimmed, "/"),
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *updateCronJobTool) Name() string { return "update_cron_job" }

func (t *updateCronJobTool) Description() string {
	return "Update an existing claw cron job. Supports changing name, daily time, timezone, notification message/title/urgency, and enabled state."
}

func (t *updateCronJobTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"job_id": {
				Type:        "string",
				Description: "ID of the cron job to update.",
			},
			"name": {
				Type:        "string",
				Description: "New display name for the job.",
			},
			"time": {
				Type:        "string",
				Description: "Daily local time in 24h HH:MM format. Updates cron expression.",
			},
			"timezone": {
				Type:        "string",
				Description: "IANA timezone string for schedule metadata.",
			},
			"message": {
				Type:        "string",
				Description: "Notification body text.",
			},
			"title": {
				Type:        "string",
				Description: "Notification title.",
			},
			"urgency": {
				Type:        "string",
				Description: "Notification urgency.",
				Enum:        []string{"low", "normal", "critical"},
			},
			"enabled": {
				Type:        "boolean",
				Description: "Enable or disable the job.",
			},
		},
		Required: []string{"job_id"},
	}
}

func (t *updateCronJobTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		JobID    string `json:"job_id"`
		Name     string `json:"name"`
		Time     string `json:"time"`
		Timezone string `json:"timezone"`
		Message  string `json:"message"`
		Title    string `json:"title"`
		Urgency  string `json:"urgency"`
		Enabled  *bool  `json:"enabled"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	jobID := strings.TrimSpace(params.JobID)
	if jobID == "" {
		return ErrorResult("job_id must not be empty")
	}

	current, err := t.getCurrentJob(ctx, jobID)
	if err != nil {
		return ErrorResult(err.Error())
	}

	patch := map[string]any{}
	if name := strings.TrimSpace(params.Name); name != "" {
		patch["name"] = name
	}
	if params.Enabled != nil {
		patch["enabled"] = *params.Enabled
	}

	if schedulePatch, changed, err := applySchedulePatch(current, strings.TrimSpace(params.Time), strings.TrimSpace(params.Timezone)); err != nil {
		return ErrorResult(err.Error())
	} else if changed {
		patch["schedule"] = schedulePatch
	}

	if payloadPatch, changed, err := applyPayloadPatch(current, strings.TrimSpace(params.Title), strings.TrimSpace(params.Message), strings.TrimSpace(params.Urgency)); err != nil {
		return ErrorResult(err.Error())
	} else if changed {
		patch["payload"] = payloadPatch
	}

	if len(patch) == 0 {
		return ErrorResult("no update fields provided")
	}

	data, err := json.Marshal(patch)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to marshal patch: %v", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, t.baseURL+"/cron/jobs/"+jobID, bytes.NewReader(data))
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create request: %v", err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to call cron API: %v", err))
	}
	defer resp.Body.Close()

	responseBody := new(bytes.Buffer)
	_, _ = responseBody.ReadFrom(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrorResult(fmt.Sprintf("cron API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(responseBody.String())))
	}

	result := map[string]any{}
	if err := json.Unmarshal(responseBody.Bytes(), &result); err != nil {
		return ErrorResult(fmt.Sprintf("invalid cron API response: %v", err))
	}

	return JSONResult(result)
}

func (t *updateCronJobTool) getCurrentJob(ctx context.Context, jobID string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/cron/jobs/"+jobID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call cron API: %w", err)
	}
	defer resp.Body.Close()

	result := map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("invalid cron API response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cron API returned HTTP %d", resp.StatusCode)
	}

	return result, nil
}

func applySchedulePatch(current map[string]any, timeInput string, timezone string) (map[string]any, bool, error) {
	if timeInput == "" && timezone == "" {
		return nil, false, nil
	}

	currentSchedule, _ := current["schedule"].(map[string]any)
	schedule := map[string]any{}
	for k, v := range currentSchedule {
		schedule[k] = v
	}

	if strings.TrimSpace(timeInput) != "" {
		hour, minute, err := parseClock(timeInput)
		if err != nil {
			return nil, false, err
		}
		schedule["kind"] = "cron"
		schedule["expr"] = fmt.Sprintf("%d %d * * *", minute, hour)
	}

	if timezone != "" {
		schedule["timezone"] = timezone
	}

	return schedule, true, nil
}

func applyPayloadPatch(current map[string]any, title string, message string, urgency string) (map[string]any, bool, error) {
	if title == "" && message == "" && urgency == "" {
		return nil, false, nil
	}

	if urgency != "" && urgency != "low" && urgency != "normal" && urgency != "critical" {
		return nil, false, fmt.Errorf("urgency must be one of: low, normal, critical")
	}

	currentPayload, _ := current["payload"].(map[string]any)
	payload := map[string]any{}
	for k, v := range currentPayload {
		payload[k] = v
	}
	if payload["kind"] == nil {
		payload["kind"] = "systemEvent"
	}

	dataMap := map[string]any{}
	if existingData, ok := payload["data"].(map[string]any); ok {
		for k, v := range existingData {
			dataMap[k] = v
		}
	}

	if title != "" {
		dataMap["summary"] = title
	}
	if message != "" {
		dataMap["body"] = message
	}
	if urgency != "" {
		dataMap["urgency"] = urgency
	}
	payload["data"] = dataMap

	return payload, true, nil
}
