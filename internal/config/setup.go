package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SetupOptions struct {
	WorkspaceDir    string
	UserName        string
	PreferredName   string
	AssistantName   string
	AssistantNature string
	AssistantVibe   string
	Provider        string
	ProviderBaseURL string
	ProviderPending bool
}

type FileConfig struct {
	Agent AgentConfig `json:"agent"`
	Setup SetupConfig `json:"setup"`
	Mcp   McpConfig   `json:"mcp,omitempty"`
}

type AgentConfig struct {
	Model     string `json:"model"`
	Workspace string `json:"workspace"`
	Provider  string `json:"provider"`
	BaseURL   string `json:"baseUrl,omitempty"`
}

type SetupConfig struct {
	ProviderPending bool `json:"providerPending"`
	BootstrapReady  bool `json:"bootstrapReady"`
}

type McpConfig struct {
	Servers map[string]McpServerConfig `json:"servers,omitempty"`
}

type McpServerConfig struct {
	Command           string            `json:"command,omitempty"`
	Args              []string          `json:"args,omitempty"`
	URL               string            `json:"url,omitempty"`
	Headers           map[string]string `json:"headers,omitempty"`
	OAuthClientID     string            `json:"oauthClientId,omitempty"`
	OAuthClientSecret string            `json:"oauthClientSecret,omitempty"`
	OAuthTokenURL     string            `json:"oauthTokenUrl,omitempty"`
	OAuthRedirectURI  string            `json:"oauthRedirectUri,omitempty"`
	OAuthScopes       []string          `json:"oauthScopes,omitempty"`
}

func InitializeWorkspace(paths Paths, options SetupOptions) error {
	if strings.TrimSpace(options.WorkspaceDir) == "" {
		options.WorkspaceDir = paths.WorkspaceDir
	}

	if err := ensureDir(filepath.Dir(paths.AuthPath)); err != nil {
		return err
	}
	if err := ensureDir(options.WorkspaceDir); err != nil {
		return err
	}
	if err := ensureDir(paths.SessionsDir); err != nil {
		return err
	}
	if err := ensureDir(paths.WorkspaceSkillsDir); err != nil {
		return err
	}

	configBytes, err := json.MarshalIndent(FileConfig{
		Agent: AgentConfig{
			Model:     "github-copilot/gpt-5.4",
			Workspace: options.WorkspaceDir,
			Provider:  options.Provider,
			BaseURL:   options.ProviderBaseURL,
		},
		Setup: SetupConfig{
			ProviderPending: options.ProviderPending,
			BootstrapReady:  true,
		},
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(paths.ConfigPath, append(configBytes, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	authPayload := map[string]any{
		"version": 1,
		"profiles": map[string]any{
			fmt.Sprintf("%s:default", options.Provider): map[string]any{
				"provider": options.Provider,
				"mode":     firstMode(options.ProviderPending),
				"token":    "",
			},
		},
	}
	authBytes, err := json.MarshalIndent(authPayload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth profiles: %w", err)
	}
	if err := os.WriteFile(paths.AuthPath, append(authBytes, '\n'), 0o600); err != nil {
		return fmt.Errorf("write auth profiles: %w", err)
	}

	files := map[string]string{
		filepath.Join(options.WorkspaceDir, "AGENTS.md"):    buildAgentsMarkdown(),
		filepath.Join(options.WorkspaceDir, "IDENTITY.md"):  buildIdentityMarkdown(options),
		filepath.Join(options.WorkspaceDir, "SOUL.md"):      buildSoulMarkdown(options),
		filepath.Join(options.WorkspaceDir, "USER.md"):      buildUserMarkdown(options),
		filepath.Join(options.WorkspaceDir, "TOOLS.md"):     buildToolsMarkdown(),
		filepath.Join(options.WorkspaceDir, "HEARTBEAT.md"): buildHeartbeatMarkdown(),
		filepath.Join(options.WorkspaceDir, "BOOTSTRAP.md"): buildBootstrapMarkdown(options),
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", filepath.Base(path), err)
		}
	}

	return nil
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", path, err)
	}
	return nil
}

func firstMode(providerPending bool) string {
	if providerPending {
		return "pending"
	}
	return "token"
}

func buildAgentsMarkdown() string {
	return "# AGENTS\n\nThis workspace is managed by elementary-claw.\n"
}

func buildIdentityMarkdown(options SetupOptions) string {
	return fmt.Sprintf("# IDENTITY\n\n- assistant_name: %s\n- assistant_nature: %s\n- assistant_vibe: %s\n", options.AssistantName, options.AssistantNature, options.AssistantVibe)
}

func buildSoulMarkdown(options SetupOptions) string {
	return fmt.Sprintf("# SOUL\n\n%s should be helpful without filler, pragmatic, and respectful of user intent.\n", options.AssistantName)
}

func buildUserMarkdown(options SetupOptions) string {
	return fmt.Sprintf("# USER\n\n- account_name: %s\n- preferred_name: %s\n", options.UserName, options.PreferredName)
}

func buildToolsMarkdown() string {
	return "# TOOLS\n\n- filesystem\n- process\n- notifications\n"
}

func buildHeartbeatMarkdown() string {
	return "# HEARTBEAT\n\nResume prior context, protect user data, and keep the local machine stable.\n"
}

func buildBootstrapMarkdown(options SetupOptions) string {
	return fmt.Sprintf("# BOOTSTRAP\n\nWhen the runtime wakes up, greet %s as %s. Explain that the assistant was created during account setup and is ready to continue from first login.\n", options.PreferredName, options.AssistantName)
}

func LoadFileConfig(paths Paths) (FileConfig, error) {
	data, err := os.ReadFile(paths.ConfigPath)
	if err != nil {
		return FileConfig{}, fmt.Errorf("read config: %w", err)
	}

	var cfg FileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return FileConfig{}, fmt.Errorf("decode config: %w", err)
	}

	return cfg, nil
}

func SaveFileConfig(paths Paths, cfg FileConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(paths.ConfigPath), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	if err := os.WriteFile(paths.ConfigPath, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
