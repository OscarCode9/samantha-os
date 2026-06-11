package tools

import (
	"context"
	"fmt"
	"strings"
)

type createTaskTool struct {
	run commandRunner
}

func NewCreateTaskTool() Tool {
	return &createTaskTool{run: defaultCommandRunner}
}

func (t *createTaskTool) Name() string { return "create_task" }

func (t *createTaskTool) Description() string {
	return "Create a task in elementary Tasks (Evolution Data Server task list)."
}

func (t *createTaskTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"title": {
				Type:        "string",
				Description: "Task title.",
			},
			"description": {
				Type:        "string",
				Description: "Optional task description.",
			},
			"due": {
				Type:        "string",
				Description: "Optional due datetime in RFC3339 format, for example 2026-04-29T17:00:00-06:00.",
			},
			"timezone": {
				Type:        "string",
				Description: "Optional IANA timezone for due metadata, for example America/Mexico_City.",
			},
		},
		Required: []string{"title"},
	}
}

func (t *createTaskTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Due         string `json:"due"`
		Timezone    string `json:"timezone"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		return ErrorResult("title must not be empty")
	}

	ics, err := buildTaskICS("", title, params.Description, params.Due, params.Timezone)
	if err != nil {
		return ErrorResult(err.Error())
	}

	uid, err := createTaskItem(ctx, t.run, ics)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create task: %v", err))
	}

	return JSONResult(map[string]any{
		"ok":          true,
		"uid":         uid,
		"title":       title,
		"due":         strings.TrimSpace(params.Due),
		"timezone":    strings.TrimSpace(params.Timezone),
		"description": strings.TrimSpace(params.Description),
	})
}
