package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// ---- parseSkillMarkdown / parseFrontmatter ----

func TestParseSkillMarkdownBasic(t *testing.T) {
	content := "# Weather\n\nGet weather forecasts.\n\n## Usage\n\nAsk for weather."
	skill := parseSkillMarkdown("weather", "/tmp/weather", content)

	if skill.Name != "weather" {
		t.Errorf("expected name %q, got %q", "weather", skill.Name)
	}
	if skill.Title != "Weather" {
		t.Errorf("expected title %q, got %q", "Weather", skill.Title)
	}
	if skill.Description != "Get weather forecasts." {
		t.Errorf("expected description %q, got %q", "Get weather forecasts.", skill.Description)
	}
	if skill.Instructions == "" {
		t.Error("expected non-empty instructions")
	}
}

func TestParseSkillMarkdownWithFrontmatter(t *testing.T) {
	content := `---
name: 1password
description: Set up and use 1Password CLI (op).
homepage: https://developer.1password.com/docs/cli/get-started/
metadata:
  {
    "openclaw":
      {
        "emoji": "🔐",
        "requires": { "bins": ["op"] },
        "install":
          [
            {
              "id": "brew",
              "kind": "brew",
              "formula": "1password-cli",
              "bins": ["op"],
              "label": "Install 1Password CLI (brew)"
            }
          ]
      }
  }
---

# 1Password CLI

Follow the official CLI get-started steps.`

	skill := parseSkillMarkdown("onepassword", "/tmp/1password", content)

	if skill.Name != "1password" {
		t.Errorf("expected name %q from frontmatter, got %q", "1password", skill.Name)
	}
	if skill.Title != "1Password CLI" {
		t.Errorf("expected title %q, got %q", "1Password CLI", skill.Title)
	}
	if skill.Description != "Set up and use 1Password CLI (op)." {
		t.Errorf("expected description from frontmatter, got %q", skill.Description)
	}
	if skill.Manifest == nil {
		t.Fatal("expected manifest to be parsed")
	}
	if skill.Manifest.Homepage != "https://developer.1password.com/docs/cli/get-started/" {
		t.Errorf("expected homepage, got %q", skill.Manifest.Homepage)
	}
	if skill.Manifest.Metadata.OpenClaw == nil {
		t.Fatal("expected openclaw metadata")
	}
	if skill.Manifest.Metadata.OpenClaw.Emoji != "🔐" {
		t.Errorf("expected emoji 🔐, got %q", skill.Manifest.Metadata.OpenClaw.Emoji)
	}
	if skill.Manifest.Metadata.OpenClaw.Requires == nil || len(skill.Manifest.Metadata.OpenClaw.Requires.Bins) != 1 {
		t.Fatal("expected requires.bins to have 1 entry")
	}
	if skill.Manifest.Metadata.OpenClaw.Requires.Bins[0] != "op" {
		t.Errorf("expected bin %q, got %q", "op", skill.Manifest.Metadata.OpenClaw.Requires.Bins[0])
	}
	if len(skill.Manifest.Metadata.OpenClaw.Install) != 1 {
		t.Fatalf("expected 1 install hint, got %d", len(skill.Manifest.Metadata.OpenClaw.Install))
	}
	if skill.Manifest.Metadata.OpenClaw.Install[0].Kind != "brew" {
		t.Errorf("expected install kind %q, got %q", "brew", skill.Manifest.Metadata.OpenClaw.Install[0].Kind)
	}
}

func TestParseSkillMarkdownNoTitle(t *testing.T) {
	content := "This skill does things."
	skill := parseSkillMarkdown("fallback", "/tmp/fallback", content)

	if skill.Title != "fallback" {
		t.Errorf("expected fallback title %q, got %q", "fallback", skill.Title)
	}
}

func TestStripFrontmatter(t *testing.T) {
	content := "---\nname: test\n---\n\n# Test\n\nBody here."
	stripped := stripFrontmatter(content)
	if stripped != "# Test\n\nBody here." {
		t.Errorf("unexpected stripped content: %q", stripped)
	}
}

func TestStripFrontmatterNoFrontmatter(t *testing.T) {
	content := "# Plain\n\nNo frontmatter."
	stripped := stripFrontmatter(content)
	if stripped != content {
		t.Errorf("expected content unchanged, got %q", stripped)
	}
}

func TestExtractJSONBlock(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`  { "a": 1 }  `, `{ "a": 1 }`},
		{`  { "a": { "b": 2 } }  `, `{ "a": { "b": 2 } }`},
		{`no json here`, ""},
		{`{ "unclosed": true`, ""},
	}
	for _, tc := range tests {
		got := extractJSONBlock(tc.input)
		if got != tc.expected {
			t.Errorf("extractJSONBlock(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// ---- Registry core operations ----

func TestRegistryLoadFromDirectory(t *testing.T) {
	dir := t.TempDir()
	createTestSkill(t, dir, "alpha", "# Alpha\n\nFirst skill.")
	createTestSkill(t, dir, "beta", "# Beta\n\nSecond skill.")

	registry := NewRegistry()
	n, err := registry.LoadFromDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 skills loaded, got %d", n)
	}
	if registry.Count() != 2 {
		t.Errorf("expected count 2, got %d", registry.Count())
	}
}

func TestRegistryLoadFromDirectoryMissing(t *testing.T) {
	registry := NewRegistry()
	n, err := registry.LoadFromDirectory("/nonexistent-path-xyz")
	if err != nil {
		t.Fatal("expected no error for missing dir")
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestRegistryLoadFromDirectorySkipsNoSkillMd(t *testing.T) {
	dir := t.TempDir()
	createTestSkill(t, dir, "valid", "# Valid\n\nHas SKILL.md.")
	// Create a dir without SKILL.md
	os.MkdirAll(filepath.Join(dir, "empty"), 0o755)

	registry := NewRegistry()
	n, err := registry.LoadFromDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 skill, got %d", n)
	}
}

func TestRegistryGetAndList(t *testing.T) {
	dir := t.TempDir()
	createTestSkill(t, dir, "charlie", "# Charlie\n\nThird.")
	createTestSkill(t, dir, "alpha", "# Alpha\n\nFirst.")

	registry := NewRegistry()
	registry.LoadFromDirectory(dir)

	s, ok := registry.Get("alpha")
	if !ok || s.Title != "Alpha" {
		t.Errorf("expected to find alpha skill")
	}

	_, ok = registry.Get("nonexistent")
	if ok {
		t.Error("expected not found")
	}

	items := registry.List()
	if len(items) != 2 {
		t.Fatalf("expected 2, got %d", len(items))
	}
	// Should be sorted
	if items[0].Name != "alpha" || items[1].Name != "charlie" {
		t.Errorf("expected sorted order, got %s, %s", items[0].Name, items[1].Name)
	}
}

// ---- Enable / Disable ----

func TestRegistryEnableDisable(t *testing.T) {
	dir := t.TempDir()
	createTestSkill(t, dir, "test-skill", "# Test\n\nTest body.")

	registry := NewRegistry()
	registry.LoadFromDirectory(dir)

	s, _ := registry.Get("test-skill")
	if !s.Enabled {
		t.Fatal("expected enabled by default")
	}

	if !registry.Disable("test-skill") {
		t.Fatal("expected disable to return true")
	}
	s, _ = registry.Get("test-skill")
	if s.Enabled {
		t.Fatal("expected disabled")
	}

	// Disabled skills excluded from ListEnabled
	enabled := registry.ListEnabled()
	if len(enabled) != 0 {
		t.Errorf("expected 0 enabled, got %d", len(enabled))
	}

	// Disabled skills excluded from CombinedInstructions
	if registry.CombinedInstructions() != "" {
		t.Error("expected empty combined instructions")
	}

	if !registry.Enable("test-skill") {
		t.Fatal("expected enable to return true")
	}
	if len(registry.ListEnabled()) != 1 {
		t.Error("expected 1 enabled after re-enable")
	}
}

func TestRegistryEnableDisableNotFound(t *testing.T) {
	registry := NewRegistry()
	if registry.Enable("nope") {
		t.Error("expected false for missing skill")
	}
	if registry.Disable("nope") {
		t.Error("expected false for missing skill")
	}
}

// ---- Remove ----

func TestRegistryRemove(t *testing.T) {
	dir := t.TempDir()
	createTestSkill(t, dir, "removeme", "# Remove\n\nRemovable.")

	registry := NewRegistry()
	registry.LoadFromDirectory(dir)

	s := registry.Remove("removeme")
	if s == nil || s.Name != "removeme" {
		t.Fatal("expected removed skill")
	}
	if registry.Count() != 0 {
		t.Errorf("expected 0 after remove, got %d", registry.Count())
	}
	if registry.Remove("removeme") != nil {
		t.Error("expected nil for already-removed skill")
	}
}

// ---- Multi-source loading ----

func TestRegistryLoadMultiSource(t *testing.T) {
	bundled := t.TempDir()
	managed := t.TempDir()
	workspace := t.TempDir()

	createTestSkill(t, bundled, "shared", "# Bundled Shared\n\nBundled version.")
	createTestSkill(t, bundled, "only-bundled", "# Only Bundled\n\nBundled only.")
	createTestSkill(t, managed, "shared", "# Managed Shared\n\nManaged version.")
	createTestSkill(t, workspace, "shared", "# Workspace Shared\n\nWorkspace version.")

	registry := NewRegistry()
	n, err := registry.LoadMultiSource(map[Source]string{
		SourceBundled:   bundled,
		SourceManaged:   managed,
		SourceWorkspace: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}

	// shared should be loaded 3 times, but workspace wins (last)
	s, ok := registry.Get("shared")
	if !ok {
		t.Fatal("expected shared skill")
	}
	if s.Source != SourceWorkspace {
		t.Errorf("expected workspace source, got %s", s.Source)
	}
	if s.Title != "Workspace Shared" {
		t.Errorf("expected workspace title, got %q", s.Title)
	}

	_, ok = registry.Get("only-bundled")
	if !ok {
		t.Fatal("expected only-bundled skill")
	}

	// n counts every load (including overwrites)
	if n != 4 {
		t.Errorf("expected 4 total loads, got %d", n)
	}
}

// ---- Install from path ----

func TestRegistryInstallFromPath(t *testing.T) {
	src := t.TempDir()
	managed := t.TempDir()
	createTestSkill(t, src, "new-skill", "# New Skill\n\nFresh install.")

	registry := NewRegistry()
	installed, err := registry.Install(filepath.Join(src, "new-skill"), managed)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Name != "new-skill" {
		t.Errorf("expected name %q, got %q", "new-skill", installed.Name)
	}
	if installed.Source != SourceManaged {
		t.Errorf("expected managed source, got %s", installed.Source)
	}

	// Verify file was copied to managed dir
	copiedFile := filepath.Join(managed, "new-skill", "SKILL.md")
	if _, err := os.Stat(copiedFile); err != nil {
		t.Errorf("expected SKILL.md in managed dir: %v", err)
	}

	// Verify it's in the registry
	s, ok := registry.Get("new-skill")
	if !ok {
		t.Fatal("expected installed skill in registry")
	}
	if s.Title != "New Skill" {
		t.Errorf("expected title %q, got %q", "New Skill", s.Title)
	}
}

// ---- Uninstall ----

func TestRegistryUninstall(t *testing.T) {
	src := t.TempDir()
	managed := t.TempDir()
	createTestSkill(t, src, "bye-skill", "# Bye\n\nGoodbye.")

	registry := NewRegistry()
	registry.Install(filepath.Join(src, "bye-skill"), managed)

	err := registry.Uninstall("bye-skill", managed)
	if err != nil {
		t.Fatal(err)
	}

	if registry.Count() != 0 {
		t.Errorf("expected 0 after uninstall, got %d", registry.Count())
	}

	// Verify directory was deleted
	removedDir := filepath.Join(managed, "bye-skill")
	if _, err := os.Stat(removedDir); !os.IsNotExist(err) {
		t.Error("expected skill directory to be deleted")
	}
}

func TestRegistryUninstallNonManaged(t *testing.T) {
	dir := t.TempDir()
	createTestSkill(t, dir, "workspace-skill", "# WS\n\nWorkspace.")

	registry := NewRegistry()
	registry.LoadFromDirectoryWithSource(dir, SourceWorkspace)

	err := registry.Uninstall("workspace-skill", dir)
	if err == nil {
		t.Fatal("expected error uninstalling non-managed skill")
	}
}

func TestRegistryUninstallNotFound(t *testing.T) {
	registry := NewRegistry()
	err := registry.Uninstall("ghost", t.TempDir())
	if err == nil {
		t.Fatal("expected error uninstalling non-existent skill")
	}
}

// ---- Reload ----

func TestRegistryReload(t *testing.T) {
	dir := t.TempDir()
	createTestSkill(t, dir, "original", "# Original\n\nFirst load.")

	registry := NewRegistry()
	registry.LoadFromDirectory(dir)
	if registry.Count() != 1 {
		t.Fatal("expected 1 skill")
	}

	// Add another skill to disk
	createTestSkill(t, dir, "added", "# Added\n\nNew.")

	// Reload
	dirs := map[Source]string{SourceWorkspace: dir}
	n, err := registry.Reload(dirs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 after reload, got %d", n)
	}
	if registry.Count() != 2 {
		t.Errorf("expected count 2, got %d", registry.Count())
	}
}

// ---- CombinedInstructions ----

func TestCombinedInstructions(t *testing.T) {
	dir := t.TempDir()
	createTestSkill(t, dir, "a", "# A\n\nInstructions A.")
	createTestSkill(t, dir, "b", "# B\n\nInstructions B.")

	registry := NewRegistry()
	registry.LoadFromDirectory(dir)

	combined := registry.CombinedInstructions()
	if combined == "" {
		t.Fatal("expected non-empty combined instructions")
	}
	if !containsSubstring(combined, "Instructions A") || !containsSubstring(combined, "Instructions B") {
		t.Error("expected both skill instructions in combined output")
	}
}

func TestCombinedInstructionsEmpty(t *testing.T) {
	registry := NewRegistry()
	if registry.CombinedInstructions() != "" {
		t.Error("expected empty combined instructions")
	}
}

// ---- CheckRequirements ----

func TestCheckRequirementsNoManifest(t *testing.T) {
	skill := &Skill{Name: "plain"}
	unmet := CheckRequirements(skill)
	if len(unmet) != 0 {
		t.Errorf("expected no unmet requirements, got %v", unmet)
	}
}

func TestCheckRequirementsMissingBin(t *testing.T) {
	skill := &Skill{
		Name: "needs-bin",
		Manifest: &Manifest{
			Metadata: Metadata{
				OpenClaw: &OpenClawMeta{
					Requires: &Requirements{
						Bins: []string{"nonexistent-binary-xyz-12345"},
					},
				},
			},
		},
	}
	unmet := CheckRequirements(skill)
	if len(unmet) != 1 {
		t.Fatalf("expected 1 unmet requirement, got %d: %v", len(unmet), unmet)
	}
}

func TestCheckRequirementsMissingEnv(t *testing.T) {
	skill := &Skill{
		Name: "needs-env",
		Manifest: &Manifest{
			Metadata: Metadata{
				OpenClaw: &OpenClawMeta{
					Requires: &Requirements{
						Env: []string{"NONEXISTENT_ENV_VAR_XYZ_12345"},
					},
				},
			},
		},
	}
	unmet := CheckRequirements(skill)
	if len(unmet) != 1 {
		t.Fatalf("expected 1 unmet requirement, got %d: %v", len(unmet), unmet)
	}
}

// ---- ToJSON ----

func TestRegistryToJSON(t *testing.T) {
	dir := t.TempDir()
	createTestSkill(t, dir, "jsontest", "# JSON Test\n\nJSON body.")

	registry := NewRegistry()
	registry.LoadFromDirectory(dir)

	result := registry.ToJSON()
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0]["name"] != "jsontest" {
		t.Errorf("expected name %q, got %v", "jsontest", result[0]["name"])
	}
	if result[0]["enabled"] != true {
		t.Error("expected enabled=true")
	}
}

// ---- Helpers ----

func createTestSkill(t *testing.T, dir, name, content string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
