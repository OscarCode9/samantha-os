package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oscarcode/elementary-claw/internal/tools"
)

// mcpTool implements tools.Tool by proxying calls to a remote MCP tool.
type mcpTool struct {
	client       *Client
	info         ToolInfo
	parsedSchema tools.Schema
}

// NewTool wraps a remote MCP ToolInfo into a tools.Tool.  It extracts the
// JSON Schema from the ToolInfo.InputSchema so the LLM gets the proper
// parameter definitions.
func NewTool(client *Client, info ToolInfo) tools.Tool {
	schema := parseInputSchema(info.InputSchema)
	return &mcpTool{
		client:       client,
		info:         info,
		parsedSchema: schema,
	}
}

func (t *mcpTool) Name() string             { return t.info.Name }
func (t *mcpTool) Description() string      { return t.info.Description }
func (t *mcpTool) Parameters() tools.Schema { return t.parsedSchema }

func (t *mcpTool) Execute(ctx context.Context, arguments string) tools.Result {
	text, isErr, err := t.client.CallTool(ctx, t.info.Name, arguments)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("MCP tool %s error: %s", t.info.Name, err))
	}
	if isErr {
		return tools.ErrorResult(text)
	}
	return tools.TextResult(text)
}

// parseInputSchema converts a raw JSON Schema (as returned by tools/list) into
// the tools.Schema used by the registry.  Returns a minimal schema on error.
func parseInputSchema(raw json.RawMessage) tools.Schema {
	if len(raw) == 0 {
		return tools.Schema{
			Type:       "object",
			Properties: map[string]tools.SchemaProperty{},
		}
	}

	var s struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return tools.Schema{
			Type:       "object",
			Properties: map[string]tools.SchemaProperty{},
		}
	}

	schema := tools.Schema{
		Type:       s.Type,
		Required:   s.Required,
		Properties: make(map[string]tools.SchemaProperty, len(s.Properties)),
	}
	if schema.Type == "" {
		schema.Type = "object"
	}

	for name, propRaw := range s.Properties {
		var prop struct {
			Type        string   `json:"type"`
			Description string   `json:"description"`
			Enum        []string `json:"enum"`
		}
		if err := json.Unmarshal(propRaw, &prop); err == nil {
			schema.Properties[name] = tools.SchemaProperty{
				Type:        prop.Type,
				Description: prop.Description,
				Enum:        prop.Enum,
			}
		}
	}
	return schema
}
