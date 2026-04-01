package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaRegistry_LoadFromDirectory(t *testing.T) {
	// Create a temp directory with test schemas
	dir := t.TempDir()

	// Create a test schema file
	schema := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"$id": "https://neokapi.dev/schemas/filters/okf_json.schema.json",
		"$version": "1.0.0",
		"title": "JSON Filter",
		"description": "Configuration for the Okapi JSON Filter",
		"type": "object",
		"formatMeta": {
			"id": "okf_json",
			"class": "net.sf.okapi.filters.json.JSONFilter",
			"extensions": [".json"],
			"mimeTypes": ["application/json"]
		},
		"ui:groups": [
			{
				"id": "extraction",
				"label": "Extraction Settings",
				"collapsed": false,
				"fields": ["extractAllPairs", "extractionRules"]
			}
		],
		"properties": {
			"extractAllPairs": {
				"type": "boolean",
				"default": true,
				"description": "Extract all key-value pairs"
			},
			"extractionRules": {
				"type": "string",
				"default": "",
				"ui:widget": "regexBuilder"
			},
			"useCodeFinder": {
				"type": "boolean",
				"default": false
			}
		}
	}`

	err := os.WriteFile(filepath.Join(dir, "okf_json.schema.json"), []byte(schema), 0644)
	require.NoError(t, err)

	// Load schemas
	reg := NewSchemaRegistry()
	err = reg.LoadFromDirectory(dir)
	require.NoError(t, err)

	// Verify schema was loaded
	assert.Equal(t, 1, reg.Count())
	assert.True(t, reg.HasSchema("okf_json"))

	// Get schema and verify contents
	s, ok := reg.GetSchema("okf_json")
	require.True(t, ok)
	assert.Equal(t, "JSON Filter", s.Title)
	assert.Equal(t, "1.0.0", s.Version)
	assert.Equal(t, "okf_json", s.FormatMeta.ID)
	assert.Equal(t, "net.sf.okapi.filters.json.JSONFilter", s.FormatMeta.Class)
	assert.Contains(t, s.FormatMeta.Extensions, ".json")
	assert.Contains(t, s.FormatMeta.MimeTypes, "application/json")

	// Verify properties
	assert.Len(t, s.Properties, 3)
	assert.Equal(t, "boolean", s.Properties["extractAllPairs"].Type)
	assert.Equal(t, true, s.Properties["extractAllPairs"].Default)
	assert.Equal(t, "regexBuilder", s.Properties["extractionRules"].Widget)

	// Verify groups
	assert.Len(t, s.Groups, 1)
	assert.Equal(t, "extraction", s.Groups[0].ID)
	assert.Contains(t, s.Groups[0].Fields, "extractAllPairs")

	// Test raw JSON access
	rawJSON, ok := reg.GetSchemaJSON("okf_json")
	assert.True(t, ok)
	assert.NotEmpty(t, rawJSON)
}

func TestSchemaRegistry_GetSchemaExactMatch(t *testing.T) {
	dir := t.TempDir()

	schema := `{
		"$version": "1.0.0",
		"title": "HTML Filter",
		"type": "object",
		"formatMeta": { "id": "okf_html", "class": "HtmlFilter", "extensions": [], "mimeTypes": [] },
		"properties": {}
	}`

	err := os.WriteFile(filepath.Join(dir, "okf_html.schema.json"), []byte(schema), 0644)
	require.NoError(t, err)

	reg := NewSchemaRegistry()
	err = reg.LoadFromDirectory(dir)
	require.NoError(t, err)

	// Should find with exact match
	_, ok := reg.GetSchema("okf_html")
	assert.True(t, ok)

	// Should NOT find without prefix — distinct formats
	_, ok = reg.GetSchema("html")
	assert.False(t, ok)
}

func TestSchemaRegistry_ValidateParams(t *testing.T) {
	dir := t.TempDir()

	schema := `{
		"$version": "1.0.0",
		"title": "Test Filter",
		"type": "object",
		"formatMeta": { "id": "okf_test", "class": "Test", "extensions": [], "mimeTypes": [] },
		"properties": {
			"enabled": { "type": "boolean" },
			"count": { "type": "integer" },
			"name": { "type": "string" }
		}
	}`

	err := os.WriteFile(filepath.Join(dir, "okf_test.schema.json"), []byte(schema), 0644)
	require.NoError(t, err)

	reg := NewSchemaRegistry()
	err = reg.LoadFromDirectory(dir)
	require.NoError(t, err)

	// Valid params
	err = reg.ValidateParams("okf_test", map[string]any{
		"enabled": true,
		"count":   42,
		"name":    "test",
	})
	assert.NoError(t, err)

	// Unknown parameter
	err = reg.ValidateParams("okf_test", map[string]any{
		"unknown": "value",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter")

	// Wrong type
	err = reg.ValidateParams("okf_test", map[string]any{
		"enabled": "not a boolean",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected boolean")

	// No schema - should pass
	err = reg.ValidateParams("okf_nonexistent", map[string]any{
		"anything": "goes",
	})
	assert.NoError(t, err)
}

func TestSchemaRegistry_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	reg := NewSchemaRegistry()
	err := reg.LoadFromDirectory(dir)
	require.NoError(t, err)
	assert.Equal(t, 0, reg.Count())
}

func TestSchemaRegistry_NonexistentDirectory(t *testing.T) {
	reg := NewSchemaRegistry()
	err := reg.LoadFromDirectory("/nonexistent/path")
	require.NoError(t, err) // Should not error, just load nothing
	assert.Equal(t, 0, reg.Count())
}

func TestSchemaRegistry_ListFormats(t *testing.T) {
	dir := t.TempDir()

	// Create two schema files
	for _, id := range []string{"okf_json", "okf_html"} {
		schema := `{
			"$version": "1.0.0",
			"title": "Filter",
			"type": "object",
			"formatMeta": { "id": "` + id + `", "class": "Filter", "extensions": [], "mimeTypes": [] },
			"properties": {}
		}`
		err := os.WriteFile(filepath.Join(dir, id+".schema.json"), []byte(schema), 0644)
		require.NoError(t, err)
	}

	reg := NewSchemaRegistry()
	err := reg.LoadFromDirectory(dir)
	require.NoError(t, err)

	filters := reg.ListFormats()
	assert.Len(t, filters, 2)
}

// TestSchemaRegistry_LoadCompositeSchemas verifies loading schemas in the
// composite format that okapi-bridge produces — with x-format metadata,
// configurations, and multiple properties.
func TestSchemaRegistry_LoadCompositeSchemas(t *testing.T) {
	dir := t.TempDir()

	// Simulate a composite schema like those produced by okapi-bridge's
	// SchemaGenerator: includes x-format with class/extensions/mimeTypes,
	// multiple property types, and x-groups for UI.
	htmlSchema := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"$id": "https://neokapi.dev/schemas/filters/okf_html.schema.json",
		"$version": "1.47.0",
		"title": "HTML Filter",
		"description": "Configuration for the Okapi HTML Filter",
		"type": "object",
		"formatMeta": {
			"id": "okf_html",
			"class": "net.sf.okapi.filters.html.HtmlFilter",
			"extensions": [".html", ".htm"],
			"mimeTypes": ["text/html"]
		},
		"ui:groups": [
			{
				"id": "general",
				"label": "General Settings",
				"fields": ["assumeWellformed", "useCodeFinder"]
			}
		],
		"properties": {
			"assumeWellformed": {
				"type": "boolean",
				"default": false,
				"description": "Assume well-formed HTML"
			},
			"useCodeFinder": {
				"type": "boolean",
				"default": true
			},
			"codeFinderRules": {
				"type": "string",
				"default": "",
				"ui:widget": "regexBuilder"
			}
		}
	}`

	xmlSchema := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"$id": "https://neokapi.dev/schemas/filters/okf_xml.schema.json",
		"$version": "1.47.0",
		"title": "XML Filter",
		"description": "Configuration for the Okapi XML Filter",
		"type": "object",
		"formatMeta": {
			"id": "okf_xml",
			"class": "net.sf.okapi.filters.xml.XMLFilter",
			"extensions": [".xml"],
			"mimeTypes": ["application/xml", "text/xml"]
		},
		"properties": {
			"preserveWhitespace": {
				"type": "boolean",
				"default": false
			}
		}
	}`

	require.NoError(t, os.WriteFile(filepath.Join(dir, "okf_html.schema.json"), []byte(htmlSchema), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "okf_xml.schema.json"), []byte(xmlSchema), 0644))

	reg := NewSchemaRegistry()
	require.NoError(t, reg.LoadFromDirectory(dir))

	assert.Equal(t, 2, reg.Count())

	// Verify HTML schema metadata
	html, ok := reg.GetSchema("okf_html")
	require.True(t, ok)
	assert.Equal(t, "HTML Filter", html.Title)
	assert.Equal(t, "1.47.0", html.Version)
	assert.Equal(t, "net.sf.okapi.filters.html.HtmlFilter", html.FormatMeta.Class)
	assert.Contains(t, html.FormatMeta.Extensions, ".html")
	assert.Contains(t, html.FormatMeta.Extensions, ".htm")
	assert.Contains(t, html.FormatMeta.MimeTypes, "text/html")
	assert.Len(t, html.Properties, 3)
	assert.Len(t, html.Groups, 1)

	// Verify XML schema
	xml, ok := reg.GetSchema("okf_xml")
	require.True(t, ok)
	assert.Equal(t, "net.sf.okapi.filters.xml.XMLFilter", xml.FormatMeta.Class)
	assert.Contains(t, xml.FormatMeta.MimeTypes, "application/xml")

	// Verify configuration extraction: ValidateParams should accept valid params
	err := reg.ValidateParams("okf_html", map[string]any{
		"assumeWellformed": true,
		"useCodeFinder":    false,
	})
	assert.NoError(t, err)
}

// TestSchemaRegistry_ExtractPresets verifies that filter configurations
// in x-format.configurations are correctly extracted into the PresetRegistry.
func TestSchemaRegistry_ExtractPresets(t *testing.T) {
	dir := t.TempDir()

	schema := `{
		"$version": "1.47.0",
		"title": "HTML Filter",
		"type": "object",
		"formatMeta": {
			"id": "okf_html",
			"class": "net.sf.okapi.filters.html.HtmlFilter",
			"extensions": [".html"],
			"mimeTypes": ["text/html"]
		},
		"presets": {
			"wellFormed": {
				"assumeWellformed": true,
				"useCodeFinder": false
			},
			"default": {
				"assumeWellformed": false,
				"useCodeFinder": true
			}
		},
		"properties": {
			"assumeWellformed": { "type": "boolean", "default": false },
			"useCodeFinder": { "type": "boolean", "default": true }
		}
	}`

	require.NoError(t, os.WriteFile(filepath.Join(dir, "okf_html.schema.json"), []byte(schema), 0644))

	schemaReg := NewSchemaRegistry()
	require.NoError(t, schemaReg.LoadFromDirectory(dir))

	presetReg := preset.NewPresetRegistry()
	schemaReg.ExtractPresets(presetReg)

	// Should have registered presets for okf_html.
	presets := presetReg.ListFormatPresets("okf_html")
	require.Len(t, presets, 2)

	// Verify the "wellFormed" preset.
	wf := presetReg.GetFormatPreset("okf_html", "wellFormed")
	require.NotNil(t, wf)
	assert.Equal(t, "wellFormed", wf.Name)
	assert.Equal(t, "okf_html", wf.Format)
	assert.Equal(t, "bridge", wf.Source)
	assert.Equal(t, true, wf.Config["assumeWellformed"])
	assert.Equal(t, false, wf.Config["useCodeFinder"])

	// Verify the "default" preset.
	def := presetReg.GetFormatPreset("okf_html", "default")
	require.NotNil(t, def)
	assert.Equal(t, "bridge", def.Source)
}

// TestSchemaRegistry_RegisterSchema verifies programmatic schema registration.
func TestSchemaRegistry_RegisterSchema(t *testing.T) {
	reg := NewSchemaRegistry()

	reg.RegisterSchema("json", &FormatSchema{
		Title: "JSON Format",
		Type:  "object",
		FormatMeta: FormatMeta{
			ID:         "json",
			Extensions: []string{".json"},
			MimeTypes:  []string{"application/json"},
		},
		Properties: map[string]PropertySchema{
			"extractAllPairs": {Type: "boolean", Default: true, Description: "Extract all pairs"},
			"useKeyAsName":    {Type: "boolean", Default: true, Description: "Use key as name"},
		},
	})

	assert.Equal(t, 1, reg.Count())
	assert.True(t, reg.HasSchema("json"))

	s, ok := reg.GetSchema("json")
	require.True(t, ok)
	assert.Equal(t, "JSON Format", s.Title)
	assert.Len(t, s.Properties, 2)

	// Should have generated RawJSON
	assert.NotEmpty(t, s.RawJSON)

	// ValidateParams should work
	err := reg.ValidateParams("json", map[string]any{"extractAllPairs": true})
	assert.NoError(t, err)

	err = reg.ValidateParams("json", map[string]any{"unknown": true})
	assert.Error(t, err)
}
