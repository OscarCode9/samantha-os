package tools

import (
	"context"
	"strings"
)

type listCalendarEventsTool struct {
	run commandRunner
}

func NewListCalendarEventsTool() Tool {
	return &listCalendarEventsTool{run: defaultCommandRunner}
}

func (t *listCalendarEventsTool) Name() string { return "list_calendar_events" }

func (t *listCalendarEventsTool) Description() string {
	return "List events currently stored in elementary Calendar."
}

func (t *listCalendarEventsTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"summary_contains": {
				Type:        "string",
				Description: "Optional case-insensitive summary filter.",
			},
		},
	}
}

func (t *listCalendarEventsTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		SummaryContains string `json:"summary_contains"`
	}{}
	if strings.TrimSpace(arguments) != "" {
		if err := ParseArgs(arguments, &params); err != nil {
			return ErrorResult(err.Error())
		}
	}

	query := "#t"
	if contains := strings.TrimSpace(params.SummaryContains); contains != "" {
		query = "(contains? \"summary\" \"" + strings.ReplaceAll(contains, "\"", "") + "\")"
	}

	icsObjects, err := listCalendarEvents(ctx, t.run, query)
	if err != nil {
		return ErrorResult(err.Error())
	}

	items := make([]map[string]any, 0, len(icsObjects))
	for _, ics := range icsObjects {
		items = append(items, map[string]any{
			"uid":     parseICSField(ics, "UID"),
			"summary": parseICSField(ics, "SUMMARY"),
			"dtstart": parseICSField(ics, "DTSTART"),
			"dtend":   parseICSField(ics, "DTEND"),
		})
	}

	return JSONResult(map[string]any{
		"count":  len(items),
		"events": items,
	})
}
