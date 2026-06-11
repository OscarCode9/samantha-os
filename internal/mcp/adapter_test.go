package mcp

import "testing"

func TestParseInputSchemaEmptyIncludesProperties(t *testing.T) {
	schema := parseInputSchema(nil)

	if schema.Type != "object" {
		t.Fatalf("expected object schema, got %q", schema.Type)
	}
	if schema.Properties == nil {
		t.Fatal("expected empty properties map for empty schema")
	}
}

func TestParseInputSchemaInvalidIncludesProperties(t *testing.T) {
	schema := parseInputSchema([]byte("{"))

	if schema.Type != "object" {
		t.Fatalf("expected object schema, got %q", schema.Type)
	}
	if schema.Properties == nil {
		t.Fatal("expected empty properties map for invalid schema fallback")
	}
}
