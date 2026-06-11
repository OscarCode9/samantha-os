package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type trashFileTool struct {
	run commandRunner
}

func NewTrashFileTool() Tool {
	return &trashFileTool{run: defaultCommandRunner}
}

func (t *trashFileTool) Name() string { return "trash_file" }

func (t *trashFileTool) Description() string {
	return "Move a local file or folder to the desktop trash using gio trash. This is reversible from the file manager trash."
}

func (t *trashFileTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"path": {
				Type:        "string",
				Description: "Local file or folder path to move to trash. Supports absolute paths, ~/ paths, and paths relative to the user's home.",
			},
		},
		Required: []string{"path"},
	}
}

func (t *trashFileTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Path string `json:"path"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	path := resolveUserPath(ctx, run, params.Path)
	if path == "" {
		return ErrorResult("path must not be empty")
	}
	if path == string(filepath.Separator) {
		return ErrorResult("refusing to trash filesystem root")
	}

	out, err := run(ctx, "gio", "trash", path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("trash failed: %v (%s)", err, strings.TrimSpace(string(out))))
	}

	return JSONResult(map[string]any{
		"ok":   true,
		"path": path,
	})
}

func resolveUserPath(ctx context.Context, run commandRunner, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	home := resolveHomeDir(ctx, run)
	if strings.HasPrefix(path, "~/") || path == "~" {
		if home == "" {
			return filepath.Clean(path)
		}
		if path == "~" {
			return home
		}
		return filepath.Clean(home + path[1:])
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if home != "" {
		return filepath.Clean(filepath.Join(home, path))
	}
	return filepath.Clean(path)
}
