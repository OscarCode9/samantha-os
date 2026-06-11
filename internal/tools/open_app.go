package tools

import (
	"context"
	"fmt"
	"strings"
)

type openAppTool struct {
	run commandRunner
}

// NewOpenAppTool creates a tool that launches desktop applications.
func NewOpenAppTool() Tool {
	return &openAppTool{run: defaultCommandRunner}
}

func (t *openAppTool) Name() string { return "open_app" }

func (t *openAppTool) Description() string {
	return "Open a system application. Supported apps: calendar (io.elementary.calendar), mail (io.elementary.mail), files (io.elementary.files), terminal (io.elementary.terminal), settings (io.elementary.switchboard), music (io.elementary.music), browser (epiphany / default browser). Can also accept a custom command or app ID."
}

func (t *openAppTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"app": {
				Type:        "string",
				Description: "The application to open. Use 'calendar', 'mail', 'files', 'terminal', 'settings', 'music', 'browser', or a custom desktop ID / command.",
			},
		},
		Required: []string{"app"},
	}
}

func (t *openAppTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		App string `json:"app"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	app := strings.ToLower(strings.TrimSpace(params.App))
	if app == "" {
		return ErrorResult("app name or ID is required")
	}

	var candidates [][]string
	switch app {
	case "calendar":
		candidates = [][]string{
			{"gtk-launch", "io.elementary.calendar"},
			{"io.elementary.calendar"},
		}
	case "mail":
		candidates = [][]string{
			{"gtk-launch", "io.elementary.mail"},
			{"io.elementary.mail"},
		}
	case "files":
		candidates = [][]string{
			{"gtk-launch", "io.elementary.files"},
			{"io.elementary.files"},
		}
	case "terminal":
		candidates = [][]string{
			{"io.elementary.terminal"},
			{"kgx"},
			{"gnome-terminal"},
		}
	case "settings":
		candidates = [][]string{
			{"gtk-launch", "io.elementary.switchboard"},
			{"io.elementary.switchboard"},
			{"gnome-control-center"},
		}
	case "music":
		candidates = [][]string{
			{"gtk-launch", "io.elementary.music"},
			{"io.elementary.music"},
		}
	case "browser":
		candidates = [][]string{
			{"xdg-open", "http://"},
			{"epiphany"},
		}
	default:
		// If it looks like a desktop ID, try gtk-launch, else run it directly
		if strings.Contains(app, ".") {
			candidates = [][]string{
				{"gtk-launch", app},
				{app},
			}
		} else {
			candidates = [][]string{
				{app},
			}
		}
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	command, err := runDetachedLaunchCandidates(ctx, run, candidates)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to launch app %q: %v", params.App, err))
	}

	return JSONResult(map[string]any{
		"ok":      true,
		"app":     params.App,
		"command": command,
	})
}
