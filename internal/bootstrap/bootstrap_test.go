package bootstrap

import (
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
	stateDir := filepath.Join(root, ".openclaw")
	workspaceDir := filepath.Join(stateDir, "workspace")
	credentialsDir := filepath.Join(stateDir, "state", "credentials")

	return config.Paths{
		HomeDir:               root,
		StateDir:              stateDir,
		WorkspaceDir:          workspaceDir,
		CredentialsDir:        credentialsDir,
		SessionsDir:           filepath.Join(stateDir, "state", "sessions"),
		ConfigPath:            filepath.Join(stateDir, "openclaw.json"),
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
