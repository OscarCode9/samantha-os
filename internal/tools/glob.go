package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const globMaxResults = 500

type globTool struct {
	workspaceRoot string
}

// NewGlobTool creates a tool that finds files matching glob patterns.
func NewGlobTool(workspaceRoot string) Tool {
	return &globTool{workspaceRoot: workspaceRoot}
}

func (t *globTool) Name() string { return "glob" }

func (t *globTool) Description() string {
	return "Find files matching a glob pattern. Supports patterns like \"**/*.go\" or \"src/**/*.ts\". Returns matching file paths."
}

func (t *globTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"pattern": {
				Type:        "string",
				Description: "Glob pattern to match files against (e.g. \"**/*.go\", \"src/*.ts\").",
			},
			"path": {
				Type:        "string",
				Description: "Base directory to search from. Defaults to workspace root.",
			},
		},
		Required: []string{"pattern"},
	}
}

func (t *globTool) Execute(ctx context.Context, arguments string) Result {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	baseDir := resolvePath(params.Path, t.workspaceRoot)
	if baseDir == "" {
		baseDir = t.workspaceRoot
	}

	info, err := os.Stat(baseDir)
	if err != nil {
		return ErrorResult(fmt.Sprintf("path error: %s", err))
	}
	if !info.IsDir() {
		return ErrorResult(fmt.Sprintf("%s is not a directory", baseDir))
	}

	// Check if pattern contains **, which requires recursive walking.
	if strings.Contains(params.Pattern, "**") {
		return t.recursiveGlob(baseDir, params.Pattern)
	}

	// Simple glob using filepath.Glob.
	fullPattern := filepath.Join(baseDir, params.Pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return ErrorResult(fmt.Sprintf("glob error: %s", err))
	}

	return formatGlobResults(matches)
}

func (t *globTool) recursiveGlob(baseDir string, pattern string) Result {
	// Extract the file-name part of the pattern (after the last **/).
	// For "**/*.go", we match "*.go" against each file name.
	parts := strings.SplitN(pattern, "**/", 2)
	filePattern := ""
	if len(parts) == 2 {
		filePattern = parts[1]
	}

	var matches []string
	_ = filepath.Walk(baseDir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if fi.IsDir() {
			base := fi.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if len(matches) >= globMaxResults {
			return filepath.SkipAll
		}

		if filePattern != "" {
			matched, _ := filepath.Match(filePattern, fi.Name())
			if !matched {
				return nil
			}
		}
		matches = append(matches, path)
		return nil
	})

	return formatGlobResults(matches)
}

func formatGlobResults(matches []string) Result {
	if len(matches) == 0 {
		return TextResult("no files matched")
	}

	sort.Strings(matches)

	result := strings.Join(matches, "\n")
	if len(matches) >= globMaxResults {
		result += fmt.Sprintf("\n\n(results truncated at %d matches)", globMaxResults)
	} else {
		result += fmt.Sprintf("\n\n(%d files matched)", len(matches))
	}
	return TextResult(result)
}
