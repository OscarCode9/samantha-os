package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oscarcode/elementary-claw/internal/config"
)

const (
	CategoryPreference      = "preference"
	CategoryImportantFolder = "important_folder"
	CategoryFrequentApp     = "frequent_app"
	CategoryRoutine         = "routine"
	CategoryBlocked         = "blocked"
	CategoryDailyNote       = "daily_note"
)

type CategorySpec struct {
	Name        string
	Label       string
	FileName    string
	Description string
}

type SaveResult struct {
	Category  string
	Path      string
	Content   string
	Duplicate bool
}

var categorySpecs = []CategorySpec{
	{
		Name:        CategoryPreference,
		Label:       "Preferences",
		FileName:    "preferences.md",
		Description: "Durable user preferences such as tone, response style, and workflow choices.",
	},
	{
		Name:        CategoryImportantFolder,
		Label:       "Important Folders",
		FileName:    "folders.md",
		Description: "Where the user keeps important things such as code, invoices, downloads, or documents.",
	},
	{
		Name:        CategoryFrequentApp,
		Label:       "Frequent Apps",
		FileName:    "apps.md",
		Description: "Apps or tools the user commonly uses and expects the assistant to remember.",
	},
	{
		Name:        CategoryRoutine,
		Label:       "Routines",
		FileName:    "routines.md",
		Description: "Recurring habits, workflows, or repeated tasks the user wants remembered.",
	},
	{
		Name:        CategoryBlocked,
		Label:       "Blocked Things",
		FileName:    "blocked.md",
		Description: "Hard constraints and rules such as asking before deleting or changing things.",
	},
	{
		Name:        CategoryDailyNote,
		Label:       "Daily Note",
		Description: "Short-lived daily context and observations for the current day only.",
	},
}

// Categories returns the canonical memory category names in stable order.
func Categories() []string {
	items := make([]string, 0, len(categorySpecs))
	for _, spec := range categorySpecs {
		items = append(items, spec.Name)
	}
	return items
}

// CategoryFile returns the file path for a canonical memory category.
func CategoryFile(paths config.Paths, category string) (string, error) {
	spec, ok := lookupCategorySpec(category)
	if !ok {
		return "", fmt.Errorf("unknown memory category %q", category)
	}
	if spec.Name == CategoryDailyNote {
		return TodayFile(paths), nil
	}
	return filepath.Join(Dir(paths), spec.FileName), nil
}

// Save stores a memory entry in the appropriate category file.
func Save(paths config.Paths, category string, content string) (SaveResult, error) {
	spec, ok := lookupCategorySpec(category)
	if !ok {
		return SaveResult{}, fmt.Errorf("unknown memory category %q", category)
	}
	entry := normalizeEntry(content)
	if entry == "" {
		return SaveResult{}, fmt.Errorf("memory content must not be empty")
	}
	if err := EnsureDir(paths); err != nil {
		return SaveResult{}, err
	}

	filePath, err := CategoryFile(paths, spec.Name)
	if err != nil {
		return SaveResult{}, err
	}

	existingBytes, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return SaveResult{}, err
	}
	existing := string(existingBytes)
	if entryExists(existing, entry) {
		return SaveResult{Category: spec.Name, Path: filePath, Content: entry, Duplicate: true}, nil
	}

	updated := strings.TrimRight(existing, "\n")
	if updated == "" {
		updated = categoryHeader(spec)
	} else {
		updated += "\n"
	}
	if !strings.HasSuffix(updated, "\n\n") {
		updated += "\n"
	}
	updated += "- " + entry + "\n"

	if err := os.WriteFile(filePath, []byte(updated), 0o600); err != nil {
		return SaveResult{}, err
	}

	return SaveResult{Category: spec.Name, Path: filePath, Content: entry}, nil
}

// Dir returns the memory directory path: {workspace}/memory/
func Dir(paths config.Paths) string {
	return filepath.Join(paths.WorkspaceDir, "memory")
}

// TodayFile returns the path for today's daily notes file.
func TodayFile(paths config.Paths) string {
	return filepath.Join(Dir(paths), time.Now().Format("2006-01-02")+".md")
}

// YesterdayFile returns the path for yesterday's daily notes file.
func YesterdayFile(paths config.Paths) string {
	return filepath.Join(Dir(paths), time.Now().AddDate(0, 0, -1).Format("2006-01-02")+".md")
}

// LongTermFile returns the path to the long-term curated MEMORY.md file in
// the workspace root.
func LongTermFile(paths config.Paths) string {
	return filepath.Join(paths.WorkspaceDir, "MEMORY.md")
}

// EnsureDir creates the memory/ directory if it does not exist.
func EnsureDir(paths config.Paths) error {
	return os.MkdirAll(Dir(paths), 0o700)
}

// ReadRecentContext reads the last N days of daily notes and returns them
// as a single string. Each day's content is prefixed with the date.
func ReadRecentContext(paths config.Paths, days int) string {
	if days <= 0 {
		days = 2
	}

	var parts []string
	now := time.Now()
	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i)
		dateStr := date.Format("2006-01-02")
		filePath := filepath.Join(Dir(paths), dateStr+".md")

		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("### %s\n\n%s", dateStr, trimmed))
	}

	return strings.Join(parts, "\n\n")
}

// ReadLongTerm reads the MEMORY.md long-term memory file.
func ReadLongTerm(paths config.Paths) string {
	content, err := os.ReadFile(LongTermFile(paths))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func ReadCategory(paths config.Paths, category string) string {
	filePath, err := CategoryFile(paths, category)
	if err != nil {
		return ""
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// BuildMemorySection generates the memory instructions and context block
// for inclusion in the system prompt. Returns empty string if there is no
// memory content at all.
func BuildMemorySection(paths config.Paths) string {
	instructions := fmt.Sprintf(`## Memory

You have a persistent local memory rooted at %s.
- Use the save_memory tool for durable user facts instead of write_file.
- Categories:
  - preference -> memory/preferences.md
  - important_folder -> memory/folders.md
  - frequent_app -> memory/apps.md
  - routine -> memory/routines.md
  - blocked -> memory/blocked.md
  - daily_note -> memory/YYYY-MM-DD.md
- Save durable user facts in the category that best matches the request.
- Use daily_note only for short-lived context or observations from today.
- MEMORY.md is legacy long-term memory: read it if present, but prefer the category files for new memories.
- Read today and yesterday's notes at session start for continuity.
- Write important things down — mental notes don't survive restarts.`, Dir(paths))

	recent := ReadRecentContext(paths, 2)
	longTerm := ReadLongTerm(paths)
	categorySections := readCategorySections(paths)

	if recent == "" && longTerm == "" && len(categorySections) == 0 {
		return instructions
	}

	var contextParts []string
	contextParts = append(contextParts, instructions)
	contextParts = append(contextParts, categorySections...)

	if longTerm != "" {
		contextParts = append(contextParts, "### Legacy Long-term Memory\n\n"+longTerm)
	}
	if recent != "" {
		contextParts = append(contextParts, "### Recent Notes\n\n"+recent)
	}

	return strings.Join(contextParts, "\n\n")
}

func lookupCategorySpec(category string) (CategorySpec, bool) {
	switch normalizeCategory(category) {
	case CategoryPreference, "preferences":
		return categorySpecs[0], true
	case CategoryImportantFolder, "folder", "folders", "important_folders":
		return categorySpecs[1], true
	case CategoryFrequentApp, "app", "apps", "frequent_apps":
		return categorySpecs[2], true
	case CategoryRoutine, "routines":
		return categorySpecs[3], true
	case CategoryBlocked, "block", "blocked_actions", "blocked_things":
		return categorySpecs[4], true
	case CategoryDailyNote, "daily_notes", "note", "notes":
		return categorySpecs[5], true
	default:
		return CategorySpec{}, false
	}
}

func normalizeCategory(category string) string {
	category = strings.TrimSpace(strings.ToLower(category))
	category = strings.ReplaceAll(category, "-", "_")
	category = strings.ReplaceAll(category, " ", "_")
	return category
}

func normalizeEntry(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.TrimSpace(strings.Join(strings.Fields(content), " "))
}

func entryExists(content string, entry string) bool {
	if strings.TrimSpace(content) == "" {
		return false
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		if strings.EqualFold(strings.TrimSpace(line), entry) {
			return true
		}
	}
	return false
}

func categoryHeader(spec CategorySpec) string {
	if spec.Name == CategoryDailyNote {
		return "# " + time.Now().Format("2006-01-02")
	}
	return "# " + spec.Label
}

func readCategorySections(paths config.Paths) []string {
	sections := make([]string, 0, len(categorySpecs)-1)
	for _, spec := range categorySpecs {
		if spec.Name == CategoryDailyNote {
			continue
		}
		content := ReadCategory(paths, spec.Name)
		if content == "" {
			continue
		}
		sections = append(sections, "### "+spec.Label+"\n\n"+content)
	}
	return sections
}
