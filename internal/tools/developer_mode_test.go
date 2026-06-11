package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDeveloperModeToolLaunchesBrowserTerminalAndEditor(t *testing.T) {
	workspace := t.TempDir()
	projectDir := filepath.Join(workspace, "apps", "web")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	tool := &developerModeTool{
		workspaceRoot: workspace,
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			call := append([]string{name}, args...)
			calls = append(calls, call)
			switch name {
			case "xdg-open":
				return []byte("ok"), nil
			case "sh":
				if len(args) == 2 && args[0] == "-lc" {
					return []byte("ok"), nil
				}
				return []byte("ok"), nil
			default:
				return nil, fmt.Errorf("unexpected command: %s", name)
			}
		},
	}

	result := tool.Execute(context.Background(), `{"path":"apps/web"}`)
	payload := decodeToolJSONResult(t, result)

	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", payload)
	}
	if payload["path"] != projectDir {
		t.Fatalf("unexpected workspace path: %#v", payload["path"])
	}
	if payload["editorPath"] != projectDir {
		t.Fatalf("unexpected editor path: %#v", payload["editorPath"])
	}
	if payload["url"] != "http://localhost:3000" {
		t.Fatalf("unexpected URL: %#v", payload["url"])
	}

	wantCalls := [][]string{
		{"sh", "-lc", "command -v 'xdg-open' >/dev/null 2>&1"},
		{"sh", "-lc", "nohup 'xdg-open' 'http://localhost:3000' >/dev/null 2>&1 </dev/null &"},
		{"sh", "-lc", "command -v 'io.elementary.terminal' >/dev/null 2>&1"},
		{"sh", "-lc", "nohup 'io.elementary.terminal' '--working-directory' '" + projectDir + "' >/dev/null 2>&1 </dev/null &"},
		{"sh", "-lc", "command -v 'io.elementary.code' >/dev/null 2>&1"},
		{"sh", "-lc", "nohup 'io.elementary.code' '" + projectDir + "' >/dev/null 2>&1 </dev/null &"},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected commands:\n got %#v\nwant %#v", calls, wantCalls)
	}
}

func TestDeveloperModeToolUsesParentFolderForTerminalWhenGivenAFile(t *testing.T) {
	workspace := t.TempDir()
	projectDir := filepath.Join(workspace, "apps", "api")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(projectDir, "server.ts")
	if err := os.WriteFile(targetFile, []byte("console.log('hi')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	tool := &developerModeTool{
		workspaceRoot: workspace,
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			call := append([]string{name}, args...)
			calls = append(calls, call)
			switch name {
			case "xdg-open", "busctl":
				return []byte("ok"), nil
			case "sh":
				if len(args) == 2 && args[0] == "-lc" {
					return []byte("ok"), nil
				}
				return []byte("ok"), nil
			default:
				return nil, fmt.Errorf("unexpected command: %s", name)
			}
		},
	}

	result := tool.Execute(context.Background(), `{"path":"apps/api/server.ts","open_folder":true}`)
	payload := decodeToolJSONResult(t, result)

	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", payload)
	}
	if payload["path"] != projectDir {
		t.Fatalf("unexpected workspace path: %#v", payload["path"])
	}
	if payload["editorPath"] != targetFile {
		t.Fatalf("unexpected editor path: %#v", payload["editorPath"])
	}

	wantCalls := [][]string{
		{"sh", "-lc", "command -v 'xdg-open' >/dev/null 2>&1"},
		{"sh", "-lc", "nohup 'xdg-open' 'http://localhost:3000' >/dev/null 2>&1 </dev/null &"},
		{"sh", "-lc", "command -v 'io.elementary.terminal' >/dev/null 2>&1"},
		{"sh", "-lc", "nohup 'io.elementary.terminal' '--working-directory' '" + projectDir + "' >/dev/null 2>&1 </dev/null &"},
		{"sh", "-lc", "command -v 'io.elementary.code' >/dev/null 2>&1"},
		{"sh", "-lc", "nohup 'io.elementary.code' '" + targetFile + "' >/dev/null 2>&1 </dev/null &"},
		{"busctl", "--user", "call", fileManagerBus, fileManagerPath, fileManagerIface, "ShowFolders", "ass", "1", "file://" + projectDir, ""},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected commands:\n got %#v\nwant %#v", calls, wantCalls)
	}
}
