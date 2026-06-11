package tools

import (
	"context"
	"fmt"
	"strings"
)

type deleteTaskTool struct {
	run commandRunner
}

func NewDeleteTaskTool() Tool {
	return &deleteTaskTool{run: defaultCommandRunner}
}

func (t *deleteTaskTool) Name() string { return "delete_task" }

func (t *deleteTaskTool) Description() string {
	return "Delete a task from elementary Tasks by UID."
}

func (t *deleteTaskTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"uid": {
				Type:        "string",
				Description: "Task UID.",
			},
		},
		Required: []string{"uid"},
	}
}

func (t *deleteTaskTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		UID string `json:"uid"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	uid := strings.TrimSpace(params.UID)
	if uid == "" {
		return ErrorResult("uid must not be empty")
	}

	if err := deleteTaskItem(ctx, t.run, uid); err != nil {
		return ErrorResult(fmt.Sprintf("failed to delete task: %v", err))
	}

	return JSONResult(map[string]any{
		"ok":  true,
		"uid": uid,
	})
}
