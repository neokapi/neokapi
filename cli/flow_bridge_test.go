package cli

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubTool is a minimal tool.Tool for testing tool resolution.
type stubTool struct {
	tool.BaseTool
}

func (s *stubTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for part := range in {
		out <- part
	}
	return nil
}

// TestBuildToolByName_BridgeTool verifies that buildToolByName resolves bridge
// tools from the ToolRegistry.
func TestBuildToolByName_BridgeTool(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	toolReg.RegisterWithSchema("search-and-replace", func() tool.Tool {
		s := &stubTool{}
		s.ToolName = "search-and-replace"
		return s
	}, &schema.ComponentSchema{
		ID:    "search-and-replace",
		Title: "Search and Replace",
		ToolMeta: &schema.ToolMeta{
			ID:          "search-and-replace",
			DisplayName: "Search and Replace",
		},
	})

	app := &App{
		ToolReg:    toolReg,
		TargetLang: "fr",
	}

	config := map[string]any{"source_locale": "en", "target_locale": "fr"}
	tools, cleanup, err := app.buildToolByName("search-and-replace", config)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "search-and-replace", tools[0].Name())
	if cleanup != nil {
		cleanup()
	}
}

// TestBuildToolByName_UnknownTool verifies that buildToolByName returns an error
// for tools not in the ToolRegistry.
func TestBuildToolByName_UnknownTool(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	app := &App{
		ToolReg:    toolReg,
		TargetLang: "fr",
	}

	_, _, err := app.buildToolByName("nonexistent-tool", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in registry")
}

// TestBuildToolByName_BuiltinToolStillWorks verifies that built-in tools
// continue to work after the bridge tool fallthrough fix.
func TestBuildToolByName_BuiltinToolStillWorks(t *testing.T) {
	toolReg := registry.NewToolRegistry()

	// Register the built-in tool in the registry (as Init does).
	toolReg.RegisterWithSchema("pseudo-translate", func() tool.Tool {
		s := &stubTool{}
		s.ToolName = "pseudo-translate"
		return s
	}, nil)

	app := &App{
		ToolReg:    toolReg,
		TargetLang: "qps-ploc",
	}

	config := map[string]any{"source_locale": "en", "target_locale": "qps-ploc"}
	tools, cleanup, err := app.buildToolByName("pseudo-translate", config)
	require.NoError(t, err)
	require.NotEmpty(t, tools)
	if cleanup != nil {
		cleanup()
	}
}

// TestBuildToolByName_MetadataOnlyToolFails verifies that tools registered
// with RegisterMetadata (no factory) produce a clear error.
func TestBuildToolByName_MetadataOnlyToolFails(t *testing.T) {
	toolReg := registry.NewToolRegistry()
	toolReg.RegisterMetadata("metadata-only-tool", &schema.ComponentSchema{
		ID:    "metadata-only-tool",
		Title: "Metadata Only",
		ToolMeta: &schema.ToolMeta{
			ID: "metadata-only-tool",
		},
	}, "test-plugin")

	app := &App{
		ToolReg:    toolReg,
		TargetLang: "fr",
	}

	_, _, err := app.buildToolByName("metadata-only-tool", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no factory")
}

// TestBuildToolByName_RegistryWithConfigFactory verifies that bridge tools
// registered with a ConfigFactory get proper config injection.
func TestBuildToolByName_RegistryWithConfigFactory(t *testing.T) {
	toolReg := registry.NewToolRegistry()

	// Register with a factory.
	toolReg.RegisterWithSchema("segmentation", func() tool.Tool {
		s := &stubTool{}
		s.ToolName = "segmentation"
		return s
	}, &schema.ComponentSchema{
		ID:    "segmentation",
		Title: "Segmentation",
		ToolMeta: &schema.ToolMeta{
			ID: "segmentation",
		},
	})

	// Add a ConfigFactory that captures the config.
	var capturedConfig map[string]any
	var capturedLang string
	toolReg.SetConfigFactory("segmentation", func(config map[string]any, targetLang string) (tool.Tool, error) {
		capturedConfig = config
		capturedLang = targetLang
		s := &stubTool{}
		s.ToolName = "segmentation"
		return s, nil
	})

	app := &App{
		ToolReg:    toolReg,
		TargetLang: "de",
	}

	config := map[string]any{
		"source_locale": "en",
		"target_locale": "de",
		"segmentSource": true,
		"segmentTarget": false,
	}
	tools, _, err := app.buildToolByName("segmentation", config)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "segmentation", tools[0].Name())
	assert.Equal(t, "de", capturedLang)
	assert.Equal(t, true, capturedConfig["segmentSource"])
}
