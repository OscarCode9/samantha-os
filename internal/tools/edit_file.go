package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type editFileTool struct {
	workspaceRoot string
}

// NewEditFileTool creates a tool that performs exact string replacements in files.
func NewEditFileTool(workspaceRoot string) Tool {
	return &editFileTool{workspaceRoot: workspaceRoot}
}

func (t *editFileTool) Name() string { return "edit_file" }

func (t *editFileTool) Description() string {
	return "Edit a file by replacing an exact string with a new string. The oldString must match exactly (including whitespace and indentation). Use replaceAll to replace every occurrence instead of just the first."
}

func (t *editFileTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"path": {
				Type:        "string",
				Description: "Absolute or workspace-relative path to the file to edit.",
			},
			"old_string": {
				Type:        "string",
				Description: "The exact string to find and replace. Must match the file content exactly, including indentation.",
			},
			"new_string": {
				Type:        "string",
				Description: "The replacement string. Can be empty to delete the old_string.",
			},
			"replace_all": {
				Type:        "string",
				Description: "Set to \"true\" to replace all occurrences. Defaults to replacing only the first match.",
			},
		},
		Required: []string{"path", "old_string", "new_string"},
	}
}

func (t *editFileTool) Execute(ctx context.Context, arguments string) Result {
	var params struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll string `json:"replace_all"`
	}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	filePath := resolvePath(params.Path, t.workspaceRoot)

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult(fmt.Sprintf("file not found: %s", filePath))
		}
		return ErrorResult(fmt.Sprintf("stat error: %s", err))
	}
	if info.IsDir() {
		return ErrorResult(fmt.Sprintf("%s is a directory, not a file", filePath))
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("read error: %s", err))
	}

	original := string(content)

	if params.OldString == "" {
		return ErrorResult("old_string must not be empty")
	}

	if params.OldString == params.NewString {
		return ErrorResult("old_string and new_string are identical; nothing to change")
	}

	count := strings.Count(original, params.OldString)
	if count == 0 {
		return ErrorResult("old_string not found in file content")
	}

	if count > 1 && params.ReplaceAll != "true" {
		return ErrorResult(fmt.Sprintf("found %d matches for old_string; set replace_all to \"true\" to replace all, or provide more context to make the match unique", count))
	}

	var updated string
	if params.ReplaceAll == "true" {
		updated = strings.ReplaceAll(original, params.OldString, params.NewString)
	} else {
		updated = strings.Replace(original, params.OldString, params.NewString, 1)
	}

	if err := os.WriteFile(filePath, []byte(updated), info.Mode()); err != nil {
		return ErrorResult(fmt.Sprintf("write error: %s", err))
	}

	replacements := 1
	if params.ReplaceAll == "true" {
		replacements = count
	}

	return TextResult(fmt.Sprintf("edited %s: %d replacement(s) applied", filePath, replacements))
}
