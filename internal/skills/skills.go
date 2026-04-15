package skills

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Source indicates where a skill was loaded from.
type Source string

const (
	SourceBundled   Source = "bundled"   // shipped with the binary
	SourceManaged   Source = "managed"   // installed via CLI
	SourceWorkspace Source = "workspace" // project-local skills/
)

// Manifest holds the YAML frontmatter metadata from a SKILL.md file.
type Manifest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Homepage    string   `json:"homepage,omitempty"`
	Metadata    Metadata `json:"metadata,omitempty"`
}

// Metadata matches the openclaw.metadata block from SKILL.md frontmatter.
type Metadata struct {
	OpenClaw *OpenClawMeta `json:"openclaw,omitempty"`
}

// OpenClawMeta holds openclaw-specific skill requirements and install hints.
type OpenClawMeta struct {
	Emoji    string        `json:"emoji,omitempty"`
	SkillKey string        `json:"skillKey,omitempty"`
	Requires *Requirements `json:"requires,omitempty"`
	Install  []InstallHint `json:"install,omitempty"`
}

// Requirements declares what the host must provide for the skill to work.
type Requirements struct {
	Bins   []string `json:"bins,omitempty"`   // required binaries on PATH
	Config []string `json:"config,omitempty"` // required config keys
	Env    []string `json:"env,omitempty"`    // required environment variables
}

// InstallHint tells the user how to install a missing dependency.
type InstallHint struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`    // "brew", "apt", "npm", etc.
	Formula string `json:"formula"` // package name
	Bins    []string `json:"bins,omitempty"`
	Label   string `json:"label,omitempty"`
}

// Skill represents a loaded skill with its metadata and instructions.
type Skill struct {
	// Name is the directory name of the skill (e.g. "weather", "github").
	Name string
	// Title is the human-readable title parsed from the SKILL.md header.
	Title string
	// Description is a short description parsed from the SKILL.md.
	Description string
	// Instructions is the full markdown content of SKILL.md, used as the
	// skill's system prompt section.
	Instructions string
	// Path is the absolute path to the skill directory.
	Path string
	// Source indicates where the skill was loaded from.
	Source Source
	// Enabled controls whether the skill is active. Disabled skills are
	// kept in the registry but excluded from combined instructions.
	Enabled bool
	// Manifest holds parsed YAML frontmatter metadata.
	Manifest *Manifest
}

// Registry holds all loaded skills with thread-safe access for hot-reload.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
}

// NewRegistry creates an empty skill registry.
func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]*Skill)}
}

// LoadFromDirectory scans a directory for skill subdirectories. Each
// subdirectory that contains a SKILL.md file is loaded as a skill.
// Returns the number of skills loaded and any error encountered while
// reading the directory itself (individual skill parse errors are skipped).
func (r *Registry) LoadFromDirectory(skillsDir string) (int, error) {
	return r.LoadFromDirectoryWithSource(skillsDir, SourceWorkspace)
}

// LoadFromDirectoryWithSource is like LoadFromDirectory but tags each skill
// with the given source.
func (r *Registry) LoadFromDirectoryWithSource(skillsDir string, source Source) (int, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // no skills directory is fine
		}
		return 0, fmt.Errorf("read skills directory: %w", err)
	}

	loaded := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(skillsDir, entry.Name())
		skillFile := filepath.Join(skillDir, "SKILL.md")

		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue // skip directories without SKILL.md
		}

		skill := parseSkillMarkdown(entry.Name(), skillDir, string(data))
		skill.Source = source
		skill.Enabled = true

		r.mu.Lock()
		r.skills[skill.Name] = skill
		r.mu.Unlock()
		loaded++
	}

	return loaded, nil
}

// LoadMultiSource loads skills from multiple directories with proper source
// tagging and precedence. Later sources override earlier ones by name.
func (r *Registry) LoadMultiSource(dirs map[Source]string) (int, error) {
	// Load in precedence order: bundled < managed < workspace.
	order := []Source{SourceBundled, SourceManaged, SourceWorkspace}
	total := 0
	for _, src := range order {
		dir, ok := dirs[src]
		if !ok || dir == "" {
			continue
		}
		n, err := r.LoadFromDirectoryWithSource(dir, src)
		if err != nil {
			return total, fmt.Errorf("load %s skills: %w", src, err)
		}
		total += n
	}
	return total, nil
}

// Get returns a skill by name.
func (r *Registry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	return s, ok
}

// List returns all loaded skills sorted by name.
func (r *Registry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		items = append(items, s)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

// ListEnabled returns only enabled skills, sorted by name.
func (r *Registry) ListEnabled() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		if s.Enabled {
			items = append(items, s)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

// Count returns the number of loaded skills.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.skills)
}

// Enable activates a skill by name. Returns false if not found.
func (r *Registry) Enable(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[name]
	if !ok {
		return false
	}
	s.Enabled = true
	return true
}

// Disable deactivates a skill by name. It remains in the registry but is
// excluded from CombinedInstructions. Returns false if not found.
func (r *Registry) Disable(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[name]
	if !ok {
		return false
	}
	s.Enabled = false
	return true
}

// Remove deletes a skill from the registry by name. Returns the removed skill
// or nil if not found.
func (r *Registry) Remove(name string) *Skill {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[name]
	if !ok {
		return nil
	}
	delete(r.skills, name)
	return s
}

// Install downloads or copies a skill into the managed skills directory and
// loads it into the registry. The source can be a local path or an HTTP(S) URL.
//
// For local paths the directory is copied. For URLs only a SKILL.md is fetched.
// Returns the installed skill or an error.
func (r *Registry) Install(source string, managedDir string) (*Skill, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return r.installFromURL(source, managedDir)
	}
	return r.installFromPath(source, managedDir)
}

func (r *Registry) installFromURL(rawURL string, managedDir string) (*Skill, error) {
	resp, err := http.Get(rawURL) //nolint:gosec // user-provided URL for skill installation
	if err != nil {
		return nil, fmt.Errorf("fetch skill: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch skill: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("read skill body: %w", err)
	}

	// Derive skill name from the URL path.
	urlPath := strings.TrimSuffix(rawURL, "/SKILL.md")
	parts := strings.Split(urlPath, "/")
	name := parts[len(parts)-1]
	if name == "" {
		return nil, fmt.Errorf("cannot derive skill name from URL %q", rawURL)
	}

	skillDir := filepath.Join(managedDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return nil, fmt.Errorf("create skill directory: %w", err)
	}

	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, data, 0o644); err != nil {
		return nil, fmt.Errorf("write SKILL.md: %w", err)
	}

	skill := parseSkillMarkdown(name, skillDir, string(data))
	skill.Source = SourceManaged
	skill.Enabled = true

	r.mu.Lock()
	r.skills[skill.Name] = skill
	r.mu.Unlock()

	return skill, nil
}

func (r *Registry) installFromPath(srcPath string, managedDir string) (*Skill, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("stat source: %w", err)
	}

	var name string
	var content []byte

	if info.IsDir() {
		name = filepath.Base(srcPath)
		content, err = os.ReadFile(filepath.Join(srcPath, "SKILL.md"))
		if err != nil {
			return nil, fmt.Errorf("read SKILL.md from source: %w", err)
		}
	} else {
		// Single file — assume SKILL.md
		name = filepath.Base(filepath.Dir(srcPath))
		content, err = os.ReadFile(srcPath)
		if err != nil {
			return nil, fmt.Errorf("read source file: %w", err)
		}
	}

	skillDir := filepath.Join(managedDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return nil, fmt.Errorf("create skill directory: %w", err)
	}

	if info.IsDir() {
		if err := copyDir(srcPath, skillDir); err != nil {
			return nil, fmt.Errorf("copy skill directory: %w", err)
		}
	} else {
		dest := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(dest, content, 0o644); err != nil {
			return nil, fmt.Errorf("write SKILL.md: %w", err)
		}
	}

	skill := parseSkillMarkdown(name, skillDir, string(content))
	skill.Source = SourceManaged
	skill.Enabled = true

	r.mu.Lock()
	r.skills[skill.Name] = skill
	r.mu.Unlock()

	return skill, nil
}

// Uninstall removes a skill from the registry and deletes its directory from
// disk. Only managed skills can be uninstalled.
func (r *Registry) Uninstall(name string, managedDir string) error {
	r.mu.Lock()
	s, ok := r.skills[name]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("skill %q not found", name)
	}
	if s.Source != SourceManaged {
		r.mu.Unlock()
		return fmt.Errorf("skill %q is %s, only managed skills can be uninstalled", name, s.Source)
	}
	delete(r.skills, name)
	r.mu.Unlock()

	skillDir := filepath.Join(managedDir, name)
	if err := os.RemoveAll(skillDir); err != nil {
		return fmt.Errorf("remove skill directory: %w", err)
	}
	return nil
}

// Reload clears all skills from source directories and reloads them.
func (r *Registry) Reload(dirs map[Source]string) (int, error) {
	r.mu.Lock()
	r.skills = make(map[string]*Skill)
	r.mu.Unlock()
	return r.LoadMultiSource(dirs)
}

// CheckRequirements verifies that the host satisfies a skill's requirements.
// Returns a list of unmet requirements (empty = all satisfied).
func CheckRequirements(skill *Skill) []string {
	if skill.Manifest == nil || skill.Manifest.Metadata.OpenClaw == nil || skill.Manifest.Metadata.OpenClaw.Requires == nil {
		return nil
	}

	req := skill.Manifest.Metadata.OpenClaw.Requires
	var unmet []string

	for _, bin := range req.Bins {
		if _, err := findExecutable(bin); err != nil {
			unmet = append(unmet, fmt.Sprintf("binary %q not found in PATH", bin))
		}
	}

	for _, envVar := range req.Env {
		if os.Getenv(envVar) == "" {
			unmet = append(unmet, fmt.Sprintf("environment variable %q not set", envVar))
		}
	}

	return unmet
}

// CombinedInstructions returns the combined instructions of all enabled
// skills, separated by horizontal rules. This is what gets injected into
// the system prompt.
func (r *Registry) CombinedInstructions() string {
	items := r.ListEnabled()
	if len(items) == 0 {
		return ""
	}

	var parts []string
	for _, skill := range items {
		if strings.TrimSpace(skill.Instructions) == "" {
			continue
		}
		parts = append(parts, skill.Instructions)
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// ToJSON returns a JSON-safe representation of all skills for API responses.
func (r *Registry) ToJSON() []map[string]any {
	items := r.List()
	result := make([]map[string]any, 0, len(items))
	for _, s := range items {
		entry := map[string]any{
			"name":        s.Name,
			"title":       s.Title,
			"description": s.Description,
			"source":      string(s.Source),
			"enabled":     s.Enabled,
			"path":        s.Path,
		}
		if s.Manifest != nil && s.Manifest.Metadata.OpenClaw != nil {
			entry["emoji"] = s.Manifest.Metadata.OpenClaw.Emoji
		}
		result = append(result, entry)
	}
	return result
}

// parseSkillMarkdown extracts metadata from a SKILL.md file.
// It parses YAML frontmatter (between --- delimiters) as JSON-compatible
// metadata, then extracts the H1 title and first paragraph as description.
func parseSkillMarkdown(name string, dir string, content string) *Skill {
	skill := &Skill{
		Name: name,
		Path: dir,
	}

	body := content
	manifest := parseFrontmatter(content)
	if manifest != nil {
		skill.Manifest = manifest
		if manifest.Name != "" {
			skill.Name = manifest.Name
		}
		if manifest.Description != "" {
			skill.Description = manifest.Description
		}
		// Strip frontmatter from instructions.
		body = stripFrontmatter(content)
	}

	skill.Instructions = strings.TrimSpace(body)

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Extract title from first H1 header.
		if skill.Title == "" && strings.HasPrefix(trimmed, "# ") {
			skill.Title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}

		// Extract description from first non-empty, non-header line after title.
		if skill.Title != "" && skill.Description == "" && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			skill.Description = trimmed
			break
		}
	}

	// Fallback: use directory name as title if no H1 found.
	if skill.Title == "" {
		skill.Title = name
	}

	return skill
}

// parseFrontmatter extracts YAML frontmatter from between --- delimiters.
// Because the OpenClaw SKILL.md files use JSON-in-YAML for complex metadata
// blocks, we parse the metadata field as a nested JSON structure.
func parseFrontmatter(content string) *Manifest {
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return nil
	}

	trimmed := strings.TrimSpace(content)
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return nil
	}

	frontBlock := strings.Join(lines[1:endIdx], "\n")

	manifest := &Manifest{}

	// Parse simple key: value pairs from the YAML.
	for _, line := range strings.Split(frontBlock, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			manifest.Name = cleanYAMLValue(strings.TrimPrefix(line, "name:"))
		} else if strings.HasPrefix(line, "description:") {
			manifest.Description = cleanYAMLValue(strings.TrimPrefix(line, "description:"))
		} else if strings.HasPrefix(line, "homepage:") {
			manifest.Homepage = cleanYAMLValue(strings.TrimPrefix(line, "homepage:"))
		}
	}

	// Try to extract the metadata JSON block. This handles the common pattern:
	// metadata:
	//   { "openclaw": { ... } }
	metaIdx := -1
	for i, line := range strings.Split(frontBlock, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "metadata:") {
			metaIdx = i
			break
		}
	}

	if metaIdx >= 0 {
		metaLines := strings.Split(frontBlock, "\n")[metaIdx:]
		// Find the JSON block within metadata.
		jsonStr := extractJSONBlock(strings.Join(metaLines[1:], "\n"))
		if jsonStr != "" {
			var meta Metadata
			if err := json.Unmarshal([]byte(jsonStr), &meta); err == nil {
				manifest.Metadata = meta
			}
		}
	}

	return manifest
}

// extractJSONBlock finds and extracts a complete JSON object from text.
func extractJSONBlock(text string) string {
	start := strings.Index(text, "{")
	if start < 0 {
		return ""
	}

	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

// stripFrontmatter removes the YAML frontmatter block from content.
func stripFrontmatter(content string) string {
	trimmed := strings.TrimSpace(content)
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return content
	}

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
		}
	}
	return content
}

func cleanYAMLValue(raw string) string {
	s := strings.TrimSpace(raw)
	// Remove surrounding quotes.
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		s = s[1 : len(s)-1]
	}
	return s
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destPath, data, info.Mode())
	})
}

// findExecutable checks if a binary exists on PATH.
func findExecutable(name string) (string, error) {
	path := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(path) {
		full := filepath.Join(dir, name)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full, nil
		}
	}
	return "", fmt.Errorf("%s not found", name)
}
