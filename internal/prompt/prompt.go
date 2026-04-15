package prompt

import (
	"os"
	"strings"

	"github.com/oscarcode/elementary-claw/internal/config"
)

// BuildSystemPrompt assembles a system-level prompt from the workspace
// personality files (IDENTITY.md, SOUL.md, USER.md, HEARTBEAT.md, AGENTS.md,
// TOOLS.md). Each file's content is included as a section. Files that are
// missing or empty are silently skipped.
func BuildSystemPrompt(paths config.Paths) string {
	sections := []string{
		paths.IdentityPath,
		paths.SoulPath,
		paths.UserPath,
		paths.AgentPath,
		paths.ToolsPath,
		paths.HeartbeatPath,
	}

	var parts []string
	for _, path := range sections {
		content := readFileOrEmpty(path)
		if content == "" {
			continue
		}
		parts = append(parts, content)
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// BuildSystemPromptWithSkills assembles the system prompt and appends active
// skill instructions as an additional section.
func BuildSystemPromptWithSkills(paths config.Paths, skillsText string) string {
	base := BuildSystemPrompt(paths)
	skillsText = strings.TrimSpace(skillsText)
	if skillsText == "" {
		return base
	}
	if base == "" {
		return "# SKILLS\n\n" + skillsText
	}
	return base + "\n\n---\n\n# SKILLS\n\n" + skillsText
}

func readFileOrEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
