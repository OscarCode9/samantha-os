package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/session"
	"github.com/oscarcode/elementary-claw/internal/tools"
)

// Live tests hit the real GitHub Copilot API via device-flow token.
// Set COPILOT_GITHUB_TOKEN env var to run. Skipped automatically when missing.
// Run with: COPILOT_GITHUB_TOKEN=ghu_xxx go test ./internal/runtime/ -run TestLive -v

func skipIfNoToken(t *testing.T) {
	t.Helper()
	if os.Getenv("COPILOT_GITHUB_TOKEN") == "" {
		t.Skip("COPILOT_GITHUB_TOKEN not set — skipping live test")
	}
}

func makeLivePaths(t *testing.T) config.Paths {
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

const liveModel = "gpt-4o"

func setupLiveConfig(t *testing.T, paths config.Paths) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(paths.ConfigPath), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.FileConfig{
		Agent: config.AgentConfig{
			Model:    liveModel,
			Provider: "github-copilot",
		},
	}
	if err := config.SaveFileConfig(paths, cfg); err != nil {
		t.Fatal(err)
	}
}

// --- Live Tests ---

func TestLiveChatCompletion(t *testing.T) {
	skipIfNoToken(t)

	paths := makeLivePaths(t)
	setupLiveConfig(t, paths)
	store := session.NewStore(paths)

	server := httptest.NewServer(newHandler(paths, store, nil, nil))
	defer server.Close()

	body := map[string]any{
		"model": liveModel,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with exactly: PONG"},
		},
		"max_tokens": 20,
	}
	bodyBytes, _ := json.Marshal(body)

	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("invalid JSON response: %v\n%s", err, string(respBody))
	}

	choices, ok := result["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("expected choices in response: %s", string(respBody))
	}

	choice := choices[0].(map[string]any)
	message := choice["message"].(map[string]any)
	content := message["content"].(string)
	t.Logf("Response: %s", content)

	if !strings.Contains(strings.ToUpper(content), "PONG") {
		t.Fatalf("expected PONG in response, got: %s", content)
	}
}

func TestLiveChatCompletionWithSession(t *testing.T) {
	skipIfNoToken(t)

	paths := makeLivePaths(t)
	setupLiveConfig(t, paths)
	store := session.NewStore(paths)

	server := httptest.NewServer(newHandler(paths, store, nil, nil))
	defer server.Close()

	// First message
	body1 := map[string]any{
		"model":      liveModel,
		"session_id": "live-test-session",
		"messages": []map[string]string{
			{"role": "user", "content": "My favorite fruit is BANANA. Reply with just OK."},
		},
		"max_tokens": 20,
	}
	resp1 := doLiveRequest(t, server.URL, body1)
	t.Logf("First response: %s", resp1)

	// Second message in same session — should have context from first
	body2 := map[string]any{
		"model":      liveModel,
		"session_id": "live-test-session",
		"messages": []map[string]string{
			{"role": "user", "content": "What is my favorite fruit? Reply with just the fruit name."},
		},
		"max_tokens": 20,
	}
	resp2 := doLiveRequest(t, server.URL, body2)
	t.Logf("Second response: %s", resp2)

	if !strings.Contains(strings.ToUpper(resp2), "BANANA") {
		t.Fatalf("expected session to remember BANANA, got: %s", resp2)
	}

	// Verify session was persisted
	record, err := store.Get("live-test-session")
	if err != nil {
		t.Fatal(err)
	}
	if record == nil {
		t.Fatal("expected session to be persisted")
	}
	if len(record.Messages) < 4 {
		t.Fatalf("expected at least 4 messages (2 user + 2 assistant), got %d", len(record.Messages))
	}
	t.Logf("Session has %d messages", len(record.Messages))
}

func TestLiveToolCallExecution(t *testing.T) {
	skipIfNoToken(t)

	paths := makeLivePaths(t)
	setupLiveConfig(t, paths)
	store := session.NewStore(paths)

	// Create a temp file for the model to read
	testFile := filepath.Join(t.TempDir(), "greeting.txt")
	os.WriteFile(testFile, []byte("magic=42"), 0o600)

	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(filepath.Dir(testFile)))

	server := httptest.NewServer(newHandler(paths, store, registry, nil))
	defer server.Close()

	body := map[string]any{
		"model": liveModel,
		"messages": []map[string]string{
			{"role": "user", "content": fmt.Sprintf("Use read_file to read %s then reply with only the value after the equals sign.", testFile)},
		},
		"max_tokens": 200,
	}
	resp := doLiveRequest(t, server.URL, body)
	t.Logf("Tool call response: %s", resp)

	if !strings.Contains(resp, "42") {
		t.Fatalf("expected '42' in response after tool call, got: %s", resp)
	}
}

func TestLiveModelsEndpoint(t *testing.T) {
	skipIfNoToken(t)

	paths := makeLivePaths(t)
	setupLiveConfig(t, paths)
	store := session.NewStore(paths)

	server := httptest.NewServer(newHandler(paths, store, nil, nil))
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	data, ok := result["data"].([]any)
	if !ok || len(data) == 0 {
		t.Fatalf("expected models list, got: %s", string(body))
	}
	t.Logf("Available models: %d", len(data))
	for _, m := range data {
		model := m.(map[string]any)
		t.Logf("  - %s", model["id"])
	}
}

// --- helpers ---

func doLiveRequest(t *testing.T, serverURL string, body map[string]any) string {
	t.Helper()
	bodyBytes, _ := json.Marshal(body)
	resp, err := http.Post(serverURL+"/v1/chat/completions", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, string(respBody))
	}

	choices, ok := result["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("no choices in response: %s", string(respBody))
	}

	choice := choices[0].(map[string]any)
	message := choice["message"].(map[string]any)
	return message["content"].(string)
}
