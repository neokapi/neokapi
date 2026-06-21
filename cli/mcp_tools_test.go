package cli

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFrameworkToolInputSchema verifies the projected MCP input schema is an
// object that keeps the tool's own parameters and adds a required `text` field
// plus an optional `target_lang`.
func TestFrameworkToolInputSchema(t *testing.T) {
	s := &schema.ComponentSchema{
		Type: "object",
		Properties: map[string]schema.PropertySchema{
			"prefix": {Type: "string"},
		},
	}
	raw, err := frameworkToolInputSchema(s)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, "object", got["type"])
	props := got["properties"].(map[string]any)
	assert.Contains(t, props, "prefix", "tool's own params preserved")
	assert.Contains(t, props, "text", "text input added")
	assert.Contains(t, props, "target_lang", "target_lang input added")
	assert.Equal(t, []any{"text"}, got["required"], "text is required")
}

// TestFrameworkToolInputSchema_NilSchema handles tools with no schema struct.
func TestFrameworkToolInputSchema_NilSchema(t *testing.T) {
	raw, err := frameworkToolInputSchema(nil)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "object", got["type"])
	props := got["properties"].(map[string]any)
	assert.Contains(t, props, "text")
}

// TestScopeFrameworkTools covers ad-hoc (no project → everything, no default)
// and project mode (filter by allowed source + project target language).
func TestScopeFrameworkTools(t *testing.T) {
	entries := []registry.CLIToolEntry{
		{Info: registry.ToolInfo{Name: "word-count", Source: registry.SourceBuiltIn}},
		{Info: registry.ToolInfo{Name: "bridge-step", Source: "okapi-bridge"}},
	}

	t.Run("ad-hoc exposes everything", func(t *testing.T) {
		scoped, def := scopeFrameworkTools(entries, nil)
		assert.Len(t, scoped, 2)
		assert.Empty(t, def)
	})

	t.Run("project scopes to allowed sources + default lang", func(t *testing.T) {
		ctx := &project.ProjectContext{
			AllowedSources: []string{registry.SourceBuiltIn},
			TargetLocales:  []model.LocaleID{"fr-FR", "de-DE"},
		}
		scoped, def := scopeFrameworkTools(entries, ctx)
		require.Len(t, scoped, 1, "plugin tool filtered out")
		assert.Equal(t, registry.ToolID("word-count"), scoped[0].Info.Name)
		assert.Equal(t, "fr-FR", def)
	})
}

// TestFrameworkMCPHandler_PseudoTranslate runs a real built-in tool end-to-end
// through the untyped MCP handler and asserts the serialized result carries the
// produced target.
func TestFrameworkMCPHandler_PseudoTranslate(t *testing.T) {
	app := &App{}
	app.InitRegistries()

	handler := app.frameworkMCPHandler("pseudo-translate", "")
	args, _ := json.Marshal(map[string]any{"text": "Hello", "target_lang": "qps"})
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: args}}

	res, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, res)

	out, ok := res.StructuredContent.(*frameworkToolOutput)
	require.True(t, ok, "structured content is a frameworkToolOutput")
	assert.Equal(t, "pseudo-translate", out.Tool)
	require.Contains(t, out.Targets, "qps", "produced a qps target")
	assert.NotEmpty(t, out.Targets["qps"], "target was produced")
	assert.Contains(t, out.Targets["qps"], "▒", "pseudo-translate markers present")
}

// TestFrameworkMCPHandler_RequiresText rejects a call with no text.
func TestFrameworkMCPHandler_RequiresText(t *testing.T) {
	app := &App{}
	app.InitRegistries()

	handler := app.frameworkMCPHandler("pseudo-translate", "")
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{}`)}}
	_, err := handler(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "text")
}
