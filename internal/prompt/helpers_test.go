package prompt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oscarcode/elementary-claw/internal/config"
)

func makeTestPaths(root string) config.Paths {
	ws := filepath.Join(root, "workspace")
	return config.Paths{
		HomeDir:      root,
		WorkspaceDir: ws,
		IdentityPath: filepath.Join(ws, "IDENTITY.md"),
		SoulPath:     filepath.Join(ws, "SOUL.md"),
		UserPath:     filepath.Join(ws, "USER.md"),
		AgentPath:    filepath.Join(ws, "AGENTS.md"),
		ToolsPath:    filepath.Join(ws, "TOOLS.md"),
		HeartbeatPath: filepath.Join(ws, "HEARTBEAT.md"),
		BootstrapPath: filepath.Join(ws, "BOOTSTRAP.md"),
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
