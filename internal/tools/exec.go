package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	execDefaultTimeout = 30
	execMaxOutput      = 50000
)

// ExecToolOptions configures the exec tool.
type ExecToolOptions struct {
	// DefaultWorkdir is the working directory used when the caller does not
	// specify one. If empty, the process's current directory is used.
	DefaultWorkdir string
	// MaxOutputBytes caps the combined stdout+stderr returned to the model.
	MaxOutputBytes int
}

type execTool struct {
	defaultWorkdir string
	maxOutput      int
}

// NewExecTool creates a tool that executes shell commands.
func NewExecTool(opts ExecToolOptions) Tool {
	maxOutput := opts.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = execMaxOutput
	}
	return &execTool{
		defaultWorkdir: opts.DefaultWorkdir,
		maxOutput:      maxOutput,
	}
}

func (t *execTool) Name() string { return "exec" }

func (t *execTool) Description() string {
	return "Execute a shell command and return its output. Use this to run programs, scripts, or system commands. The command is executed via /bin/sh -c."
}

func (t *execTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"command": {
				Type:        "string",
				Description: "The shell command to execute.",
			},
			"workdir": {
				Type:        "string",
				Description: "Working directory for the command. Defaults to the workspace root.",
			},
			"timeout": {
				Type:        "number",
				Description: "Timeout in seconds. Defaults to 30.",
			},
		},
		Required: []string{"command"},
	}
}

func (t *execTool) Execute(ctx context.Context, arguments string) Result {
	var params struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
		Timeout int    `json:"timeout"`
	}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	if strings.TrimSpace(params.Command) == "" {
		return ErrorResult("command must not be empty")
	}

	workdir := params.Workdir
	if workdir == "" {
		workdir = t.defaultWorkdir
	}

	timeout := params.Timeout
	if timeout <= 0 {
		timeout = execDefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", params.Command)
	if workdir != "" {
		cmd.Dir = workdir
	}

	output, err := cmd.CombinedOutput()
	text := TruncateMiddle(string(output), t.maxOutput)

	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if ctx.Err() == context.DeadlineExceeded {
			return ErrorResult(fmt.Sprintf("%s\n\ntimed out after %d seconds", text, timeout))
		}
		return ErrorResult(fmt.Sprintf("%s\nexit code: %d", text, exitCode))
	}

	if text == "" {
		text = "(no output)"
	}
	return TextResult(text)
}
