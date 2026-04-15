package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	grepMaxMatches = 200
	grepMaxFiles   = 500
)

type grepSearchTool struct {
	workspaceRoot string
}

// NewGrepSearchTool creates a tool that searches file contents using regular
// expressions.
func NewGrepSearchTool(workspaceRoot string) Tool {
	return &grepSearchTool{workspaceRoot: workspaceRoot}
}

func (t *grepSearchTool) Name() string { return "grep_search" }

func (t *grepSearchTool) Description() string {
	return "Search file contents using a regular expression pattern. Returns matching file paths and line numbers with context."
}

func (t *grepSearchTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"pattern": {
				Type:        "string",
				Description: "Regular expression pattern to search for.",
			},
			"path": {
				Type:        "string",
				Description: "Directory or file to search in. Defaults to workspace root.",
			},
			"include": {
				Type:        "string",
				Description: "Glob pattern to filter files (e.g. \"*.go\", \"*.ts\").",
			},
		},
		Required: []string{"pattern"},
	}
}

func (t *grepSearchTool) Execute(ctx context.Context, arguments string) Result {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	re, err := regexp.Compile(params.Pattern)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid regex: %s", err))
	}

	searchRoot := resolvePath(params.Path, t.workspaceRoot)
	if searchRoot == "" {
		searchRoot = t.workspaceRoot
	}

	info, err := os.Stat(searchRoot)
	if err != nil {
		return ErrorResult(fmt.Sprintf("path error: %s", err))
	}

	var matches []string
	totalMatches := 0

	if !info.IsDir() {
		// Search single file.
		fileMatches := searchFile(searchRoot, re)
		matches = append(matches, fileMatches...)
		totalMatches = len(fileMatches)
	} else {
		// Walk directory.
		filesSearched := 0
		_ = filepath.Walk(searchRoot, func(path string, fi os.FileInfo, walkErr error) error {
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
			if fi.Size() > 1*1024*1024 {
				return nil // skip files > 1MB
			}
			if params.Include != "" {
				matched, _ := filepath.Match(params.Include, fi.Name())
				if !matched {
					return nil
				}
			}
			if filesSearched >= grepMaxFiles {
				return filepath.SkipAll
			}
			filesSearched++

			fileMatches := searchFile(path, re)
			if len(fileMatches) > 0 {
				matches = append(matches, fileMatches...)
				totalMatches += len(fileMatches)
				if totalMatches >= grepMaxMatches {
					return filepath.SkipAll
				}
			}
			return nil
		})
	}

	if len(matches) == 0 {
		return TextResult(fmt.Sprintf("no matches found for pattern %q", params.Pattern))
	}

	result := strings.Join(matches, "\n")
	if totalMatches >= grepMaxMatches {
		result += fmt.Sprintf("\n\n(results truncated at %d matches)", grepMaxMatches)
	}
	return TextResult(result)
}

func searchFile(path string, re *regexp.Regexp) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var matches []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			if len(line) > 200 {
				line = line[:200] + "..."
			}
			matches = append(matches, fmt.Sprintf("%s:%d: %s", path, lineNum, line))
		}
	}
	return matches
}
