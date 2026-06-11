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

type createDailyNotificationJobWithCalendarTool struct {
	baseURL string
	client  *http.Client
	run     commandRunner
}

func NewCreateDailyNotificationJobWithCalendarTool(baseURL string) Tool {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		trimmed = defaultCronBaseURL
	}
	return &createDailyNotificationJobWithCalendarTool{
		baseURL: strings.TrimRight(trimmed, "/"),
		client:  &http.Client{Timeout: 20 * time.Second},
		run:     defaultCommandRunner,
	}
}

func (t *createDailyNotificationJobWithCalendarTool) Name() string {
	return "create_daily_notification_job_with_calendar"
}

func (t *createDailyNotificationJobWithCalendarTool) Description() string {
	return "Create a daily reminder cron job and mirror it as a recurring event in elementary Calendar."
}

func (t *createDailyNotificationJobWithCalendarTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"message": {
				Type:        "string",
				Description: "Notification body text.",
			},
			"title": {
				Type:        "string",
				Description: "Notification title. Defaults to Samantha.",
			},
			"time": {
				Type:        "string",
				Description: "Daily local time in HH:MM 24h format. Defaults to 17:00.",
			},
			"timezone": {
				Type:        "string",
				Description: "IANA timezone label for cron metadata and calendar TZID.",
			},
			"name": {
				Type:        "string",
				Description: "Cron job display name. Defaults to Daily notification.",
			},
			"urgency": {
				Type:        "string",
				Description: "Notification urgency.",
				Enum:        []string{"low", "normal", "critical"},
			},
		},
		Required: []string{"message"},
	}
}

func (t *createDailyNotificationJobWithCalendarTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Message  string `json:"message"`
		Title    string `json:"title"`
		Time     string `json:"time"`
		Timezone string `json:"timezone"`
		Name     string `json:"name"`
		Urgency  string `json:"urgency"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	message := strings.TrimSpace(params.Message)
	if message == "" {
		return ErrorResult("message must not be empty")
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		title = "Samantha"
	}

	name := strings.TrimSpace(params.Name)
	if name == "" {
		name = "Daily notification"
	}

	hour, minute, err := parseClock(strings.TrimSpace(params.Time))
	if err != nil {
		return ErrorResult(err.Error())
	}

	urgency := strings.TrimSpace(strings.ToLower(params.Urgency))
	if urgency == "" {
		urgency = "normal"
	}
	if urgency != "low" && urgency != "normal" && urgency != "critical" {
		return ErrorResult("urgency must be one of: low, normal, critical")
	}

	timezone := strings.TrimSpace(params.Timezone)
	loc := time.Now().Location()
	if timezone != "" {
		loaded, tzErr := time.LoadLocation(timezone)
		if tzErr != nil {
			return ErrorResult("timezone must be a valid IANA zone, for example America/Mexico_City")
		}
		loc = loaded
		timezone = loaded.String()
	} else {
		if localName := strings.TrimSpace(loc.String()); localName != "" && !strings.EqualFold(localName, "Local") {
			timezone = localName
		}
	}

	calendarUID := generateCalendarUID()
	cronInput := map[string]any{
		"name": name,
		"schedule": map[string]any{
			"kind":     "cron",
			"expr":     fmt.Sprintf("%d %d * * *", minute, hour),
			"timezone": timezone,
		},
		"payload": map[string]any{
			"kind": "systemEvent",
			"data": map[string]any{
				"summary":            title,
				"body":               message,
				"urgency":            urgency,
				"calendar_event_uid": calendarUID,
			},
		},
		"deliveryMode": "announce",
	}

	body, err := json.Marshal(cronInput)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to marshal cron input: %v", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/cron/jobs/create", bytes.NewReader(body))
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create cron request: %v", err))
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

	cronJob := map[string]any{}
	if err := json.Unmarshal(responseBody.Bytes(), &cronJob); err != nil {
		return ErrorResult(fmt.Sprintf("invalid cron API response: %v", err))
	}

	start := time.Now().In(loc).AddDate(0, 0, 1)
	start = time.Date(start.Year(), start.Month(), start.Day(), hour, minute, 0, 0, loc)

	calendarEvent := calendarEvent{
		UID:         calendarUID,
		Title:       title,
		Description: message,
		Start:       start,
		End:         start.Add(30 * time.Minute),
		Timezone:    timezone,
		Daily:       true,
	}
	createdCalendarUID, calErr := createCalendarEvent(ctx, t.run, calendarEvent)

	result := map[string]any{
		"cron":             cronJob,
		"calendarEventUID": createdCalendarUID,
	}
	if calErr != nil {
		result["calendarSyncError"] = calErr.Error()
	}

	return JSONResult(result)
}
