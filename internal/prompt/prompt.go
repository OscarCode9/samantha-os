package prompt

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/oscarcode/elementary-claw/internal/config"
)

// FullPromptOptions controls optional sections of the enriched system prompt.
type FullPromptOptions struct {
	// ToolDescriptions is a list of "name — description" lines for each tool.
	ToolDescriptions []string
	// SkillEntries is a list of "name — description" lines for available skills.
	SkillEntries []string
	// Timezone is the IANA timezone string (e.g. "America/Mexico_City").
	Timezone string
	// MemorySection is the pre-built memory instructions + context block.
	// If empty, no memory section is included.
	MemorySection string
	// BootstrapInstructions is the content of BOOTSTRAP.md for first-run
	// onboarding. When set, it is injected as a high-priority section.
	BootstrapInstructions string
}

// BuildFullSystemPrompt assembles a structured system prompt with dedicated
// sections for identity, tooling, safety, skills, workspace context, and time.
// It replaces the simpler BuildSystemPromptWithSkills for production use.
func BuildFullSystemPrompt(paths config.Paths, opts FullPromptOptions) string {
	var sections []string

	// --- Identity line ---
	identityContent := readFileOrEmpty(paths.IdentityPath)
	name := ParseIdentityName(identityContent)
	if name != "" {
		sections = append(sections, fmt.Sprintf("You are %s, a personal AI assistant.", name))
	} else {
		sections = append(sections, "You are a personal AI assistant.")
	}

	// --- Bootstrap section (first-run onboarding) ---
	if opts.BootstrapInstructions != "" {
		bootstrapSection := "## BOOTSTRAP MODE\n\n" +
			"This is the first time the user is setting up their assistant. " +
			"Follow the bootstrap instructions carefully. When the setup is " +
			"complete, include the marker [BOOTSTRAP_COMPLETE] in your response.\n\n" +
			opts.BootstrapInstructions
		sections = append(sections, bootstrapSection)
	}

	// --- Tooling section ---
	if len(opts.ToolDescriptions) > 0 {
		toolSection := "## Available Tools\n\n"
		for _, desc := range opts.ToolDescriptions {
			toolSection += "- " + desc + "\n"
		}
		toolSection += "\nUse these tools when they help answer the user's request. Prefer reading files and searching over guessing."
		sections = append(sections, toolSection)
	}

	// --- Safety section ---
	sections = append(sections, SafetySection())

	// --- Skills section ---
	if len(opts.SkillEntries) > 0 {
		skillSection := "## Available Skills\n\n<available_skills>\n"
		for _, entry := range opts.SkillEntries {
			skillSection += "- " + entry + "\n"
		}
		skillSection += "</available_skills>\n\n"
		skillSection += "When a user request matches a skill, use the read_file tool to read the skill's SKILL.md for detailed instructions. Only load one skill per request."
		sections = append(sections, skillSection)
	}

	// --- Memory section ---
	if opts.MemorySection != "" {
		sections = append(sections, opts.MemorySection)
	}

	// --- Workspace section ---
	sections = append(sections, fmt.Sprintf("## Workspace\n\nWorking directory: `%s`\nUse relative paths when possible. This is the user's workspace root.", paths.WorkspaceDir))

	// --- Time section ---
	tz := opts.Timezone
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
		tz = "UTC"
	}
	now := time.Now().In(loc)
	sections = append(sections, fmt.Sprintf("## Time\n\nUser timezone: %s\nCurrent time: %s", tz, now.Format("2006-01-02 15:04 MST")))

	// --- Workspace files (injected with budget) ---
	workspaceFiles := loadWorkspaceFiles(paths)
	if injected := InjectWorkspaceFiles(workspaceFiles, DefaultMaxCharsPerFile, DefaultTotalMaxChars); injected != "" {
		sections = append(sections, injected)
	}

	return strings.Join(sections, "\n\n---\n\n")
}

// BuildToolDescriptions generates "name — description" lines from a list of
// tool definitions. The caller typically gets these from registry.Definitions().
func BuildToolDescriptions(names []string, descriptions []string) []string {
	n := len(names)
	if len(descriptions) < n {
		n = len(descriptions)
	}
	lines := make([]string, 0, n)
	for i := 0; i < n; i++ {
		lines = append(lines, fmt.Sprintf("`%s` — %s", names[i], descriptions[i]))
	}
	return lines
}

// ParseIdentityName extracts the assistant name from IDENTITY.md content.
// It looks for a "Name: <value>" line (case-insensitive).
func ParseIdentityName(content string) string {
	if content == "" {
		return ""
	}
	re := regexp.MustCompile(`(?im)^\s*[\-\*]?\s*\*?\*?(?:Name|assistant_name)\*?\*?\s*:\s*(.+)$`)
	matches := re.FindStringSubmatch(content)
	if len(matches) < 2 {
		return ""
	}
	name := strings.TrimSpace(matches[1])
	// Strip surrounding markdown formatting and re-trim.
	name = strings.Trim(name, "*_`")
	name = strings.TrimSpace(name)
	return name
}

// loadWorkspaceFiles reads the standard workspace files and returns them
// as WorkspaceFile entries suitable for budget injection.
func loadWorkspaceFiles(paths config.Paths) []WorkspaceFile {
	fileDefs := []struct {
		name string
		path string
	}{
		{"IDENTITY.md", paths.IdentityPath},
		{"SOUL.md", paths.SoulPath},
		{"USER.md", paths.UserPath},
		{"AGENTS.md", paths.AgentPath},
		{"TOOLS.md", paths.ToolsPath},
		{"HEARTBEAT.md", paths.HeartbeatPath},
	}

	var files []WorkspaceFile
	for _, fd := range fileDefs {
		content := readFileOrEmpty(fd.path)
		if content != "" {
			files = append(files, WorkspaceFile{
				Name:    fd.name,
				Path:    fd.path,
				Content: content,
			})
		}
	}
	return files
}

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
