package tools

import (
	"context"
	"strings"
)

type listTasksTool struct {
	run commandRunner
}

func NewListTasksTool() Tool {
	return &listTasksTool{run: defaultCommandRunner}
}

func (t *listTasksTool) Name() string { return "list_tasks" }

func (t *listTasksTool) Description() string {
	return "List tasks currently stored in elementary Tasks."
}

func (t *listTasksTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"summary_contains": {
				Type:        "string",
				Description: "Optional case-insensitive title filter.",
			},
			"include_completed": {
				Type:        "boolean",
				Description: "When true, includes completed tasks.",
			},
		},
	}
}

func (t *listTasksTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		SummaryContains  string `json:"summary_contains"`
		IncludeCompleted bool   `json:"include_completed"`
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

	icsObjects, err := listTaskItems(ctx, t.run, query)
	if err != nil {
		return ErrorResult(err.Error())
	}

	items := make([]map[string]any, 0, len(icsObjects))
	for _, ics := range icsObjects {
		status := strings.TrimSpace(parseICSField(ics, "STATUS"))
		isCompleted := strings.EqualFold(status, "COMPLETED")
		if !params.IncludeCompleted && isCompleted {
			continue
		}

		items = append(items, map[string]any{
			"uid":         parseICSField(ics, "UID"),
			"summary":     parseICSField(ics, "SUMMARY"),
			"description": parseICSField(ics, "DESCRIPTION"),
			"due":         parseICSField(ics, "DUE"),
			"status":      status,
			"completed":   parseICSField(ics, "COMPLETED"),
		})
	}

	return JSONResult(map[string]any{
		"count": len(items),
		"tasks": items,
	})
}
