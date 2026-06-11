package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	screenshotBus   = "org.gnome.Shell.Screenshot"
	screenshotPath  = "/org/gnome/Shell/Screenshot"
	screenshotIface = "org.gnome.Shell.Screenshot"
)

type takeScreenshotTool struct {
	run commandRunner
}

func NewTakeScreenshotTool() Tool {
	return &takeScreenshotTool{run: defaultCommandRunner}
}

func (t *takeScreenshotTool) Name() string { return "take_screenshot" }

func (t *takeScreenshotTool) Description() string {
	return "Take a full-screen screenshot using GNOME Shell and save it to ~/Pictures. Returns the file path of the saved screenshot."
}

func (t *takeScreenshotTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"include_cursor": {
				Type:        "boolean",
				Description: "Whether to include the mouse cursor in the screenshot. Defaults to false.",
			},
			"filename": {
				Type:        "string",
				Description: "Optional output filename (without path). Saved under ~/Pictures. Defaults to a timestamp-based name.",
			},
		},
	}
}

func (t *takeScreenshotTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		IncludeCursor bool   `json:"include_cursor"`
		Filename      string `json:"filename"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	// Resolve destination path.
	home := resolveHomeDir(ctx, run)
	if home == "" {
		home = "/tmp"
	}

	filename := strings.TrimSpace(params.Filename)
	if filename == "" {
		filename = "screenshot-" + time.Now().Format("2006-01-02_15-04-05") + ".png"
	}
	filename = filepath.Base(filename)
	if filename == "." || filename == string(filepath.Separator) {
		return ErrorResult("filename must be a file name, not a path")
	}
	if !strings.HasSuffix(filename, ".png") {
		filename += ".png"
	}

	destPath := home + "/Pictures/" + filename

	// Ensure Pictures directory exists.
	_, _ = run(ctx, "mkdir", "-p", home+"/Pictures")

	includeCursor := "false"
	if params.IncludeCursor {
		includeCursor = "true"
	}

	// Screenshot(include_cursor bool, flash bool, filename string) → (success bool, filename_used string)
	out, err := run(ctx, "busctl", "--user", "--json=short", "call",
		screenshotBus, screenshotPath, screenshotIface,
		"Screenshot",
		"bbs",
		includeCursor,
		"false",
		destPath,
	)
	if err != nil {
		return ErrorResult(fmt.Sprintf("screenshot failed: %v", err))
	}

	// Response: {"type":"(bs)","data":[true, "/actual/path.png"]}
	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil || len(resp.Data) < 2 {
		return ErrorResult("unexpected screenshot response")
	}

	var success bool
	var actualPath string
	_ = json.Unmarshal(resp.Data[0], &success)
	_ = json.Unmarshal(resp.Data[1], &actualPath)

	if !success {
		return ErrorResult("screenshot was not saved (GNOME Shell returned failure)")
	}

	return imageAttachmentResult(actualPath, "image/png")
}
