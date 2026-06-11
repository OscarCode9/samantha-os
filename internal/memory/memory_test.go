package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oscarcode/elementary-claw/internal/config"
)

func makeTestPaths(t *testing.T) config.Paths {
	t.Helper()
	root := t.TempDir()
	stateDir := filepath.Join(root, ".samantha")
	workspaceDir := filepath.Join(stateDir, "workspace")
	os.MkdirAll(workspaceDir, 0o700)

	return config.Paths{
		HomeDir:      root,
		StateDir:     stateDir,
		WorkspaceDir: workspaceDir,
	}
}

func TestDirPath(t *testing.T) {
	paths := makeTestPaths(t)
	d := Dir(paths)
	if !strings.HasSuffix(d, filepath.Join("workspace", "memory")) {
		t.Fatalf("unexpected memory dir: %s", d)
	}
}

func TestTodayFile(t *testing.T) {
	paths := makeTestPaths(t)
	f := TodayFile(paths)
	expected := time.Now().Format("2006-01-02") + ".md"
	if !strings.HasSuffix(f, expected) {
		t.Fatalf("today file unexpected: %s, want suffix %s", f, expected)
	}
}

func TestYesterdayFile(t *testing.T) {
	paths := makeTestPaths(t)
	f := YesterdayFile(paths)
	expected := time.Now().AddDate(0, 0, -1).Format("2006-01-02") + ".md"
	if !strings.HasSuffix(f, expected) {
		t.Fatalf("yesterday file unexpected: %s, want suffix %s", f, expected)
	}
}

func TestLongTermFile(t *testing.T) {
	paths := makeTestPaths(t)
	f := LongTermFile(paths)
	if !strings.HasSuffix(f, "MEMORY.md") {
		t.Fatalf("long-term file unexpected: %s", f)
	}
}

func TestCategoryFile(t *testing.T) {
	paths := makeTestPaths(t)

	cases := map[string]string{
		CategoryPreference:      filepath.Join("memory", "preferences.md"),
		CategoryImportantFolder: filepath.Join("memory", "folders.md"),
		CategoryFrequentApp:     filepath.Join("memory", "apps.md"),
		CategoryRoutine:         filepath.Join("memory", "routines.md"),
		CategoryBlocked:         filepath.Join("memory", "blocked.md"),
	}

	for category, suffix := range cases {
		path, err := CategoryFile(paths, category)
		if err != nil {
			t.Fatalf("category %s returned error: %v", category, err)
		}
		if !strings.HasSuffix(path, suffix) {
			t.Fatalf("category %s unexpected path: %s", category, path)
		}
	}
}

func TestEnsureDir(t *testing.T) {
	paths := makeTestPaths(t)

	if err := EnsureDir(paths); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(Dir(paths))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
}

func TestReadRecentContextEmpty(t *testing.T) {
	paths := makeTestPaths(t)
	result := ReadRecentContext(paths, 2)
	if result != "" {
		t.Fatalf("expected empty, got: %s", result)
	}
}

func TestReadRecentContextWithFiles(t *testing.T) {
	paths := makeTestPaths(t)
	EnsureDir(paths)

	today := time.Now().Format("2006-01-02")
	todayPath := filepath.Join(Dir(paths), today+".md")
	os.WriteFile(todayPath, []byte("Meeting with Carlos at 3pm"), 0o600)

	result := ReadRecentContext(paths, 2)
	if !strings.Contains(result, "Meeting with Carlos") {
		t.Fatalf("expected today's notes, got: %s", result)
	}
	if !strings.Contains(result, today) {
		t.Fatalf("expected date header, got: %s", result)
	}
}

func TestReadLongTermEmpty(t *testing.T) {
	paths := makeTestPaths(t)
	result := ReadLongTerm(paths)
	if result != "" {
		t.Fatalf("expected empty, got: %s", result)
	}
}

func TestReadLongTermWithFile(t *testing.T) {
	paths := makeTestPaths(t)
	os.WriteFile(LongTermFile(paths), []byte("User prefers dark mode.\nFavorite color: blue."), 0o600)

	result := ReadLongTerm(paths)
	if !strings.Contains(result, "dark mode") {
		t.Fatalf("expected long-term content, got: %s", result)
	}
}

func TestSaveCategorizedMemory(t *testing.T) {
	paths := makeTestPaths(t)
	result, err := Save(paths, CategoryImportantFolder, "Mis proyectos de código van en ~/Code")
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "# Important Folders") {
		t.Fatalf("expected header, got: %s", text)
	}
	if !strings.Contains(text, "~/Code") {
		t.Fatalf("expected memory content, got: %s", text)
	}
}

func TestSaveMemoryDeduplicates(t *testing.T) {
	paths := makeTestPaths(t)
	if _, err := Save(paths, CategoryPreference, "Prefiero respuestas técnicas y directas"); err != nil {
		t.Fatal(err)
	}
	result, err := Save(paths, CategoryPreference, "Prefiero respuestas técnicas y directas")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Duplicate {
		t.Fatal("expected duplicate save to be reported")
	}

	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(content), "Prefiero respuestas técnicas y directas") != 1 {
		t.Fatalf("expected one copy of memory entry, got: %s", string(content))
	}
}

func TestBuildMemorySectionNoContent(t *testing.T) {
	paths := makeTestPaths(t)
	section := BuildMemorySection(paths)

	if !strings.Contains(section, "## Memory") {
		t.Fatal("missing Memory header")
	}
	if !strings.Contains(section, "save_memory") {
		t.Fatal("missing save_memory instruction")
	}
	if !strings.Contains(section, "memory/preferences.md") {
		t.Fatal("missing categorized memory instruction")
	}
	// No context sections when no files exist.
	if strings.Contains(section, "### Legacy Long-term Memory") {
		t.Fatal("should not have long-term section without file")
	}
}

func TestBuildMemorySectionWithBoth(t *testing.T) {
	paths := makeTestPaths(t)
	EnsureDir(paths)

	// Write long-term memory.
	os.WriteFile(LongTermFile(paths), []byte("User name: Oscar"), 0o600)
	if _, err := Save(paths, CategoryPreference, "Prefiero respuestas técnicas y directas"); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(paths, CategoryImportantFolder, "Mis facturas van en ~/Documents/Facturas"); err != nil {
		t.Fatal(err)
	}

	// Write today's notes.
	today := time.Now().Format("2006-01-02")
	todayPath := filepath.Join(Dir(paths), today+".md")
	os.WriteFile(todayPath, []byte("Worked on memory system"), 0o600)

	section := BuildMemorySection(paths)

	if !strings.Contains(section, "## Memory") {
		t.Fatal("missing Memory header")
	}
	if !strings.Contains(section, "### Preferences") {
		t.Fatal("missing preferences section")
	}
	if !strings.Contains(section, "### Important Folders") {
		t.Fatal("missing folders section")
	}
	if !strings.Contains(section, "### Legacy Long-term Memory") {
		t.Fatal("missing long-term section")
	}
	if !strings.Contains(section, "Oscar") {
		t.Fatal("missing long-term content")
	}
	if !strings.Contains(section, "respuestas técnicas y directas") {
		t.Fatal("missing preference content")
	}
	if !strings.Contains(section, "~/Documents/Facturas") {
		t.Fatal("missing folder content")
	}
	if !strings.Contains(section, "### Recent Notes") {
		t.Fatal("missing recent notes section")
	}
	if !strings.Contains(section, "Worked on memory system") {
		t.Fatal("missing today's notes content")
	}
}
