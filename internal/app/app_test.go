package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oscarcode/elementary-claw/internal/config"
)

func TestBuildToolRegistryDefaultsToWorkspaceDir(t *testing.T) {
	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	workspaceDir := filepath.Join(root, "workspace")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	app := &App{
		paths: config.Paths{
			HomeDir:      homeDir,
			WorkspaceDir: workspaceDir,
		},
	}

	registry := app.buildToolRegistry("")
	tool, ok := registry.Get("write_file")
	if !ok {
		t.Fatal("write_file tool not registered")
	}

	result := tool.Execute(context.Background(), `{"path":"memory/notes.md","content":"hello workspace memory"}`)
	if result.IsError {
		t.Fatalf("unexpected write_file error: %s", result.Content)
	}

	workspacePath := filepath.Join(workspaceDir, "memory", "notes.md")
	if _, err := os.Stat(workspacePath); err != nil {
		t.Fatalf("expected memory file in workspace: %v", err)
	}

	homePath := filepath.Join(homeDir, "memory", "notes.md")
	if _, err := os.Stat(homePath); err == nil {
		t.Fatalf("did not expect memory file in home dir: %s", homePath)
	}
}

func TestBuildToolRegistryUsesExplicitWorkdir(t *testing.T) {
	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	workspaceDir := filepath.Join(root, "workspace")
	explicitDir := filepath.Join(root, "custom")
	for _, dir := range []string{homeDir, workspaceDir, explicitDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	app := &App{
		paths: config.Paths{
			HomeDir:      homeDir,
			WorkspaceDir: workspaceDir,
		},
	}

	registry := app.buildToolRegistry(explicitDir)
	tool, ok := registry.Get("write_file")
	if !ok {
		t.Fatal("write_file tool not registered")
	}

	result := tool.Execute(context.Background(), `{"path":"memory/notes.md","content":"hello explicit workdir"}`)
	if result.IsError {
		t.Fatalf("unexpected write_file error: %s", result.Content)
	}

	explicitPath := filepath.Join(explicitDir, "memory", "notes.md")
	if _, err := os.Stat(explicitPath); err != nil {
		t.Fatalf("expected memory file in explicit workdir: %v", err)
	}
}

func TestBuildToolRegistryRegistersSaveMemoryTool(t *testing.T) {
	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	workspaceDir := filepath.Join(root, "workspace")
	for _, dir := range []string{homeDir, workspaceDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	app := &App{
		paths: config.Paths{
			HomeDir:      homeDir,
			WorkspaceDir: workspaceDir,
		},
	}

	registry := app.buildToolRegistry("")
	tool, ok := registry.Get("save_memory")
	if !ok {
		t.Fatal("save_memory tool not registered")
	}

	result := tool.Execute(context.Background(), `{"category":"preference","content":"Prefiero respuestas técnicas y directas"}`)
	if result.IsError {
		t.Fatalf("unexpected save_memory error: %s", result.Content)
	}

	path := filepath.Join(workspaceDir, "memory", "preferences.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Prefiero respuestas técnicas y directas") {
		t.Fatalf("unexpected memory content: %s", string(data))
	}
}

func TestBuildToolRegistryRegistersInspectImageTool(t *testing.T) {
	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	workspaceDir := filepath.Join(root, "workspace")
	for _, dir := range []string{homeDir, workspaceDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	app := &App{
		paths: config.Paths{
			HomeDir:      homeDir,
			WorkspaceDir: workspaceDir,
		},
	}

	registry := app.buildToolRegistry("")
	if _, ok := registry.Get("inspect_image"); !ok {
		t.Fatal("inspect_image tool not registered")
	}
	if _, ok := registry.Get("clean_cache"); !ok {
		t.Fatal("clean_cache tool not registered")
	}
}
