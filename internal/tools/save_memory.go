package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/config"
	clawmemory "github.com/oscarcode/elementary-claw/internal/memory"
)

type saveMemoryTool struct {
	workspaceRoot string
}

// NewSaveMemoryTool creates a tool dedicated to persistent user memory.
func NewSaveMemoryTool(workspaceRoot string) Tool {
	return &saveMemoryTool{workspaceRoot: workspaceRoot}
}

func (t *saveMemoryTool) Name() string { return "save_memory" }

func (t *saveMemoryTool) Description() string {
	return "Save a durable user memory in the local categorized memory store. Use this when the user asks you to remember a preference, important folder, frequent app, routine, blocked action, or a daily note."
}

func (t *saveMemoryTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"category": {
				Type:        "string",
				Description: "Memory category to save into.",
				Enum:        clawmemory.Categories(),
			},
			"content": {
				Type:        "string",
				Description: "The durable fact or instruction to remember.",
			},
		},
		Required: []string{"category", "content"},
	}
}

func (t *saveMemoryTool) Execute(ctx context.Context, arguments string) Result {
	var params struct {
		Category string `json:"category"`
		Content  string `json:"content"`
	}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	if strings.TrimSpace(params.Category) == "" {
		return ErrorResult("category must not be empty")
	}
	if strings.TrimSpace(params.Content) == "" {
		return ErrorResult("content must not be empty")
	}

	paths := config.Paths{WorkspaceDir: t.workspaceRoot}
	result, err := clawmemory.Save(paths, params.Category, params.Content)
	if err != nil {
		return ErrorResult(fmt.Sprintf("save memory failed: %v", err))
	}

	return JSONResult(map[string]any{
		"ok":        true,
		"category":  result.Category,
		"path":      result.Path,
		"content":   result.Content,
		"duplicate": result.Duplicate,
	})
}