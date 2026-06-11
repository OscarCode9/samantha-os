package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type modifyCalendarEventTool struct {
	run commandRunner
}

func NewModifyCalendarEventTool() Tool {
	return &modifyCalendarEventTool{run: defaultCommandRunner}
}

func (t *modifyCalendarEventTool) Name() string { return "modify_calendar_event" }

func (t *modifyCalendarEventTool) Description() string {
	return "Modify an existing elementary Calendar event by UID. Use list_calendar_events first when you need to locate the event UID."
}

func (t *modifyCalendarEventTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"uid": {
				Type:        "string",
				Description: "Calendar event UID to modify.",
			},
			"title": {
				Type:        "string",
				Description: "Updated event title.",
			},
			"description": {
				Type:        "string",
				Description: "Updated event description. Pass an empty string to clear it.",
			},
			"start": {
				Type:        "string",
				Description: "Updated start datetime in RFC3339 format, for example 2026-04-29T17:00:00-06:00.",
			},
			"duration_minutes": {
				Type:        "integer",
				Description: "Updated duration in minutes. Keeps the existing duration when omitted.",
			},
			"timezone": {
				Type:        "string",
				Description: "Updated IANA timezone label for DTSTART/DTEND TZID metadata. Pass an empty string to switch back to UTC timestamps.",
			},
			"location": {
				Type:        "string",
				Description: "Updated event location. Pass an empty string to clear it.",
			},
			"daily": {
				Type:        "boolean",
				Description: "When true, keeps the event as daily recurring; when false, removes the daily recurrence.",
			},
		},
		Required: []string{"uid"},
	}
}

func (t *modifyCalendarEventTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		UID             string  `json:"uid"`
		Title           *string `json:"title"`
		Description     *string `json:"description"`
		Start           *string `json:"start"`
		DurationMinutes *int    `json:"duration_minutes"`
		Timezone        *string `json:"timezone"`
		Location        *string `json:"location"`
		Daily           *bool   `json:"daily"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	uid := strings.TrimSpace(params.UID)
	if uid == "" {
		return ErrorResult("uid must not be empty")
	}
	if params.Title == nil && params.Description == nil && params.Start == nil && params.DurationMinutes == nil && params.Timezone == nil && params.Location == nil && params.Daily == nil {
		return ErrorResult("at least one field to modify must be provided")
	}

	existingICS, err := getCalendarEvent(ctx, t.run, uid)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to load calendar event: %v", err))
	}
	event, err := parseCalendarEventICS(existingICS)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to parse existing calendar event: %v", err))
	}
	event.UID = uid

	duration := event.End.Sub(event.Start)
	if duration <= 0 {
		if event.AllDay {
			duration = 24 * time.Hour
		} else {
			duration = 60 * time.Minute
		}
	}

	if params.Title != nil {
		title := strings.TrimSpace(*params.Title)
		if title == "" {
			return ErrorResult("title must not be empty when provided")
		}
		event.Title = title
	}
	if params.Description != nil {
		event.Description = strings.TrimSpace(*params.Description)
	}
	if params.Location != nil {
		event.Location = strings.TrimSpace(*params.Location)
	}
	if params.Daily != nil {
		event.Daily = *params.Daily
	}
	if params.Timezone != nil {
		timezone := strings.TrimSpace(*params.Timezone)
		if timezone != "" {
			loc, tzErr := time.LoadLocation(timezone)
			if tzErr != nil {
				return ErrorResult("timezone must be a valid IANA zone, for example America/Mexico_City")
			}
			event.Start = event.Start.In(loc)
			event.End = event.End.In(loc)
			timezone = loc.String()
		}
		event.Timezone = timezone
	}
	if params.DurationMinutes != nil {
		if event.AllDay {
			return ErrorResult("duration_minutes is not supported for all-day events")
		}
		if *params.DurationMinutes <= 0 {
			return ErrorResult("duration_minutes must be greater than 0")
		}
		duration = time.Duration(*params.DurationMinutes) * time.Minute
		event.End = event.Start.Add(duration)
	}
	if params.Start != nil {
		startRaw := strings.TrimSpace(*params.Start)
		if startRaw == "" {
			return ErrorResult("start must not be empty when provided")
		}
		start, parseErr := time.Parse(time.RFC3339, startRaw)
		if parseErr != nil {
			return ErrorResult("start must be RFC3339, for example 2026-04-29T17:00:00-06:00")
		}
		if event.Timezone != "" {
			if loc, tzErr := time.LoadLocation(event.Timezone); tzErr == nil {
				start = start.In(loc)
			}
		}
		event.Start = start
		event.End = start.Add(duration)
	}

	updatedICS := buildEventICS(event)
	if err := modifyCalendarEvent(ctx, t.run, updatedICS); err != nil {
		return ErrorResult(fmt.Sprintf("failed to update calendar event: %v", err))
	}

	return JSONResult(map[string]any{
		"ok":           true,
		"calendar_uid": uid,
		"title":        event.Title,
		"start":        event.Start.Format(time.RFC3339),
		"daily":        event.Daily,
		"all_day":      event.AllDay,
	})
}