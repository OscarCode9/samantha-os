package config

import (
	"encoding/json"
	"errors"
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
			Model:     "gpt-5.4",
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

type workspaceSnapshot struct {
	agent     string
	identity  string
	soul      string
	user      string
	tools     string
	heartbeat string
	bootstrap string
}

// RepairLegacyWorkspaceFiles upgrades legacy generated workspace prompt files
// to the current canonical format while preserving the user's identity values.
// It is idempotent and only rewrites files when it detects the old seeded
// workspace templates from earlier builds.
func RepairLegacyWorkspaceFiles(paths Paths) error {
	snapshot, err := readWorkspaceSnapshot(paths)
	if err != nil {
		return err
	}
	if !looksLikeLegacyWorkspace(snapshot) {
		return nil
	}
	if err := ensureDir(paths.WorkspaceDir); err != nil {
		return err
	}

	options := deriveSetupOptions(snapshot)
	files := map[string]string{
		paths.AgentPath:     buildAgentsMarkdown(),
		paths.IdentityPath:  buildIdentityMarkdown(options),
		paths.SoulPath:      buildSoulMarkdown(options),
		paths.UserPath:      buildUserMarkdown(options),
		paths.ToolsPath:     buildToolsMarkdown(),
		paths.HeartbeatPath: buildHeartbeatMarkdown(),
	}
	if strings.TrimSpace(snapshot.bootstrap) != "" {
		files[paths.BootstrapPath] = buildBootstrapMarkdown(options)
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", filepath.Base(path), err)
		}
	}

	return nil
}

func readWorkspaceSnapshot(paths Paths) (workspaceSnapshot, error) {
	agent, err := readOptionalFile(paths.AgentPath)
	if err != nil {
		return workspaceSnapshot{}, err
	}
	identity, err := readOptionalFile(paths.IdentityPath)
	if err != nil {
		return workspaceSnapshot{}, err
	}
	soul, err := readOptionalFile(paths.SoulPath)
	if err != nil {
		return workspaceSnapshot{}, err
	}
	user, err := readOptionalFile(paths.UserPath)
	if err != nil {
		return workspaceSnapshot{}, err
	}
	tools, err := readOptionalFile(paths.ToolsPath)
	if err != nil {
		return workspaceSnapshot{}, err
	}
	heartbeat, err := readOptionalFile(paths.HeartbeatPath)
	if err != nil {
		return workspaceSnapshot{}, err
	}
	bootstrapText, err := readOptionalFile(paths.BootstrapPath)
	if err != nil {
		return workspaceSnapshot{}, err
	}

	return workspaceSnapshot{
		agent:     agent,
		identity:  identity,
		soul:      soul,
		user:      user,
		tools:     tools,
		heartbeat: heartbeat,
		bootstrap: bootstrapText,
	}, nil
}

func readOptionalFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	return string(data), nil
}

func looksLikeLegacyWorkspace(snapshot workspaceSnapshot) bool {
	return strings.Contains(snapshot.agent, "# AGENTS.md - Initial Setup Workspace") ||
		strings.Contains(snapshot.identity, "# IDENTITY.md") ||
		strings.Contains(snapshot.identity, "Configured during elementary OS Initial Setup") ||
		strings.Contains(snapshot.soul, "# SOUL.md") ||
		strings.Contains(snapshot.soul, "On first real conversation") ||
		strings.Contains(snapshot.user, "# USER.md") ||
		strings.Contains(snapshot.tools, "Local machine notes can be added here later.") ||
		strings.Contains(snapshot.heartbeat, "HEARTBEAT_OK")
}

func deriveSetupOptions(snapshot workspaceSnapshot) SetupOptions {
	assistantName := firstNonEmptyValue(
		extractMarkdownValue(snapshot.identity, "assistant_name", "name"),
		extractMarkdownValue(snapshot.soul, "assistant_name", "name"),
		"Assistant",
	)
	assistantNature := firstNonEmptyValue(
		extractMarkdownValue(snapshot.identity, "assistant_nature", "nature"),
		extractMarkdownValue(snapshot.soul, "assistant_nature", "nature"),
		"personal AI assistant",
	)
	assistantVibe := firstNonEmptyValue(
		extractMarkdownValue(snapshot.identity, "assistant_vibe", "vibe"),
		extractMarkdownValue(snapshot.soul, "assistant_vibe", "vibe"),
		"helpful",
	)
	userName := firstNonEmptyValue(
		extractMarkdownValue(snapshot.user, "account_name", "account username", "username"),
		extractMarkdownValue(snapshot.user, "full name"),
	)
	preferredName := firstNonEmptyValue(
		extractMarkdownValue(snapshot.user, "preferred_name", "preferred name"),
		userName,
	)
	if userName == "" {
		userName = preferredName
	}

	return SetupOptions{
		UserName:        userName,
		PreferredName:   preferredName,
		AssistantName:   assistantName,
		AssistantNature: assistantNature,
		AssistantVibe:   assistantVibe,
	}
}

func extractMarkdownValue(content string, keys ...string) string {
	if content == "" {
		return ""
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimLeft(trimmed, "-* ")
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		trimmed = strings.ReplaceAll(trimmed, "`", "")

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		for _, candidate := range keys {
			if key == strings.ToLower(candidate) {
				value := strings.TrimSpace(parts[1])
				return strings.Trim(value, "*_` ")
			}
		}
	}

	return ""
}

func firstNonEmptyValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
