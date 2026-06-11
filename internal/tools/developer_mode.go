package tools

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type developerModeTool struct {
	run           commandRunner
	workspaceRoot string
}

func NewDeveloperModeTool(workspaceRoot string) Tool {
	return &developerModeTool{
		run:           defaultCommandRunner,
		workspaceRoot: workspaceRoot,
	}
}

func (t *developerModeTool) Name() string { return "developer_mode" }

func (t *developerModeTool) Description() string {
	return "Open a developer workspace for the user. Use when they ask for 'modo programador', 'developer mode', or to open their dev environment. Opens the default browser on a local URL, a terminal in the target folder, VS Code in that folder, and can optionally open the Files app too."
}

func (t *developerModeTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"path": {
				Type:        "string",
				Description: "Project folder or file path. Relative paths are resolved against the workspace root when available.",
			},
			"url": {
				Type:        "string",
				Description: "Local URL to open in the default browser. Defaults to http://localhost:3000.",
			},
			"open_browser": {
				Type:        "boolean",
				Description: "Whether to open the browser. Defaults to true.",
			},
			"open_terminal": {
				Type:        "boolean",
				Description: "Whether to open a terminal in the project folder. Defaults to true.",
			},
			"open_editor": {
				Type:        "boolean",
				Description: "Whether to open VS Code or VSCodium in the project folder. Defaults to true.",
			},
			"open_folder": {
				Type:        "boolean",
				Description: "Whether to also open the folder in the Files app. Defaults to false.",
			},
		},
		Required: []string{"path"},
	}
}

func (t *developerModeTool) Execute(ctx context.Context, arguments string) Result {
	params := struct {
		Path         string `json:"path"`
		URL          string `json:"url"`
		OpenBrowser  *bool  `json:"open_browser"`
		OpenTerminal *bool  `json:"open_terminal"`
		OpenEditor   *bool  `json:"open_editor"`
		OpenFolder   *bool  `json:"open_folder"`
	}{}
	if err := ParseArgs(arguments, &params); err != nil {
		return ErrorResult(err.Error())
	}

	run := t.run
	if run == nil {
		run = defaultCommandRunner
	}

	targetPath := resolveDeveloperModePath(ctx, run, strings.TrimSpace(params.Path), t.workspaceRoot)
	if targetPath == "" {
		return ErrorResult("path must not be empty")
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("target path not found: %s", targetPath))
	}

	workspacePath := targetPath
	editorTarget := targetPath
	if !info.IsDir() {
		workspacePath = filepath.Dir(targetPath)
	}

	openBrowser := true
	if params.OpenBrowser != nil {
		openBrowser = *params.OpenBrowser
	}
	openTerminal := true
	if params.OpenTerminal != nil {
		openTerminal = *params.OpenTerminal
	}
	openEditor := true
	if params.OpenEditor != nil {
		openEditor = *params.OpenEditor
	}
	openFolder := false
	if params.OpenFolder != nil {
		openFolder = *params.OpenFolder
	}

	launchURL := strings.TrimSpace(params.URL)
	if launchURL == "" {
		launchURL = "http://localhost:3000"
	}
	if openBrowser {
		parsedURL, err := url.Parse(launchURL)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return ErrorResult(fmt.Sprintf("invalid URL %q", launchURL))
		}
	}

	steps := map[string]any{
		"path":       workspacePath,
		"editorPath": editorTarget,
		"url":        launchURL,
	}
	var failures []string

	if openBrowser {
		command, err := runDetachedLaunchCandidates(ctx, run, [][]string{
			{"xdg-open", launchURL},
			{"gio", "open", launchURL},
		})
		steps["browser"] = launchStepResult(command, err)
		if err != nil {
			failures = append(failures, "browser: "+err.Error())
		}
	}

	if openTerminal {
		command, err := runDetachedLaunchCandidates(ctx, run, [][]string{
			{"io.elementary.terminal", "--working-directory", workspacePath},
			{"kgx", "--working-directory", workspacePath},
			{"gnome-terminal", "--working-directory=" + workspacePath},
		})
		steps["terminal"] = launchStepResult(command, err)
		if err != nil {
			failures = append(failures, "terminal: "+err.Error())
		}
	}

	if openEditor {
		command, err := runDetachedLaunchCandidates(ctx, run, [][]string{
			{"io.elementary.code", editorTarget},
			{"io.elementary.code", "--new-window", editorTarget},
			{"code", editorTarget},
			{"code", "--new-window", editorTarget},
			{"codium", editorTarget},
			{"codium", "--new-window", editorTarget},
			{"flatpak", "run", "com.visualstudio.code", "--new-window", editorTarget},
			{"flatpak", "run", "com.vscodium.codium", "--new-window", editorTarget},
		})
		steps["editor"] = launchStepResult(command, err)
		if err != nil {
			failures = append(failures, "editor: "+err.Error())
		}
	}

	if openFolder {
		uri := (&url.URL{Scheme: "file", Path: filepath.Clean(workspacePath)}).String()
		command, err := runLaunchCandidates(ctx, run, [][]string{
			{"busctl", "--user", "call", fileManagerBus, fileManagerPath, fileManagerIface, "ShowFolders", "ass", "1", uri, ""},
		})
		steps["files"] = launchStepResult(command, err)
		if err != nil {
			failures = append(failures, "files: "+err.Error())
		}
	}

	steps["ok"] = len(failures) == 0
	if len(failures) > 0 {
		steps["warnings"] = failures
	}
	return JSONResult(steps)
}

func resolveDeveloperModePath(ctx context.Context, run commandRunner, path string, workspaceRoot string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	if strings.HasPrefix(path, "~/") || path == "~" {
		return resolveUserPath(ctx, run, path)
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if strings.TrimSpace(workspaceRoot) != "" {
		return filepath.Clean(filepath.Join(workspaceRoot, path))
	}
	return resolveUserPath(ctx, run, path)
}

func runLaunchCandidates(ctx context.Context, run commandRunner, candidates [][]string) (string, error) {
	if run == nil {
		run = defaultCommandRunner
	}

	attempts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate) == 0 || strings.TrimSpace(candidate[0]) == "" {
			continue
		}
		output, err := run(ctx, candidate[0], candidate[1:]...)
		if err == nil {
			return strings.Join(candidate, " "), nil
		}

		detail := strings.TrimSpace(string(output))
		if detail != "" {
			attempts = append(attempts, fmt.Sprintf("%s: %v (%s)", candidate[0], err, detail))
		} else {
			attempts = append(attempts, fmt.Sprintf("%s: %v", candidate[0], err))
		}
	}

	if len(attempts) == 0 {
		return "", fmt.Errorf("no launch candidates configured")
	}
	return "", errors.New(strings.Join(attempts, "; "))
}

func runDetachedLaunchCandidates(ctx context.Context, run commandRunner, candidates [][]string) (string, error) {
	if run == nil {
		run = defaultCommandRunner
	}

	attempts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate) == 0 || strings.TrimSpace(candidate[0]) == "" {
			continue
		}

		if err := probeLaunchCandidate(ctx, run, candidate); err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: %v", candidate[0], err))
			continue
		}

		displayCommand := strings.Join(candidate, " ")
		shellCommand := "nohup " + shellJoin(candidate) + " >/dev/null 2>&1 </dev/null &"
		output, err := run(ctx, "sh", "-lc", shellCommand)
		if err == nil {
			return displayCommand, nil
		}

		detail := strings.TrimSpace(string(output))
		if detail != "" {
			attempts = append(attempts, fmt.Sprintf("%s: %v (%s)", candidate[0], err, detail))
		} else {
			attempts = append(attempts, fmt.Sprintf("%s: %v", candidate[0], err))
		}
	}

	if len(attempts) == 0 {
		return "", fmt.Errorf("no detached launch candidates configured")
	}
	return "", errors.New(strings.Join(attempts, "; "))
}

func probeLaunchCandidate(ctx context.Context, run commandRunner, candidate []string) error {
	if len(candidate) == 0 {
		return fmt.Errorf("empty candidate")
	}

	if candidate[0] == "flatpak" && len(candidate) >= 3 && candidate[1] == "run" {
		_, err := run(ctx, "flatpak", "info", candidate[2])
		if err != nil {
			return fmt.Errorf("flatpak app %q not available", candidate[2])
		}
		return nil
	}

	_, err := run(ctx, "sh", "-lc", "command -v "+shellQuote(candidate[0])+" >/dev/null 2>&1")
	if err != nil {
		return fmt.Errorf("command not available")
	}
	return nil
}

func shellJoin(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, shellQuote(part))
	}
	return strings.Join(quoted, " ")
}

func launchStepResult(command string, err error) map[string]any {
	payload := map[string]any{
		"ok": err == nil,
	}
	if strings.TrimSpace(command) != "" {
		payload["command"] = command
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	return payload
}
