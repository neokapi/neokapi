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

func TestLoadBridgeStepTools_HasFactory(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	loader.loadBridgeStepTools("testdata", bridgeReg, bridge.BridgeConfig{}, toolReg, "test")

	// Verify that tools registered via loadBridgeStepTools have factories
	// (not just metadata). NewTool should succeed — it calls the factory.
	for _, name := range []string{"search-and-replace", "segmentation", "quality-check"} {
		t.Run(name, func(t *testing.T) {
			tl, err := toolReg.NewTool(registry.ToolID(name))
			require.NoError(t, err, "NewTool(%q) should succeed — factory must be registered", name)
			assert.Equal(t, name, tl.Name())
		})
	}
}

func TestLoadBridgeStepTools_NewToolWithConfig(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	loader.loadBridgeStepTools("testdata", bridgeReg, bridge.BridgeConfig{}, toolReg, "test")

	// NewToolWithConfig should fall back to the zero-arg factory when
	// no ConfigFactory is registered (bridge tools use the factory).
	config := map[string]any{"regEx": true, "source": true}
	tl, err := toolReg.NewToolWithConfig("search-and-replace", config, "fr")
	require.NoError(t, err)
	assert.Equal(t, "search-and-replace", tl.Name())
}

func TestLoadBridgeStepTools_StepMetaParsed(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	loader.loadBridgeStepTools("testdata", bridgeReg, bridge.BridgeConfig{}, toolReg, "test")

	// Verify x-step metadata is parsed into StepMeta on the schema.
	s := toolReg.GetSchema("search-and-replace")
	require.NotNil(t, s)
	require.NotNil(t, s.StepMeta, "search-and-replace should have StepMeta from x-step")
	assert.Equal(t, "net.sf.okapi.steps.searchandreplace.SearchAndReplaceStep", s.StepMeta.Class)
	assert.Equal(t, "filter-events", s.StepMeta.InputType)
	assert.Equal(t, "filter-events", s.StepMeta.OutputType)
	assert.Contains(t, s.StepMeta.ParameterMappings, "SOURCE_LOCALE")
}

func TestLoadBridgeStepTools_StepMetaInToolInfo(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	loader := &PluginLoader{}
	bridgeReg := bridge.NewBridgeRegistry(1, 1, nil)

	loader.loadBridgeStepTools("testdata", bridgeReg, bridge.BridgeConfig{}, toolReg, "test")

	// Verify StepMeta is surfaced in ToolInfo.
	info := toolReg.GetToolInfo("search-and-replace")
	require.NotNil(t, info)
	require.NotNil(t, info.StepMeta, "ToolInfo should have StepMeta")
	assert.Equal(t, "filter-events", info.StepMeta.InputType)
}

func TestMetadataOnly_HasNoFactory(t *testing.T) {
	toolReg := registry.NewToolRegistry()

	// RegisterMetadata simulates what ScanMetadata does for plugin tools.
	toolReg.RegisterMetadata("some-plugin-tool", &schema.ComponentSchema{
		ID:    "some-plugin-tool",
		Title: "Some Plugin Tool",
		ToolMeta: &schema.ToolMeta{
			ID: "some-plugin-tool",
		},
	}, "test-plugin")

	// Has returns true — the tool appears in listings.
	assert.True(t, toolReg.Has("some-plugin-tool"))

	// But NewTool fails — no factory.
	_, err := toolReg.NewTool("some-plugin-tool")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be instantiated locally")

	// NewToolWithConfig also fails — no factory.
	_, err = toolReg.NewToolWithConfig("some-plugin-tool", nil, "fr")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no factory")
}
