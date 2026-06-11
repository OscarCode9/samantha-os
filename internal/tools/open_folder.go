package tools

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	fileManagerBus   = "org.freedesktop.FileManager1"
	fileManagerPath  = "/org/freedesktop/FileManager1"
	fileManagerIface = "org.freedesktop.FileManager1"
)

type openFolderTool struct {
	run commandRunner
}

func NewOpenFolderTool() Tool {
	return &openFolderTool{run: defaultCommandRunner}
}

func (t *openFolderTool) Name() string { return "open_folder" }

func (t *openFolderTool) Description() string {
	return "Open a folder in the Files app (elementary Files / Nautilus). Accepts a local path (e.g. /home/user/Downloads or ~/Documents) or a file:// URI."
}

func (t *openFolderTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"path": {
				Type:        "string",
				Description: "Local folder path (e.g. ~/Downloads, /home/user/Pictures) or file:// URI to open.",
			},
		},
		Required: []string{"path"},
	}
}

func (t *openFolderTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Path string `json:"path"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	path := strings.TrimSpace(params.Path)
	if path == "" {
		return ErrorResult("path must not be empty")
	}

	// Resolve ~ prefix.
	if strings.HasPrefix(path, "~/") || path == "~" {
		home := resolveHomeDir(ctx, t.run)
		if home != "" {
			if path == "~" {
				path = home
			} else {
				path = home + path[1:]
			}
		}
	}

	home := resolveHomeDir(ctx, t.run)
	if home != "" && !strings.HasPrefix(path, "file://") && !strings.HasPrefix(path, "trash://") && !filepath.IsAbs(path) {
		path = filepath.Join(home, path)
	}

	// Ensure file:// URI.
	uri := path
	if !strings.HasPrefix(uri, "file://") && !strings.HasPrefix(uri, "trash://") {
		uri = (&url.URL{Scheme: "file", Path: filepath.Clean(uri)}).String()
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	// ShowFolders(as uris, s startup_id)
	_, err := run(ctx, "busctl", "--user", "call",
		fileManagerBus, fileManagerPath, fileManagerIface,
		"ShowFolders",
		"ass",
		"1",
		uri,
		"",
	)
	if err != nil {
		return ErrorResult(fmt.Sprintf("open folder failed: %v", err))
	}

	return JSONResult(map[string]any{
		"ok":  true,
		"uri": uri,
	})
}

// resolveHomeDir returns the home directory by reading the HOME env via a
// busctl call isn't possible — we exec id / env instead.
func resolveHomeDir(ctx context.Context, run commandRunner) string {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return home
	}
	if run == nil {
		run = defaultCommandRunner
	}
	out, err := run(ctx, "sh", "-c", "echo $HOME")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
