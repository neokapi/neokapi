package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaRegistry_LoadFromDirectory(t *testing.T) {
	// Create a temp directory with test schemas
	dir := t.TempDir()

	// Create a test schema file
	schema := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"$id": "https://gokapi.dev/schemas/filters/okf_json.schema.json",
		"$version": "1.0.0",
		"title": "JSON Filter",
		"description": "Configuration for the Okapi JSON Filter",
		"type": "object",
		"x-filter": {
			"id": "okf_json",
			"class": "net.sf.okapi.filters.json.JSONFilter",
			"extensions": [".json"],
			"mimeTypes": ["application/json"]
		},
		"x-groups": [
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
				"x-widget": "regexBuilder"
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
	assert.Equal(t, "okf_json", s.FilterMeta.ID)
	assert.Equal(t, "net.sf.okapi.filters.json.JSONFilter", s.FilterMeta.Class)
	assert.Contains(t, s.FilterMeta.Extensions, ".json")
	assert.Contains(t, s.FilterMeta.MimeTypes, "application/json")

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

func TestSchemaRegistry_GetSchemaWithPrefix(t *testing.T) {
	dir := t.TempDir()

	schema := `{
		"$version": "1.0.0",
		"title": "HTML Filter",
		"type": "object",
		"x-filter": { "id": "okf_html", "class": "HtmlFilter", "extensions": [], "mimeTypes": [] },
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

	// Should find without prefix
	_, ok = reg.GetSchema("html")
	assert.True(t, ok)
}

func TestSchemaRegistry_ValidateParams(t *testing.T) {
	dir := t.TempDir()

	schema := `{
		"$version": "1.0.0",
		"title": "Test Filter",
		"type": "object",
		"x-filter": { "id": "okf_test", "class": "Test", "extensions": [], "mimeTypes": [] },
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

func TestSchemaRegistry_ListFilters(t *testing.T) {
	dir := t.TempDir()

	// Create two schema files
	for _, id := range []string{"okf_json", "okf_html"} {
		schema := `{
			"$version": "1.0.0",
			"title": "Filter",
			"type": "object",
			"x-filter": { "id": "` + id + `", "class": "Filter", "extensions": [], "mimeTypes": [] },
			"properties": {}
		}`
		err := os.WriteFile(filepath.Join(dir, id+".schema.json"), []byte(schema), 0644)
		require.NoError(t, err)
	}

	reg := NewSchemaRegistry()
	err := reg.LoadFromDirectory(dir)
	require.NoError(t, err)

	filters := reg.ListFilters()
	assert.Len(t, filters, 2)
}
