package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Live tests hit the real GitHub Copilot API.
// Run with: COPILOT_GITHUB_TOKEN=ghu_xxx go test ./internal/memory/ -run TestLive -v -timeout 120s

func skipIfNoToken(t *testing.T) {
	t.Helper()
	if os.Getenv("COPILOT_GITHUB_TOKEN") == "" {
		t.Skip("COPILOT_GITHUB_TOKEN not set — skipping live test")
	}
}

// TestLiveMemoryDailyNotes verifies that daily notes are stored and retrieved
// correctly by the memory system.
func TestLiveMemoryDailyNotes(t *testing.T) {
	skipIfNoToken(t)

	paths := makeTestPaths(t)
	EnsureDir(paths)

	today := time.Now().Format("2006-01-02")
	todayPath := filepath.Join(Dir(paths), today+".md")
	os.WriteFile(todayPath, []byte("Meeting with Carlos at 3pm tomorrow."), 0o600)

	// Verify the notes show up in recent context.
	recent := ReadRecentContext(paths, 2)
	if !strings.Contains(recent, "Meeting with Carlos") {
		t.Fatalf("daily notes not in recent context: %s", recent)
	}
	if !strings.Contains(recent, today) {
		t.Fatalf("date header missing from recent context: %s", recent)
	}
}

// TestLiveLongTermMemory verifies that MEMORY.md is read and included in
// the memory section.
func TestLiveLongTermMemory(t *testing.T) {
	skipIfNoToken(t)

	paths := makeTestPaths(t)
	os.WriteFile(LongTermFile(paths), []byte("User prefers dark mode. Favorite color: blue."), 0o600)

	section := BuildMemorySection(paths)
	if !strings.Contains(section, "dark mode") {
		t.Fatalf("long-term memory not in section: %s", section)
	}
	if !strings.Contains(section, "blue") {
		t.Fatalf("favorite color not in section: %s", section)
	}

	t.Logf("Memory section length: %d chars", len(section))
}
