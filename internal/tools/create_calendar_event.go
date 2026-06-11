package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type createCalendarEventTool struct {
	run commandRunner
}

func NewCreateCalendarEventTool() Tool {
	return &createCalendarEventTool{run: defaultCommandRunner}
}

func (t *createCalendarEventTool) Name() string { return "create_calendar_event" }

func (t *createCalendarEventTool) Description() string {
	return "Create an event directly in elementary Calendar (Evolution Data Server backend)."
}

func (t *createCalendarEventTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"title": {
				Type:        "string",
				Description: "Event title.",
			},
			"description": {
				Type:        "string",
				Description: "Event description.",
			},
			"start": {
				Type:        "string",
				Description: "Start datetime in RFC3339 format, for example 2026-04-29T17:00:00-06:00.",
			},
			"duration_minutes": {
				Type:        "integer",
				Description: "Event duration in minutes. Defaults to 60.",
			},
			"timezone": {
				Type:        "string",
				Description: "IANA timezone label for DTSTART/DTEND TZID metadata.",
			},
			"location": {
				Type:        "string",
				Description: "Event location.",
			},
			"daily": {
				Type:        "boolean",
				Description: "When true, creates a daily recurring event.",
			},
		},
		Required: []string{"title", "start"},
	}
}

func (t *createCalendarEventTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Title           string `json:"title"`
		Description     string `json:"description"`
		Start           string `json:"start"`
		DurationMinutes int    `json:"duration_minutes"`
		Timezone        string `json:"timezone"`
		Location        string `json:"location"`
		Daily           bool   `json:"daily"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		return ErrorResult("title must not be empty")
	}

	startRaw := strings.TrimSpace(params.Start)
	if startRaw == "" {
		return ErrorResult("start must not be empty")
	}
	start, err := time.Parse(time.RFC3339, startRaw)
	if err != nil {
		return ErrorResult("start must be RFC3339, for example 2026-04-29T17:00:00-06:00")
	}

	timezone := strings.TrimSpace(params.Timezone)
	if timezone != "" {
		loc, tzErr := time.LoadLocation(timezone)
		if tzErr != nil {
			return ErrorResult("timezone must be a valid IANA zone, for example America/Mexico_City")
		}
		start = start.In(loc)
		timezone = loc.String()
	}

	duration := params.DurationMinutes
	if duration <= 0 {
		duration = 60
	}

	event := calendarEvent{
		UID:         generateCalendarUID(),
		Title:       title,
		Description: strings.TrimSpace(params.Description),
		Location:    strings.TrimSpace(params.Location),
		Start:       start,
		End:         start.Add(time.Duration(duration) * time.Minute),
		Timezone:    timezone,
		Daily:       params.Daily,
	}

	uid, err := createCalendarEvent(ctx, t.run, event)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create calendar event: %v", err))
	}

	return JSONResult(map[string]any{
		"ok":           true,
		"calendar_uid": uid,
		"title":        title,
		"start":        start.Format(time.RFC3339),
		"daily":        params.Daily,
	})
}
