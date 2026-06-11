package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/session"
)

type Result struct {
	Path string
}

// DetectBootstrapMode returns true if BOOTSTRAP.md exists and is not empty,
// meaning the workspace still needs first-run onboarding.
func DetectBootstrapMode(paths config.Paths) bool {
	data, err := os.ReadFile(paths.BootstrapPath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) != ""
}

// ReadBootstrapInstructions returns the content of BOOTSTRAP.md, or empty
// string if the file does not exist.
func ReadBootstrapInstructions(paths config.Paths) string {
	data, err := os.ReadFile(paths.BootstrapPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// CompleteBootstrap removes BOOTSTRAP.md and writes the completion state
// to .workspace-state.json. Call this when the LLM signals bootstrap is done.
func CompleteBootstrap(paths config.Paths) error {
	// Remove BOOTSTRAP.md.
	if err := os.Remove(paths.BootstrapPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove bootstrap file: %w", err)
	}

	// Write completion state.
	statePath := workspaceStatePath(paths)
	state := map[string]any{
		"bootstrapCompleted": true,
		"completedAt":        time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace state: %w", err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write workspace state: %w", err)
	}
	return nil
}

func workspaceStatePath(paths config.Paths) string {
	return paths.WorkspaceDir + "/.workspace-state.json"
}

func GenerateFirstMessage(paths config.Paths, store *session.Store, explicitMessage string) (Result, error) {
	bootstrapText, err := os.ReadFile(paths.BootstrapPath)
	if err != nil {
		return Result{}, fmt.Errorf("read bootstrap instructions: %w", err)
	}

	message := strings.TrimSpace(explicitMessage)
	if message == "" {
		message = strings.TrimSpace(string(bootstrapText))
	}

	record := session.Record{
		ID:        "bootstrap",
		Kind:      "bootstrap",
		Title:     "First Login Welcome",
		CreatedAt: time.Now().UTC(),
		Messages: []session.Message{
			{
				Role:    "assistant",
				Content: message,
			},
		},
	}

	id, err := store.Save(&record)
	if err != nil {
		return Result{}, err
	}

	return Result{Path: id}, nil
}

func EnsureFirstMessage(paths config.Paths, store *session.Store, explicitMessage string) (Result, bool, error) {
	if _, err := os.Stat(paths.BootstrapPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{}, false, nil
		}
		return Result{}, false, fmt.Errorf("stat bootstrap instructions: %w", err)
	}

	record, err := store.Get("bootstrap")
	if err != nil {
		return Result{}, false, err
	}
	if record != nil {
		return Result{}, false, nil
	}

	result, err := GenerateFirstMessage(paths, store, explicitMessage)
	if err != nil {
		return Result{}, false, err
	}

	return result, true, nil
}
