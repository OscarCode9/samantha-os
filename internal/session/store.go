package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/oscarcode/elementary-claw/internal/config"
)

// ToolCall represents a tool invocation requested by the assistant.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the name and JSON-encoded arguments of a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ContentPart represents OpenAI-compatible multimodal message content.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL holds a data URL or remote URL for an image content part.
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// Message represents a chat message with full tool-call support.
// For assistant messages requesting tools: ToolCalls is populated.
// For tool result messages: Role is "tool", ToolCallID references the call, and Name identifies the tool.
type Message struct {
	Role         string        `json:"role"`
	Content      string        `json:"content,omitempty"`
	ContentParts []ContentPart `json:"content_parts,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
	Name         string        `json:"name,omitempty"`
}

type Record struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Messages  []Message `json:"messages"`
}

type Store struct {
	paths config.Paths
}

func NewStore(paths config.Paths) *Store {
	return &Store{paths: paths}
}

func (store *Store) Save(record *Record) (string, error) {
	if err := os.MkdirAll(store.paths.SessionsDir, 0o700); err != nil {
		return "", fmt.Errorf("create sessions directory: %w", err)
	}

	now := time.Now()
	if record.ID == "" {
		record.ID = fmt.Sprintf("%d", now.UnixNano())
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal session %s: %w", record.ID, err)
	}

	path := filepath.Join(store.paths.SessionsDir, record.ID+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write session %s: %w", record.ID, err)
	}

	return record.ID, nil
}

func (store *Store) Get(id string) (*Record, error) {
	path := filepath.Join(store.paths.SessionsDir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session file %s: %w", id, err)
	}

	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("decode session file %s: %w", id, err)
	}

	return &record, nil
}

func (store *Store) List() ([]Record, error) {
	entries, err := os.ReadDir(store.paths.SessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions directory: %w", err)
	}

	items := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(store.paths.SessionsDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read session file %s: %w", entry.Name(), err)
		}

		var record Record
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("decode session file %s: %w", entry.Name(), err)
		}
		items = append(items, record)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	return items, nil
}
