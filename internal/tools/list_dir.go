package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type listDirTool struct {
	workspaceRoot string
}

// NewListDirTool creates a tool that lists directory contents.
func NewListDirTool(workspaceRoot string) Tool {
	return &listDirTool{workspaceRoot: workspaceRoot}
}

func (t *listDirTool) Name() string { return "list_dir" }

func (t *listDirTool) Description() string {
	return "List the contents of a directory. Returns one entry per line. Directories have a trailing /."
}

func (t *listDirTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"path": {
				Type:        "string",
				Description: "Absolute or workspace-relative path to the directory to list.",
			},
		},
		Required: []string{"path"},
	}
}

func (t *listDirTool) Execute(ctx context.Context, arguments string) Result {
	var params struct {
		Path string `json:"path"`
	}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	dirPath := resolvePath(params.Path, t.workspaceRoot)

	info, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult(fmt.Sprintf("directory not found: %s", dirPath))
		}
		return ErrorResult(fmt.Sprintf("stat error: %s", err))
	}
	if !info.IsDir() {
		return ErrorResult(fmt.Sprintf("%s is a file, not a directory", dirPath))
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("read directory error: %s", err))
	}

	// Sort alphabetically, directories first.
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return entries[i].Name() < entries[j].Name()
	})

	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}

	if len(lines) == 0 {
		return TextResult("(empty directory)")
	}

	result := strings.Join(lines, "\n")
	result += fmt.Sprintf("\n\n(%d entries in %s)", len(lines), filepath.Clean(dirPath))
	return TextResult(result)
}
