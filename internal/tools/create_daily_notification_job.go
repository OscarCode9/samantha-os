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

const defaultCronBaseURL = "http://127.0.0.1:4389"

type createDailyNotificationJobTool struct {
	baseURL string
	client  *http.Client
}

// NewCreateDailyNotificationJobTool creates a tool that schedules a daily
// system notification job through the local cron API.
func NewCreateDailyNotificationJobTool(baseURL string) Tool {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		trimmed = defaultCronBaseURL
	}
	return &createDailyNotificationJobTool{
		baseURL: strings.TrimRight(trimmed, "/"),
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *createDailyNotificationJobTool) Name() string { return "create_daily_notification_job" }

func (t *createDailyNotificationJobTool) Description() string {
	return "Create a daily cron job that sends a desktop system notification. Defaults to 17:00 every day."
}

func (t *createDailyNotificationJobTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"message": {
				Type:        "string",
				Description: "Notification body text.",
			},
			"title": {
				Type:        "string",
				Description: "Notification title. Defaults to 'Samantha'.",
			},
			"time": {
				Type:        "string",
				Description: "Daily local time in 24h format HH:MM. Defaults to 17:00.",
			},
			"timezone": {
				Type:        "string",
				Description: "IANA timezone for metadata (for example America/Mexico_City).",
			},
			"urgency": {
				Type:        "string",
				Description: "Notification urgency.",
				Enum:        []string{"low", "normal", "critical"},
			},
			"name": {
				Type:        "string",
				Description: "Cron job display name. Defaults to 'Daily notification'.",
			},
		},
		Required: []string{"message"},
	}
}

func (t *createDailyNotificationJobTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Message  string `json:"message"`
		Title    string `json:"title"`
		Time     string `json:"time"`
		Timezone string `json:"timezone"`
		Urgency  string `json:"urgency"`
		Name     string `json:"name"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	bodyText := strings.TrimSpace(params.Message)
	if bodyText == "" {
		return ErrorResult("message must not be empty")
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		title = "Samantha"
	}

	jobName := strings.TrimSpace(params.Name)
	if jobName == "" {
		jobName = "Daily notification"
	}

	hour, minute, err := parseClock(strings.TrimSpace(params.Time))
	if err != nil {
		return ErrorResult(err.Error())
	}
	cronExpr := fmt.Sprintf("%d %d * * *", minute, hour)

	urgency := strings.TrimSpace(strings.ToLower(params.Urgency))
	if urgency == "" {
		urgency = "normal"
	}
	if urgency != "low" && urgency != "normal" && urgency != "critical" {
		return ErrorResult("urgency must be one of: low, normal, critical")
	}

	timezone := strings.TrimSpace(params.Timezone)
	if timezone != "" {
		loaded, tzErr := time.LoadLocation(timezone)
		if tzErr != nil {
			return ErrorResult("timezone must be a valid IANA zone, for example America/Mexico_City")
		}
		timezone = loaded.String()
	} else {
		localName := strings.TrimSpace(time.Now().Location().String())
		if localName != "" && !strings.EqualFold(localName, "Local") {
			timezone = localName
		}
	}

	input := map[string]any{
		"name": jobName,
		"schedule": map[string]any{
			"kind":     "cron",
			"expr":     cronExpr,
			"timezone": timezone,
		},
		"payload": map[string]any{
			"kind": "systemEvent",
			"data": map[string]any{
				"summary": title,
				"body":    bodyText,
				"urgency": urgency,
			},
		},
		"deliveryMode": "announce",
	}

	data, err := json.Marshal(input)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to marshal request: %v", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/cron/jobs/create", bytes.NewReader(data))
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

	result["scheduledAt"] = fmt.Sprintf("%02d:%02d", hour, minute)
	result["scheduleExpr"] = cronExpr
	if timezone != "" {
		result["timezone"] = timezone
	}

	return JSONResult(result)
}

func parseClock(value string) (int, int, error) {
	if value == "" {
		return 17, 0, nil
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("time must be HH:MM (24h)")
	}
	var hour, minute int
	if _, err := fmt.Sscanf(value, "%d:%d", &hour, &minute); err != nil {
		return 0, 0, fmt.Errorf("time must be HH:MM (24h)")
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("time out of range: expected HH:MM in 24h")
	}
	return hour, minute, nil
}
