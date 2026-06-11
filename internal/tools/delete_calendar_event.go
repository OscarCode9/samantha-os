package tools

import (
	"context"
	"fmt"
	"strings"
)

type deleteCalendarEventTool struct {
	run commandRunner
}

func NewDeleteCalendarEventTool() Tool {
	return &deleteCalendarEventTool{run: defaultCommandRunner}
}

func (t *deleteCalendarEventTool) Name() string { return "delete_calendar_event" }

func (t *deleteCalendarEventTool) Description() string {
	return "Delete a specific calendar event by its UID in elementary Calendar (Evolution Data Server backend)."
}

func (t *deleteCalendarEventTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"calendar_uid": {
				Type:        "string",
				Description: "The UID of the calendar event to delete.",
			},
		},
		Required: []string{"calendar_uid"},
	}
}

func (t *deleteCalendarEventTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		CalendarUID string `json:"calendar_uid"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	uid := strings.TrimSpace(params.CalendarUID)
	if uid == "" {
		return ErrorResult("calendar_uid must not be empty")
	}

	err := deleteCalendarEvent(ctx, t.run, uid)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to delete calendar event: %v", err))
	}

	return JSONResult(map[string]any{
		"ok":           true,
		"calendar_uid": uid,
	})
}
