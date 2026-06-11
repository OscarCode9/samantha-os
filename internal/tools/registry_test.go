package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type stubTool struct {
	name        string
	description string
	result      Result
}

type noArgStubTool struct {
	name        string
	description string
}

func (s *stubTool) Name() string        { return s.name }
func (s *stubTool) Description() string { return s.description }
func (s *stubTool) Parameters() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"input": {Type: "string", Description: "test input"},
		},
		Required: []string{"input"},
	}
}
func (s *stubTool) Execute(ctx context.Context, arguments string) Result {
	return s.result
}

func (s *noArgStubTool) Name() string        { return s.name }
func (s *noArgStubTool) Description() string { return s.description }
func (s *noArgStubTool) Parameters() Schema  { return Schema{Type: "object"} }
func (s *noArgStubTool) Execute(ctx context.Context, arguments string) Result {
	return TextResult("ok")
}

func TestRegistryRegisterAndGet(t *testing.T) {
	registry := NewRegistry()
	tool := &stubTool{name: "test_tool", description: "A test tool"}
	registry.Register(tool)

	got, ok := registry.Get("test_tool")
	if !ok {
		t.Fatal("expected tool to be found")
	}
	if got.Name() != "test_tool" {
		t.Fatalf("unexpected tool name: %s", got.Name())
	}
}

func TestRegistryGetMissing(t *testing.T) {
	registry := NewRegistry()
	_, ok := registry.Get("nonexistent")
	if ok {
		t.Fatal("expected tool not to be found")
	}
}

func TestRegistryCount(t *testing.T) {
	registry := NewRegistry()
	if registry.Count() != 0 {
		t.Fatalf("expected count 0, got %d", registry.Count())
	}
	registry.Register(&stubTool{name: "a"})
	registry.Register(&stubTool{name: "b"})
	if registry.Count() != 2 {
		t.Fatalf("expected count 2, got %d", registry.Count())
	}
}

func TestRegistryListSorted(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&stubTool{name: "zebra"})
	registry.Register(&stubTool{name: "alpha"})
	registry.Register(&stubTool{name: "middle"})

	items := registry.List()
	if len(items) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(items))
	}
	if items[0].Name() != "alpha" || items[1].Name() != "middle" || items[2].Name() != "zebra" {
		t.Fatalf("unexpected order: %s, %s, %s", items[0].Name(), items[1].Name(), items[2].Name())
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&stubTool{name: "dup"})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	registry.Register(&stubTool{name: "dup"})
}

func TestRegistryDefinitions(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&stubTool{name: "my_tool", description: "does stuff"})

	defs := registry.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Type != "function" {
		t.Fatalf("unexpected type: %s", defs[0].Type)
	}
	if defs[0].Function.Name != "my_tool" {
		t.Fatalf("unexpected name: %s", defs[0].Function.Name)
	}
	if defs[0].Function.Description != "does stuff" {
		t.Fatalf("unexpected description: %s", defs[0].Function.Description)
	}
}

func TestRegistryDefinitionsNormalizeNoArgSchemas(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&noArgStubTool{name: "no_args", description: "does not need input"})

	defs := registry.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Function.Parameters.Properties == nil {
		t.Fatal("expected object schema properties to be normalized to an empty map")
	}

	body, err := json.Marshal(defs[0])
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	if !strings.Contains(string(body), `"properties":{}`) {
		t.Fatalf("expected serialized definition to include an empty properties object, got %s", string(body))
	}
}

func TestTextResultAndErrorResult(t *testing.T) {
	tr := TextResult("hello")
	if tr.Content != "hello" || tr.IsError {
		t.Fatalf("unexpected text result: %+v", tr)
	}

	er := ErrorResult("bad")
	if er.Content != "bad" || !er.IsError {
		t.Fatalf("unexpected error result: %+v", er)
	}
}

func TestJSONResult(t *testing.T) {
	r := JSONResult(map[string]string{"key": "value"})
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Content)
	}
	if r.Content == "" {
		t.Fatal("expected non-empty JSON content")
	}
}

func TestParseArgs(t *testing.T) {
	var dest struct {
		Name string `json:"name"`
	}
	if err := ParseArgs(`{"name":"test"}`, &dest); err != nil {
		t.Fatal(err)
	}
	if dest.Name != "test" {
		t.Fatalf("unexpected name: %s", dest.Name)
	}
}

func TestParseArgsInvalid(t *testing.T) {
	var dest struct{}
	if err := ParseArgs("not json", &dest); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestTruncateMiddle(t *testing.T) {
	short := "hello"
	if TruncateMiddle(short, 100) != short {
		t.Fatal("short string should not be truncated")
	}

	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	result := TruncateMiddle(long, 50)
	if len(result) > 60 { // some slack for marker
		t.Fatalf("truncated result too long: %d chars", len(result))
	}
	if !contains(result, "[...truncated...]") {
		t.Fatal("expected truncation marker")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsAt(s, substr)
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
