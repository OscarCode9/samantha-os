package prompt

import (
	"strings"
	"testing"
)

func TestParseIdentityNameBasic(t *testing.T) {
	content := "# IDENTITY.md\n\n- **Name:** Ziggy\n- **Nature:** AI assistant\n"
	name := ParseIdentityName(content)
	if name != "Ziggy" {
		t.Fatalf("expected Ziggy, got %q", name)
	}
}

func TestParseIdentityNamePlainText(t *testing.T) {
	content := "Name: Semantha\nVibe: warm"
	name := ParseIdentityName(content)
	if name != "Semantha" {
		t.Fatalf("expected Semantha, got %q", name)
	}
}

func TestParseIdentityNameEmpty(t *testing.T) {
	name := ParseIdentityName("")
	if name != "" {
		t.Fatalf("expected empty, got %q", name)
	}
}

func TestParseIdentityNameNoMatch(t *testing.T) {
	name := ParseIdentityName("Just some random text without a name field")
	if name != "" {
		t.Fatalf("expected empty, got %q", name)
	}
}

func TestParseIdentityNameBulletStar(t *testing.T) {
	content := "* Name: Bot9000"
	name := ParseIdentityName(content)
	if name != "Bot9000" {
		t.Fatalf("expected Bot9000, got %q", name)
	}
}

func TestParseIdentityNameCurrentWorkspaceFormat(t *testing.T) {
	content := "# IDENTITY\n\n- assistant_name: Semantha\n- assistant_vibe: warm"
	name := ParseIdentityName(content)
	if name != "Semantha" {
		t.Fatalf("expected Semantha, got %q", name)
	}
}

func TestTruncateWithBudgetShort(t *testing.T) {
	result := TruncateWithBudget("hello", 100)
	if result != "hello" {
		t.Fatalf("expected no truncation, got %q", result)
	}
}

func TestTruncateWithBudgetLong(t *testing.T) {
	content := strings.Repeat("x", 200)
	result := TruncateWithBudget(content, 50)
	if len(result) < 50 {
		t.Fatal("result should be at least 50 chars")
	}
	if !strings.Contains(result, "[...truncated") {
		t.Fatal("expected truncation marker")
	}
}

func TestInjectWorkspaceFilesEmpty(t *testing.T) {
	result := InjectWorkspaceFiles(nil, 0, 0)
	if result != "" {
		t.Fatalf("expected empty, got %q", result)
	}
}

func TestInjectWorkspaceFilesMultiple(t *testing.T) {
	files := []WorkspaceFile{
		{Name: "IDENTITY.md", Content: "Name: Test"},
		{Name: "SOUL.md", Content: "Be helpful."},
	}
	result := InjectWorkspaceFiles(files, 4000, 20000)
	if !strings.Contains(result, `<workspace_file path="IDENTITY.md">`) {
		t.Fatal("expected IDENTITY.md boundary tag")
	}
	if !strings.Contains(result, `<workspace_file path="SOUL.md">`) {
		t.Fatal("expected SOUL.md boundary tag")
	}
	if !strings.Contains(result, "Name: Test") {
		t.Fatal("expected IDENTITY content")
	}
}

func TestInjectWorkspaceFilesTotalBudget(t *testing.T) {
	files := []WorkspaceFile{
		{Name: "A.md", Content: strings.Repeat("a", 500)},
		{Name: "B.md", Content: strings.Repeat("b", 500)},
	}
	// Budget so tight only one file fits fully.
	result := InjectWorkspaceFiles(files, 4000, 600)
	if !strings.Contains(result, "A.md") {
		t.Fatal("expected first file")
	}
	// Second file should be truncated or missing.
	if strings.Contains(result, strings.Repeat("b", 500)) {
		t.Fatal("second file should have been truncated or excluded")
	}
}

func TestBuildFullSystemPromptMinimal(t *testing.T) {
	dir := t.TempDir()
	paths := makeTestPaths(dir)

	result := BuildFullSystemPrompt(paths, FullPromptOptions{})
	if !strings.Contains(result, "personal AI assistant") {
		t.Fatal("expected identity line")
	}
	if !strings.Contains(result, "Safety") {
		t.Fatal("expected safety section")
	}
	if !strings.Contains(result, "Workspace") {
		t.Fatal("expected workspace section")
	}
	if !strings.Contains(result, "Time") {
		t.Fatal("expected time section")
	}
}

func TestBuildFullSystemPromptWithIdentity(t *testing.T) {
	dir := t.TempDir()
	paths := makeTestPaths(dir)
	writeFile(t, paths.IdentityPath, "- **Name:** Ziggy\n- **Vibe:** chill")

	result := BuildFullSystemPrompt(paths, FullPromptOptions{})
	if !strings.Contains(result, "You are Ziggy") {
		t.Fatal("expected personalized identity line")
	}
	// Workspace file should be injected too.
	if !strings.Contains(result, `<workspace_file path="IDENTITY.md">`) {
		t.Fatal("expected workspace file injection")
	}
}

func TestBuildFullSystemPromptWithTools(t *testing.T) {
	dir := t.TempDir()
	paths := makeTestPaths(dir)

	opts := FullPromptOptions{
		ToolDescriptions: []string{
			"`read_file` — Read a file with line numbers",
			"`exec` — Execute a shell command",
		},
	}
	result := BuildFullSystemPrompt(paths, opts)
	if !strings.Contains(result, "Available Tools") {
		t.Fatal("expected tools section")
	}
	if !strings.Contains(result, "read_file") {
		t.Fatal("expected read_file in tool list")
	}
}

func TestBuildFullSystemPromptWithSkills(t *testing.T) {
	dir := t.TempDir()
	paths := makeTestPaths(dir)

	opts := FullPromptOptions{
		SkillEntries: []string{
			"**web-search**: Search the web for current information",
		},
	}
	result := BuildFullSystemPrompt(paths, opts)
	if !strings.Contains(result, "<available_skills>") {
		t.Fatal("expected skills section with available_skills tag")
	}
	if !strings.Contains(result, "web-search") {
		t.Fatal("expected skill name")
	}
}

func TestBuildFullSystemPromptTimezone(t *testing.T) {
	dir := t.TempDir()
	paths := makeTestPaths(dir)

	opts := FullPromptOptions{Timezone: "America/Mexico_City"}
	result := BuildFullSystemPrompt(paths, opts)
	if !strings.Contains(result, "America/Mexico_City") {
		t.Fatal("expected timezone in prompt")
	}
}

func TestSafetySectionContent(t *testing.T) {
	s := SafetySection()
	if !strings.Contains(s, "Safety") {
		t.Fatal("expected Safety header")
	}
	if !strings.Contains(s, "Never reveal") {
		t.Fatal("expected anti-leak rule")
	}
	if !strings.Contains(s, "prompt injection") {
		t.Fatal("expected prompt injection warning")
	}
}
