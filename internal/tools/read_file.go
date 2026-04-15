package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

const readFileDefaultLimit = 2000

type readFileTool struct {
	workspaceRoot string
}

// NewReadFileTool creates a tool that reads file contents with optional
// offset and line limit.
func NewReadFileTool(workspaceRoot string) Tool {
	return &readFileTool{workspaceRoot: workspaceRoot}
}

func (t *readFileTool) Name() string { return "read_file" }

func (t *readFileTool) Description() string {
	return "Read the contents of a file. Returns numbered lines. Use offset and limit to read specific sections of large files."
}

func (t *readFileTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"path": {
				Type:        "string",
				Description: "Absolute or workspace-relative path to the file to read.",
			},
			"offset": {
				Type:        "number",
				Description: "1-based line number to start reading from. Defaults to 1.",
			},
			"limit": {
				Type:        "number",
				Description: "Maximum number of lines to return. Defaults to 2000.",
			},
		},
		Required: []string{"path"},
	}
}

func (t *readFileTool) Execute(ctx context.Context, arguments string) Result {
	var params struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
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
		return ErrorResult(fmt.Sprintf("%s is a directory, use list_dir instead", filePath))
	}

	offset := params.Offset
	if offset < 1 {
		offset = 1
	}
	limit := params.Limit
	if limit <= 0 {
		limit = readFileDefaultLimit
	}

	file, err := os.Open(filePath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("open error: %s", err))
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024) // handle long lines
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if len(lines) >= limit {
			break
		}
		line := scanner.Text()
		// Truncate extremely long lines.
		if len(line) > 2000 {
			line = line[:2000] + "...(truncated)"
		}
		lines = append(lines, fmt.Sprintf("%d: %s", lineNum, line))
	}
	if err := scanner.Err(); err != nil {
		return ErrorResult(fmt.Sprintf("read error: %s", err))
	}

	if len(lines) == 0 {
		if offset > 1 {
			return TextResult(fmt.Sprintf("(no lines at offset %d, file has %d lines)", offset, lineNum))
		}
		return TextResult("(empty file)")
	}

	result := strings.Join(lines, "\n")
	totalLines := lineNum
	if len(lines) == limit && lineNum < totalLines {
		result += fmt.Sprintf("\n\n(showing lines %d-%d of %d total)", offset, offset+len(lines)-1, totalLines)
	}
	return TextResult(result)
}
