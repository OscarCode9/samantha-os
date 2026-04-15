package config

import (
	"os"
	"path/filepath"
)

type Paths struct {
	HomeDir               string
	StateDir              string
	WorkspaceDir          string
	CredentialsDir        string
	SessionsDir           string
	ConfigPath            string
	AuthPath              string
	CopilotTokenCachePath string
	AgentPath             string
	IdentityPath          string
	SoulPath              string
	UserPath              string
	ToolsPath             string
	HeartbeatPath         string
	BootstrapPath         string
	BundledSkillsDir      string // skills shipped with the binary
	ManagedSkillsDir      string // skills installed via CLI
	WorkspaceSkillsDir    string // project-local skills
}

// McpTokenPath returns the path where a cached OAuth token for an MCP server
// named serverName is stored.
func (p Paths) McpTokenPath(serverName string) string {
	return filepath.Join(p.CredentialsDir, "mcp-"+serverName+".token.json")
}

func DefaultPaths() (Paths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}

	stateDir := filepath.Join(homeDir, ".openclaw")
	workspaceDir := filepath.Join(stateDir, "workspace")
	credentialsDir := filepath.Join(stateDir, "state", "credentials")

	return Paths{
		HomeDir:               homeDir,
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
	}, nil
}
