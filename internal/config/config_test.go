package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Paths tests ---

func TestDefaultPathsStructure(t *testing.T) {
	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	if paths.HomeDir == "" {
		t.Fatal("HomeDir should not be empty")
	}
	if !strings.HasSuffix(paths.StateDir, ".openclaw") {
		t.Fatalf("StateDir should end with .openclaw: %s", paths.StateDir)
	}
	if !strings.Contains(paths.WorkspaceDir, "workspace") {
		t.Fatalf("WorkspaceDir should contain 'workspace': %s", paths.WorkspaceDir)
	}
	if !strings.HasSuffix(paths.ConfigPath, "openclaw.json") {
		t.Fatalf("ConfigPath should end with openclaw.json: %s", paths.ConfigPath)
	}
	if !strings.Contains(paths.WorkspaceSkillsDir, "skills") {
		t.Fatalf("WorkspaceSkillsDir should contain 'skills': %s", paths.WorkspaceSkillsDir)
	}
	if !strings.HasSuffix(paths.AgentPath, "AGENTS.md") {
		t.Fatalf("AgentPath should end with AGENTS.md: %s", paths.AgentPath)
	}
	if !strings.HasSuffix(paths.BootstrapPath, "BOOTSTRAP.md") {
		t.Fatalf("BootstrapPath should end with BOOTSTRAP.md: %s", paths.BootstrapPath)
	}
}

func TestDefaultPathsConsistency(t *testing.T) {
	paths, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}

	// WorkspaceDir should be under StateDir
	if !strings.HasPrefix(paths.WorkspaceDir, paths.StateDir) {
		t.Fatalf("WorkspaceDir should be under StateDir: %s vs %s", paths.WorkspaceDir, paths.StateDir)
	}
	// SessionsDir should be under StateDir
	if !strings.HasPrefix(paths.SessionsDir, paths.StateDir) {
		t.Fatalf("SessionsDir should be under StateDir: %s vs %s", paths.SessionsDir, paths.StateDir)
	}
	// WorkspaceSkillsDir should be under WorkspaceDir
	if !strings.HasPrefix(paths.WorkspaceSkillsDir, paths.WorkspaceDir) {
		t.Fatalf("WorkspaceSkillsDir should be under WorkspaceDir: %s vs %s", paths.WorkspaceSkillsDir, paths.WorkspaceDir)
	}
}

// --- InitializeWorkspace tests ---

func testPaths(root string) Paths {
	stateDir := filepath.Join(root, ".openclaw")
	workspaceDir := filepath.Join(stateDir, "workspace")
	credentialsDir := filepath.Join(stateDir, "state", "credentials")

	return Paths{
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
		BundledSkillsDir:      filepath.Join(stateDir, "skills", "bundled"),
		ManagedSkillsDir:      filepath.Join(stateDir, "skills", "managed"),
		WorkspaceSkillsDir:    filepath.Join(workspaceDir, "skills"),
	}
}

func TestInitializeWorkspaceCreatesFiles(t *testing.T) {
	paths := testPaths(t.TempDir())
	opts := SetupOptions{
		UserName:        "oscar",
		PreferredName:   "Oscar",
		AssistantName:   "Claw",
		AssistantNature: "AI assistant",
		AssistantVibe:   "helpful",
		Provider:        "copilot-proxy",
		ProviderBaseURL: "http://localhost:1234/v1",
	}

	if err := InitializeWorkspace(paths, opts); err != nil {
		t.Fatal(err)
	}

	// Config file should exist
	if _, err := os.Stat(paths.ConfigPath); err != nil {
		t.Fatalf("config file should exist: %s", err)
	}

	// Auth file should exist
	if _, err := os.Stat(paths.AuthPath); err != nil {
		t.Fatalf("auth file should exist: %s", err)
	}

	// Workspace markdown files should exist
	for _, path := range []string{
		paths.AgentPath, paths.IdentityPath, paths.SoulPath,
		paths.UserPath, paths.ToolsPath, paths.HeartbeatPath, paths.BootstrapPath,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("workspace file should exist %s: %s", filepath.Base(path), err)
		}
	}

	// Skills directory should exist
	if _, err := os.Stat(paths.WorkspaceSkillsDir); err != nil {
		t.Fatalf("skills directory should exist: %s", err)
	}
}

func TestInitializeWorkspaceIdentityContent(t *testing.T) {
	paths := testPaths(t.TempDir())
	opts := SetupOptions{
		UserName:        "testuser",
		PreferredName:   "Test",
		AssistantName:   "Vero",
		AssistantNature: "coding partner",
		AssistantVibe:   "direct",
		Provider:        "copilot-proxy",
	}

	if err := InitializeWorkspace(paths, opts); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(paths.IdentityPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "Vero") {
		t.Fatalf("IDENTITY.md should contain assistant name 'Vero': %s", content)
	}
	if !strings.Contains(content, "coding partner") {
		t.Fatalf("IDENTITY.md should contain nature: %s", content)
	}
}

func TestInitializeWorkspaceUserContent(t *testing.T) {
	paths := testPaths(t.TempDir())
	opts := SetupOptions{
		UserName:      "myuser",
		PreferredName: "MyName",
		AssistantName: "Claw",
		Provider:      "copilot-proxy",
	}

	if err := InitializeWorkspace(paths, opts); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(paths.UserPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "myuser") {
		t.Fatalf("USER.md should contain account name: %s", content)
	}
	if !strings.Contains(content, "MyName") {
		t.Fatalf("USER.md should contain preferred name: %s", content)
	}
}

func TestInitializeWorkspaceBootstrapContent(t *testing.T) {
	paths := testPaths(t.TempDir())
	opts := SetupOptions{
		UserName:      "user",
		PreferredName: "Oscar",
		AssistantName: "Vero",
		Provider:      "copilot-proxy",
	}

	if err := InitializeWorkspace(paths, opts); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(paths.BootstrapPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "Oscar") {
		t.Fatalf("BOOTSTRAP.md should contain preferred name: %s", content)
	}
	if !strings.Contains(content, "Vero") {
		t.Fatalf("BOOTSTRAP.md should contain assistant name: %s", content)
	}
}

// --- LoadFileConfig tests ---

func TestLoadFileConfig(t *testing.T) {
	paths := testPaths(t.TempDir())
	opts := SetupOptions{
		UserName:        "user",
		PreferredName:   "User",
		AssistantName:   "Claw",
		Provider:        "copilot-proxy",
		ProviderBaseURL: "http://localhost:8080/v1",
	}

	if err := InitializeWorkspace(paths, opts); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFileConfig(paths)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Agent.Provider != "copilot-proxy" {
		t.Fatalf("expected provider 'copilot-proxy', got %q", cfg.Agent.Provider)
	}
	if cfg.Agent.BaseURL != "http://localhost:8080/v1" {
		t.Fatalf("expected base URL, got %q", cfg.Agent.BaseURL)
	}
	if !cfg.Setup.BootstrapReady {
		t.Fatal("expected bootstrapReady to be true")
	}
}

func TestLoadFileConfigMissing(t *testing.T) {
	paths := testPaths(t.TempDir())
	_, err := LoadFileConfig(paths)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadFileConfigInvalidJSON(t *testing.T) {
	paths := testPaths(t.TempDir())
	os.MkdirAll(filepath.Dir(paths.ConfigPath), 0o700)
	os.WriteFile(paths.ConfigPath, []byte("not json"), 0o600)

	_, err := LoadFileConfig(paths)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- SaveFileConfig tests ---

func TestSaveFileConfig(t *testing.T) {
	paths := testPaths(t.TempDir())
	os.MkdirAll(filepath.Dir(paths.ConfigPath), 0o700)

	cfg := FileConfig{
		Agent: AgentConfig{
			Model:    "gpt-4o",
			Provider: "copilot-proxy",
			BaseURL:  "http://example.com/v1",
		},
		Setup: SetupConfig{
			BootstrapReady: true,
		},
	}

	if err := SaveFileConfig(paths, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadFileConfig(paths)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Agent.Provider != "copilot-proxy" {
		t.Fatalf("expected provider preserved, got %q", loaded.Agent.Provider)
	}
	if loaded.Agent.BaseURL != "http://example.com/v1" {
		t.Fatalf("expected base URL preserved, got %q", loaded.Agent.BaseURL)
	}
}

func TestSaveFileConfigWithMcp(t *testing.T) {
	paths := testPaths(t.TempDir())
	os.MkdirAll(filepath.Dir(paths.ConfigPath), 0o700)

	cfg := FileConfig{
		Agent: AgentConfig{Provider: "test"},
		Mcp: McpConfig{
			Servers: map[string]McpServerConfig{
				"github": {
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-github"},
				},
			},
		},
	}

	if err := SaveFileConfig(paths, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadFileConfig(paths)
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded.Mcp.Servers) != 1 {
		t.Fatalf("expected 1 MCP server, got %d", len(loaded.Mcp.Servers))
	}
	srv, ok := loaded.Mcp.Servers["github"]
	if !ok {
		t.Fatal("expected 'github' MCP server")
	}
	if srv.Command != "npx" {
		t.Fatalf("expected command 'npx', got %q", srv.Command)
	}
}

// --- Config JSON roundtrip ---

func TestFileConfigJSONRoundtrip(t *testing.T) {
	original := FileConfig{
		Agent: AgentConfig{
			Model:     "gpt-4o",
			Workspace: "/home/user/.openclaw/workspace",
			Provider:  "github-copilot",
			BaseURL:   "https://api.example.com/v1",
		},
		Setup: SetupConfig{
			ProviderPending: true,
			BootstrapReady:  true,
		},
		Mcp: McpConfig{
			Servers: map[string]McpServerConfig{
				"filesystem": {
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
				},
				"remote": {
					URL:     "https://mcp.example.com",
					Headers: map[string]string{"Authorization": "Bearer token"},
				},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var decoded FileConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Agent.Model != original.Agent.Model {
		t.Fatalf("model mismatch: %q vs %q", decoded.Agent.Model, original.Agent.Model)
	}
	if decoded.Setup.ProviderPending != original.Setup.ProviderPending {
		t.Fatal("providerPending mismatch")
	}
	if len(decoded.Mcp.Servers) != 2 {
		t.Fatalf("expected 2 MCP servers, got %d", len(decoded.Mcp.Servers))
	}
}

func TestInitializeWorkspaceConfigContent(t *testing.T) {
	paths := testPaths(t.TempDir())
	opts := SetupOptions{
		UserName:        "user",
		PreferredName:   "User",
		AssistantName:   "Claw",
		Provider:        "github-copilot",
		ProviderPending: true,
	}

	if err := InitializeWorkspace(paths, opts); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFileConfig(paths)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Agent.Provider != "github-copilot" {
		t.Fatalf("expected provider 'github-copilot', got %q", cfg.Agent.Provider)
	}
	if !cfg.Setup.ProviderPending {
		t.Fatal("expected providerPending to be true")
	}
	if !cfg.Setup.BootstrapReady {
		t.Fatal("expected bootstrapReady to be true")
	}
}
