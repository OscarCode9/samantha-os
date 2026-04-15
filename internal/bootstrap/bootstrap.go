package bootstrap

import (
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

	path, err := store.Save(record)
	if err != nil {
		return Result{}, err
	}

	return Result{Path: path}, nil
}

func EnsureFirstMessage(paths config.Paths, store *session.Store, explicitMessage string) (Result, bool, error) {
	if _, err := os.Stat(paths.BootstrapPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{}, false, nil
		}
		return Result{}, false, fmt.Errorf("stat bootstrap instructions: %w", err)
	}

	if _, err := store.Get("bootstrap"); err == nil {
		return Result{}, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, false, err
	}

	result, err := GenerateFirstMessage(paths, store, explicitMessage)
	if err != nil {
		return Result{}, false, err
	}

	return result, true, nil
}
