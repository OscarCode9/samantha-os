package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const notifyDefaultTimeout = 10

type notifyTool struct{}

// NewNotifyTool creates a tool that sends desktop notifications using
// notify-send (Linux/elementary OS). Falls back to osascript on macOS.
func NewNotifyTool() Tool {
	return &notifyTool{}
}

func (t *notifyTool) Name() string { return "notify" }

func (t *notifyTool) Description() string {
	return "Send a desktop notification to the user. Use this to alert the user when a long-running task completes, or when their attention is needed."
}

func (t *notifyTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"summary": {
				Type:        "string",
				Description: "Short title for the notification.",
			},
			"body": {
				Type:        "string",
				Description: "Longer description text for the notification. Optional.",
			},
			"urgency": {
				Type:        "string",
				Description: "Urgency level: low, normal, or critical. Defaults to normal.",
				Enum:        []string{"low", "normal", "critical"},
			},
		},
		Required: []string{"summary"},
	}
}

func (t *notifyTool) Execute(ctx context.Context, arguments string) Result {
	var params struct {
		Summary string `json:"summary"`
		Body    string `json:"body"`
		Urgency string `json:"urgency"`
	}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	summary := strings.TrimSpace(params.Summary)
	if summary == "" {
		return ErrorResult("summary must not be empty")
	}

	urgency := strings.TrimSpace(params.Urgency)
	if urgency == "" {
		urgency = "normal"
	}

	ctx, cancel := context.WithTimeout(ctx, notifyDefaultTimeout*time.Second)
	defer cancel()

	// Try notify-send first (Linux / elementary OS), fall back to osascript (macOS).
	if path, err := exec.LookPath("notify-send"); err == nil {
		return t.executeNotifySend(ctx, path, summary, params.Body, urgency)
	}
	if path, err := exec.LookPath("osascript"); err == nil {
		return t.executeOsascript(ctx, path, summary, params.Body)
	}

	return ErrorResult("no notification command available (need notify-send or osascript)")
}

func (t *notifyTool) executeNotifySend(ctx context.Context, path string, summary string, body string, urgency string) Result {
	args := []string{
		"--urgency=" + urgency,
		"--app-name=elementary-claw",
		summary,
	}
	if body != "" {
		args = append(args, body)
	}

	cmd := exec.CommandContext(ctx, path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ErrorResult(fmt.Sprintf("notify-send failed: %s\n%s", err, string(output)))
	}

	return TextResult(fmt.Sprintf("notification sent: %s", summary))
}

func (t *notifyTool) executeOsascript(ctx context.Context, path string, summary string, body string) Result {
	message := summary
	if body != "" {
		message = body
	}

	// Escape double quotes for AppleScript.
	escapedTitle := strings.ReplaceAll(summary, `"`, `\"`)
	escapedMessage := strings.ReplaceAll(message, `"`, `\"`)

	script := fmt.Sprintf(`display notification "%s" with title "%s"`, escapedMessage, escapedTitle)

	cmd := exec.CommandContext(ctx, path, "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ErrorResult(fmt.Sprintf("osascript failed: %s\n%s", err, string(output)))
	}

	return TextResult(fmt.Sprintf("notification sent: %s", summary))
}
