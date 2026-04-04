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
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	loader.loadBridgeStepTools("testdata", bridgeReg, bridge.BridgeConfig{}, toolReg, "test")

	names := toolReg.Names()
	assert.NotEmpty(t, names, "expected step tools to be registered")
	assert.True(t, toolReg.Has("search-and-replace"))
	assert.True(t, toolReg.Has("segmentation"))
	assert.True(t, toolReg.Has("quality-check"))
}

func TestLoadBridgeStepTools_SchemaMetadata(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	loader.loadBridgeStepTools("testdata", bridgeReg, bridge.BridgeConfig{}, toolReg, "test")

	s := toolReg.GetSchema("search-and-replace")
	require.NotNil(t, s)

	// Schema $id preserves Okapi naming (no okapi: prefix)
	assert.Equal(t, "search-and-replace", s.ID)
	assert.Equal(t, "Search and Replace", s.Title)
	assert.Equal(t, "search-and-replace", s.ToolMeta.ID)
	assert.NotEmpty(t, s.Properties)
	assert.Equal(t, "boolean", s.Properties["regEx"].Type)
	assert.Equal(t, "boolean", s.Properties["target"].Type)
}

func TestLoadBridgeStepTools_MapsOkapiNameToNeokapi(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	loader.loadBridgeStepTools("testdata", bridgeReg, bridge.BridgeConfig{}, toolReg, "test")

	// Schema preserves Okapi naming
	s := toolReg.GetSchema("search-and-replace")
	require.NotNil(t, s)
	assert.Equal(t, "search-and-replace", s.ID)
	assert.Equal(t, "search-and-replace", s.ToolMeta.ID)

	// Tool registered with step ID as name
	tl, err := toolReg.NewTool("search-and-replace")
	require.NoError(t, err)
	assert.Equal(t, "search-and-replace", tl.Name())
}

func TestLoadBridgeStepTools_SkipsNonStepSchemas(t *testing.T) {
	tmpDir := t.TempDir()
	stepsDir := filepath.Join(tmpDir, "schemas", "steps")
	require.NoError(t, os.MkdirAll(stepsDir, 0o755))

	filterSchema := schema.ComponentSchema{
		ID:    "some-filter",
		Title: "Some Filter",
	}
	data, _ := json.Marshal(filterSchema)
	require.NoError(t, os.WriteFile(filepath.Join(stepsDir, "some-filter.schema.json"), data, 0o644))

	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	loader.loadBridgeStepTools(tmpDir, bridgeReg, bridge.BridgeConfig{}, toolReg, "test")
	assert.False(t, toolReg.Has("some-filter"))
}

func TestLoadBridgeStepTools_MissingDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	loader.loadBridgeStepTools(tmpDir, bridgeReg, bridge.BridgeConfig{}, toolReg, "test")
	assert.Empty(t, toolReg.Names())
}

func TestLoadBridgeStepTools_ToolInfoMetadata(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	loader.loadBridgeStepTools("testdata", bridgeReg, bridge.BridgeConfig{}, toolReg, "test")

	infos := toolReg.ListWithSchemas()
	var found *registry.ToolInfo
	for i := range infos {
		if infos[i].Name == "search-and-replace" {
			found = &infos[i]
			break
		}
	}
	require.NotNil(t, found)
	assert.True(t, found.HasSchema)
}
