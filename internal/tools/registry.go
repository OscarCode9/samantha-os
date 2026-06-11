package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Tool is the interface all tools must implement.
type Tool interface {
	Name() string
	Description() string
	Parameters() Schema
	Execute(ctx context.Context, arguments string) Result
}

// Schema defines a tool's input parameters as JSON Schema.
type Schema struct {
	Type       string                    `json:"type"`
	Properties map[string]SchemaProperty `json:"properties"`
	Required   []string                  `json:"required,omitempty"`
}

func normalizeSchema(schema Schema) Schema {
	if schema.Type == "" {
		schema.Type = "object"
	}
	if schema.Type == "object" && schema.Properties == nil {
		schema.Properties = map[string]SchemaProperty{}
	}
	return schema
}

// SchemaProperty describes a single parameter in a tool's JSON Schema.
type SchemaProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Items       *SchemaProperty `json:"items,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Minimum     *int     `json:"minimum,omitempty"`
	Maximum     *int     `json:"maximum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

// Definition is the OpenAI-compatible tool definition sent to the LLM.
type Definition struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef describes the function inside a tool definition.
type FunctionDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  Schema `json:"parameters"`
}

// Result is what a tool returns after execution.
type Result struct {
	Content     string       `json:"content"`
	IsError     bool         `json:"is_error,omitempty"`
	Attachments []Attachment `json:"-"`
}

// Attachment is a local file produced by a tool that the runtime may attach
// to the next model turn as multimodal context.
type Attachment struct {
	Type     string
	Path     string
	MimeType string
}

// TextResult creates a successful result with text content.
func TextResult(text string) Result {
	return Result{Content: text}
}

// ErrorResult creates a failed result with an error message.
func ErrorResult(text string) Result {
	return Result{Content: text, IsError: true}
}

// JSONResult creates a successful result with JSON-encoded content.
func JSONResult(v any) Result {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to marshal result: %s", err))
	}
	return TextResult(string(data))
}

// Registry holds all registered tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry. Panics if a tool with the same name
// is already registered.
func (r *Registry) Register(tool Tool) {
	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		panic(fmt.Sprintf("tool %q already registered", name))
	}
	r.tools[name] = tool
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools sorted by name.
func (r *Registry) List() []Tool {
	items := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		items = append(items, t)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name() < items[j].Name()
	})
	return items
}

// Definitions returns OpenAI-compatible tool definitions for all registered
// tools, sorted by name. This is what gets sent in the "tools" array of
// a chat completions request.
func (r *Registry) Definitions() []Definition {
	tools := r.List()
	defs := make([]Definition, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, Definition{
			Type: "function",
			Function: FunctionDef{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  normalizeSchema(t.Parameters()),
			},
		})
	}
	return defs
}

// Count returns the number of registered tools.
func (r *Registry) Count() int {
	return len(r.tools)
}

// ParseArgs is a helper that unmarshals a JSON arguments string into a struct.
func ParseArgs(arguments string, dest any) error {
	if err := json.Unmarshal([]byte(arguments), dest); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}

// TruncateMiddle truncates a string in the middle if it exceeds maxLen,
// preserving the beginning and end for context.
func TruncateMiddle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	marker := "\n\n[...truncated...]\n\n"
	half := (maxLen - len(marker)) / 2
	if half < 0 {
		half = 0
	}
	return s[:half] + marker + s[len(s)-half:]
}
