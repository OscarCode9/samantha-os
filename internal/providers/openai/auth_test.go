package openai

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oscarcode/elementary-claw/internal/config"
)

func testPaths(t *testing.T) config.Paths {
	t.Helper()
	root := t.TempDir()
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
		BundledSkillsDir:      filepath.Join(stateDir, "skills", "bundled"),
		ManagedSkillsDir:      filepath.Join(stateDir, "skills", "managed"),
		WorkspaceSkillsDir:    filepath.Join(workspaceDir, "skills"),
	}
}

func TestSaveAndResolveAPIKeyFromAuthStore(t *testing.T) {
	paths := testPaths(t)
	if err := SaveAPIKey(paths, "sk-test-123"); err != nil {
		t.Fatal(err)
	}

	key, source, err := ResolveAPIKey(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if key != "sk-test-123" {
		t.Fatalf("unexpected key: %s", key)
	}
	if source != "auth-store:openai:default" {
		t.Fatalf("unexpected source: %s", source)
	}
}

func TestResolveAPIKeyPrefersExplicitValue(t *testing.T) {
	paths := testPaths(t)
	t.Setenv("OPENAI_API_KEY", "sk-env-123")

	key, source, err := ResolveAPIKey(paths, "sk-explicit-123")
	if err != nil {
		t.Fatal(err)
	}
	if key != "sk-explicit-123" || source != "flag" {
		t.Fatalf("unexpected explicit resolution: %s %s", key, source)
	}
}

func TestResolveAPIKeyFromEnv(t *testing.T) {
	paths := testPaths(t)
	t.Setenv("OPENAI_API_KEY", "sk-env-123")

	key, source, err := ResolveAPIKey(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if key != "sk-env-123" || source != "env:OPENAI_API_KEY" {
		t.Fatalf("unexpected env resolution: %s %s", key, source)
	}
}

func TestResolveAPIKeyMissing(t *testing.T) {
	paths := testPaths(t)
	_ = os.Remove(paths.AuthPath)

	if _, _, err := ResolveAPIKey(paths, ""); err == nil {
		t.Fatal("expected missing key error")
	}
}
