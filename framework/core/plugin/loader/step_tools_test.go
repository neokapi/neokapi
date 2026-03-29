package loader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBridgeStepTools_LoadsFromDirectory(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}

	// Use testdata/schemas/tools/ which contains real schemas from okapi-bridge
	testDir := filepath.Join("testdata")

	// Create a dummy bridge registry and config (won't actually connect)
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)
	cfg := bridge.BridgeConfig{}

	loader.loadBridgeStepTools(testDir, bridgeReg, cfg, toolReg, "test")

	// Verify tools were registered
	names := toolReg.Names()
	assert.NotEmpty(t, names, "expected step tools to be registered")

	// Verify specific tools from our test fixtures
	assert.True(t, toolReg.Has("okapi:search-and-replace"), "expected okapi:search-and-replace tool")
	assert.True(t, toolReg.Has("okapi:segmentation"), "expected okapi:segmentation tool")
	assert.True(t, toolReg.Has("okapi:quality-check"), "expected okapi:quality-check tool")
}

func TestLoadBridgeStepTools_SchemaMetadata(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}

	testDir := filepath.Join("testdata")
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)
	cfg := bridge.BridgeConfig{}

	loader.loadBridgeStepTools(testDir, bridgeReg, cfg, toolReg, "test")

	// Check the search-and-replace schema
	s := toolReg.GetSchema("okapi:search-and-replace")
	require.NotNil(t, s, "expected schema for okapi:search-and-replace")

	assert.Equal(t, "okapi:search-and-replace", s.ID)
	assert.Equal(t, "Search and Replace", s.Title)
	assert.Equal(t, "step", s.Meta.Type)
	assert.NotEmpty(t, s.Properties)

	// Verify properties have correct types
	regEx, ok := s.Properties["regEx"]
	assert.True(t, ok, "expected regEx property")
	assert.Equal(t, "boolean", regEx.Type)

	target, ok := s.Properties["target"]
	assert.True(t, ok, "expected target property")
	assert.Equal(t, "boolean", target.Type)
}

func TestLoadBridgeStepTools_CreatesWorkingTool(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}

	testDir := filepath.Join("testdata")
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)
	cfg := bridge.BridgeConfig{}

	loader.loadBridgeStepTools(testDir, bridgeReg, cfg, toolReg, "test")

	// Create a tool instance from the factory
	tl, err := toolReg.NewTool("okapi:search-and-replace")
	require.NoError(t, err)
	require.NotNil(t, tl)

	assert.Equal(t, "okapi:search-and-replace", tl.Name())

	// Verify it implements SchemaProvider
	sp, ok := tl.(interface{ Schema() *schema.ComponentSchema })
	if ok {
		assert.NotNil(t, sp.Schema())
	}
}

func TestLoadBridgeStepTools_SkipsNonStepSchemas(t *testing.T) {
	// Create a temp dir with a non-step schema
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "schemas", "tools")
	require.NoError(t, os.MkdirAll(toolsDir, 0o755))

	// Write a schema with type != "step"
	filterSchema := schema.ComponentSchema{
		ID:    "some-filter",
		Title: "Some Filter",
		Meta:  schema.ComponentMeta{ID: "some-filter", Type: "format"},
	}
	data, _ := json.Marshal(filterSchema)
	require.NoError(t, os.WriteFile(filepath.Join(toolsDir, "some-filter.schema.json"), data, 0o644))

	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	loader.loadBridgeStepTools(tmpDir, bridgeReg, bridge.BridgeConfig{}, toolReg, "test")

	// Should not register non-step schemas
	assert.False(t, toolReg.Has("some-filter"))
}

func TestLoadBridgeStepTools_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't create schemas/tools/ — should return silently

	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	// Should not panic or error
	loader.loadBridgeStepTools(tmpDir, bridgeReg, bridge.BridgeConfig{}, toolReg, "test")
	assert.Empty(t, toolReg.Names())
}

func TestLoadBridgeStepTools_ToolInfoMetadata(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}

	testDir := filepath.Join("testdata")
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)
	cfg := bridge.BridgeConfig{}

	loader.loadBridgeStepTools(testDir, bridgeReg, cfg, toolReg, "test-plugin")

	// Check tool info includes schema status
	infos := toolReg.ListWithSchemas()
	var searchReplace *registry.ToolInfo
	for i := range infos {
		if infos[i].Name == "okapi:search-and-replace" {
			searchReplace = &infos[i]
			break
		}
	}
	require.NotNil(t, searchReplace)
	assert.True(t, searchReplace.HasSchema)
}
