package mcp

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/greg0x46/julius/internal/db/dbtest"
)

// Schema keywords whose value must be an object, array or boolean; a null
// there is invalid JSON Schema and strict MCP clients reject the tool.
var schemaKeywordsNeverNull = map[string]bool{
	"$defs": true, "properties": true, "items": true, "required": true,
	"additionalProperties": true, "enum": true, "type": true,
}

func TestToolSchemasHaveNoNullKeywords(t *testing.T) {
	f := newWriteFixture(t, dbtest.OpenSQLite(t))
	listed, err := f.session.ListTools(context.Background(), &sdk.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) == 0 {
		t.Fatal("no tools listed")
	}
	var walk func(tool, path string, v any)
	walk = func(tool, path string, v any) {
		switch node := v.(type) {
		case map[string]any:
			for key, child := range node {
				if child == nil && schemaKeywordsNeverNull[key] {
					t.Errorf("tool %s: %s.%s is null", tool, path, key)
				}
				walk(tool, path+"."+key, child)
			}
		case []any:
			for _, child := range node {
				walk(tool, path, child)
			}
		}
	}
	for _, tool := range listed.Tools {
		for name, schema := range map[string]any{"inputSchema": tool.InputSchema, "outputSchema": tool.OutputSchema} {
			if schema == nil {
				continue
			}
			raw, err := json.Marshal(schema)
			if err != nil {
				t.Fatalf("tool %s: %v", tool.Name, err)
			}
			var decoded any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("tool %s: %v", tool.Name, err)
			}
			walk(tool.Name, name, decoded)
		}
	}
}
