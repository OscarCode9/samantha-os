package tools

import (
	"context"
	"fmt"
	"strings"
)

type completeTaskTool struct {
	run commandRunner
}

func NewCompleteTaskTool() Tool {
	return &completeTaskTool{run: defaultCommandRunner}
}

func (t *completeTaskTool) Name() string { return "complete_task" }

func (t *completeTaskTool) Description() string {
	return "Mark a task in elementary Tasks as completed (or reopen it)."
}

func (t *completeTaskTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"uid": {
				Type:        "string",
				Description: "Task UID.",
			},
			"completed": {
				Type:        "boolean",
				Description: "When false, reopens the task. Defaults to true.",
			},
		},
		Required: []string{"uid"},
	}
}

func (t *completeTaskTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		UID       string `json:"uid"`
		Completed *bool  `json:"completed"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	uid := strings.TrimSpace(params.UID)
	if uid == "" {
		return ErrorResult("uid must not be empty")
	}

	completed := true
	if params.Completed != nil {
		completed = *params.Completed
	}

	ics, err := getTaskItem(ctx, t.run, uid)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to load task: %v", err))
	}
	updated := setTaskCompleted(ics, completed)
	if err := modifyTaskItem(ctx, t.run, updated); err != nil {
		return ErrorResult(fmt.Sprintf("failed to update task: %v", err))
	}

	return JSONResult(map[string]any{
		"ok":        true,
		"uid":       uid,
		"completed": completed,
	})
}
