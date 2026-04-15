package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type writeFileTool struct {
	workspaceRoot string
}

// NewWriteFileTool creates a tool that writes content to a file.
func NewWriteFileTool(workspaceRoot string) Tool {
	return &writeFileTool{workspaceRoot: workspaceRoot}
}

func (t *writeFileTool) Name() string { return "write_file" }

func (t *writeFileTool) Description() string {
	return "Write content to a file. Creates the file and any parent directories if they do not exist. Overwrites existing content."
}

func (t *writeFileTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"path": {
				Type:        "string",
				Description: "Absolute or workspace-relative path to the file to write.",
			},
			"content": {
				Type:        "string",
				Description: "The content to write to the file.",
			},
		},
		Required: []string{"path", "content"},
	}
}

func (t *writeFileTool) Execute(ctx context.Context, arguments string) Result {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	filePath := resolvePath(params.Path, t.workspaceRoot)

	// Ensure parent directory exists.
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ErrorResult(fmt.Sprintf("create directory error: %s", err))
	}

	if err := os.WriteFile(filePath, []byte(params.Content), 0o644); err != nil {
		return ErrorResult(fmt.Sprintf("write error: %s", err))
	}

	return TextResult(fmt.Sprintf("wrote %d bytes to %s", len(params.Content), filePath))
}
