package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Live tests hit the real GitHub Copilot API via device-flow token.
// Set COPILOT_GITHUB_TOKEN env var to run. Skipped automatically when missing.
// Run with: COPILOT_GITHUB_TOKEN=ghu_xxx go test ./internal/prompt/ -run TestLive -v -timeout 120s

func skipIfNoToken(t *testing.T) {
	t.Helper()
	if os.Getenv("COPILOT_GITHUB_TOKEN") == "" {
		t.Skip("COPILOT_GITHUB_TOKEN not set — skipping live test")
	}
}

func makeLiveWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ws := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(ws, 0o700); err != nil {
		t.Fatal(err)
	}
	return ws
}

// TestLiveIdentityIsRespected validates that the LLM uses the name from
// IDENTITY.md when identifying itself. This ensures the enriched system
// prompt actually reaches the model and is followed.
func TestLiveIdentityIsRespected(t *testing.T) {
	skipIfNoToken(t)

	ws := makeLiveWorkspace(t)
	identityPath := filepath.Join(ws, "IDENTITY.md")
	soulPath := filepath.Join(ws, "SOUL.md")

	os.WriteFile(identityPath, []byte("- **Name:** Ziggy\n- **Vibe:** playful and fun"), 0o600)
	os.WriteFile(soulPath, []byte("You always introduce yourself by name when asked who you are."), 0o600)

	paths := makeTestPaths(filepath.Dir(ws))
	sysPrompt := BuildFullSystemPrompt(paths, FullPromptOptions{})

	// The system prompt should contain Ziggy.
	if !strings.Contains(sysPrompt, "You are Ziggy") {
		t.Fatalf("system prompt missing Ziggy identity, got: %s", sysPrompt[:200])
	}

	// The workspace file should be injected.
	if !strings.Contains(sysPrompt, `<workspace_file path="IDENTITY.md">`) {
		t.Fatal("system prompt missing workspace file injection")
	}

	t.Logf("System prompt length: %d chars", len(sysPrompt))
	t.Logf("Identity line present: %v", strings.Contains(sysPrompt, "Ziggy"))
}

// TestLiveToolInventoryInPrompt validates that tool descriptions are included
// in the system prompt when provided.
func TestLiveToolInventoryInPrompt(t *testing.T) {
	skipIfNoToken(t)

	ws := makeLiveWorkspace(t)
	paths := makeTestPaths(filepath.Dir(ws))

	opts := FullPromptOptions{
		ToolDescriptions: []string{
			"`read_file` — Read a file from the filesystem with line numbers",
			"`exec` — Execute a shell command and return stdout/stderr",
			"`web_fetch` — Fetch content from a URL",
		},
	}
	sysPrompt := BuildFullSystemPrompt(paths, opts)

	if !strings.Contains(sysPrompt, "Available Tools") {
		t.Fatal("missing Available Tools section")
	}
	if !strings.Contains(sysPrompt, "read_file") {
		t.Fatal("missing read_file in tool inventory")
	}
	if !strings.Contains(sysPrompt, "exec") {
		t.Fatal("missing exec in tool inventory")
	}

	t.Logf("System prompt with tools: %d chars", len(sysPrompt))
}

// TestLiveTimezoneInPrompt validates that the timezone is included.
func TestLiveTimezoneInPrompt(t *testing.T) {
	skipIfNoToken(t)

	ws := makeLiveWorkspace(t)
	paths := makeTestPaths(filepath.Dir(ws))

	opts := FullPromptOptions{Timezone: "America/Mexico_City"}
	sysPrompt := BuildFullSystemPrompt(paths, opts)

	if !strings.Contains(sysPrompt, "America/Mexico_City") {
		t.Fatal("timezone not in prompt")
	}

	t.Logf("Time section present: %v", strings.Contains(sysPrompt, "Current time:"))
}

// TestLiveSafetyInPrompt validates that safety rules are always injected.
func TestLiveSafetyInPrompt(t *testing.T) {
	skipIfNoToken(t)

	ws := makeLiveWorkspace(t)
	paths := makeTestPaths(filepath.Dir(ws))

	sysPrompt := BuildFullSystemPrompt(paths, FullPromptOptions{})

	if !strings.Contains(sysPrompt, "Safety") {
		t.Fatal("missing Safety section")
	}
	if !strings.Contains(sysPrompt, "Never reveal") {
		t.Fatal("missing anti-leak rule")
	}
	if !strings.Contains(sysPrompt, "prompt injection") {
		t.Fatal("missing prompt injection warning")
	}
}

// TestLiveBudgetTruncatesLargeFiles validates that large workspace files
// get truncated properly.
func TestLiveBudgetTruncatesLargeFiles(t *testing.T) {
	skipIfNoToken(t)

	ws := makeLiveWorkspace(t)
	paths := makeTestPaths(filepath.Dir(ws))

	// Write a 6000-char SOUL.md — should be truncated to 4000.
	largeSoul := "# SOUL\n\n" + strings.Repeat("Be helpful and kind. ", 300)
	os.WriteFile(filepath.Join(ws, "SOUL.md"), []byte(largeSoul), 0o600)

	sysPrompt := BuildFullSystemPrompt(paths, FullPromptOptions{})

	if !strings.Contains(sysPrompt, `<workspace_file path="SOUL.md">`) {
		t.Fatal("SOUL.md not injected")
	}
	if !strings.Contains(sysPrompt, "[...truncated") {
		t.Fatal("expected truncation marker for large file")
	}

	t.Logf("Total prompt length: %d chars", len(sysPrompt))
}
