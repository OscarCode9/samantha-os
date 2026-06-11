package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/session"
)

func TestEnsureFirstMessageCreatesBootstrapOnce(t *testing.T) {
	paths := makeBootstrapTestPaths(t.TempDir())
	store := session.NewStore(paths)

	if err := os.MkdirAll(filepath.Dir(paths.BootstrapPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.BootstrapPath, []byte("hello from bootstrap"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, created, err := EnsureFirstMessage(paths, store, "")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected bootstrap session to be created")
	}
	if result.Path == "" {
		t.Fatal("expected bootstrap path to be returned")
	}

	_, created, err = EnsureFirstMessage(paths, store, "")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("expected bootstrap ensure to be idempotent")
	}

	record, err := store.Get("bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Messages) != 1 {
		t.Fatalf("unexpected bootstrap message count: %d", len(record.Messages))
	}
}

func makeBootstrapTestPaths(root string) config.Paths {
	stateDir := filepath.Join(root, ".samantha")
	workspaceDir := filepath.Join(stateDir, "workspace")
	credentialsDir := filepath.Join(stateDir, "state", "credentials")

	return config.Paths{
		HomeDir:               root,
		StateDir:              stateDir,
		WorkspaceDir:          workspaceDir,
		CredentialsDir:        credentialsDir,
		SessionsDir:           filepath.Join(stateDir, "state", "sessions"),
		ConfigPath:            filepath.Join(stateDir, "samantha.json"),
		AuthPath:              filepath.Join(stateDir, "agents", "main", "agent", "auth-profiles.json"),
		CopilotTokenCachePath: filepath.Join(credentialsDir, "github-copilot.token.json"),
		AgentPath:             filepath.Join(workspaceDir, "AGENTS.md"),
		IdentityPath:          filepath.Join(workspaceDir, "IDENTITY.md"),
		SoulPath:              filepath.Join(workspaceDir, "SOUL.md"),
		UserPath:              filepath.Join(workspaceDir, "USER.md"),
		ToolsPath:             filepath.Join(workspaceDir, "TOOLS.md"),
		HeartbeatPath:         filepath.Join(workspaceDir, "HEARTBEAT.md"),
		BootstrapPath:         filepath.Join(workspaceDir, "BOOTSTRAP.md"),
	}
}

func TestDetectBootstrapModeTrue(t *testing.T) {
	paths := makeBootstrapTestPaths(t.TempDir())
	os.MkdirAll(filepath.Dir(paths.BootstrapPath), 0o700)
	os.WriteFile(paths.BootstrapPath, []byte("Setup instructions here"), 0o600)

	if !DetectBootstrapMode(paths) {
		t.Fatal("expected bootstrap mode to be detected")
	}
}

func TestDetectBootstrapModeFalseNoFile(t *testing.T) {
	paths := makeBootstrapTestPaths(t.TempDir())
	if DetectBootstrapMode(paths) {
		t.Fatal("expected no bootstrap mode without file")
	}
}

func TestDetectBootstrapModeFalseEmpty(t *testing.T) {
	paths := makeBootstrapTestPaths(t.TempDir())
	os.MkdirAll(filepath.Dir(paths.BootstrapPath), 0o700)
	os.WriteFile(paths.BootstrapPath, []byte("   "), 0o600)

	if DetectBootstrapMode(paths) {
		t.Fatal("expected no bootstrap mode for empty file")
	}
}

func TestReadBootstrapInstructions(t *testing.T) {
	paths := makeBootstrapTestPaths(t.TempDir())
	os.MkdirAll(filepath.Dir(paths.BootstrapPath), 0o700)
	os.WriteFile(paths.BootstrapPath, []byte("Welcome! Let me help you set up."), 0o600)

	content := ReadBootstrapInstructions(paths)
	if content != "Welcome! Let me help you set up." {
		t.Fatalf("unexpected content: %s", content)
	}
}

func TestReadBootstrapInstructionsNoFile(t *testing.T) {
	paths := makeBootstrapTestPaths(t.TempDir())
	content := ReadBootstrapInstructions(paths)
	if content != "" {
		t.Fatalf("expected empty, got: %s", content)
	}
}

func TestCompleteBootstrap(t *testing.T) {
	paths := makeBootstrapTestPaths(t.TempDir())
	os.MkdirAll(paths.WorkspaceDir, 0o700)
	os.WriteFile(paths.BootstrapPath, []byte("bootstrap content"), 0o600)

	if err := CompleteBootstrap(paths); err != nil {
		t.Fatal(err)
	}

	// BOOTSTRAP.md should be gone.
	if _, err := os.Stat(paths.BootstrapPath); !os.IsNotExist(err) {
		t.Fatal("expected BOOTSTRAP.md to be removed")
	}

	// .workspace-state.json should exist with completion marker.
	statePath := workspaceStatePath(paths)
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state["bootstrapCompleted"] != true {
		t.Fatalf("expected bootstrapCompleted=true, got: %v", state)
	}
}

func TestCompleteBootstrapIdempotent(t *testing.T) {
	paths := makeBootstrapTestPaths(t.TempDir())
	os.MkdirAll(paths.WorkspaceDir, 0o700)
	// No BOOTSTRAP.md exists — should not error.
	if err := CompleteBootstrap(paths); err != nil {
		t.Fatalf("expected no error for idempotent complete, got: %v", err)
	}
}
