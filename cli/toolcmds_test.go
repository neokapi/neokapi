package cli

import (
	"slices"
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestApp creates an App with a fully populated ToolRegistry for testing.
func newTestApp() *App {
	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)
	aitools.RegisterAll(toolReg)
	return &App{ToolReg: toolReg}
}

func TestAllCLIToolsHaveCategory(t *testing.T) {
	app := newTestApp()
	entries := app.ToolReg.CLITools()
	require.NotEmpty(t, entries)
	for _, entry := range entries {
		assert.NotEmpty(t, entry.Info.Category, "tool %q has no category", entry.Info.Name)
	}
}

func TestCLIToolCategories(t *testing.T) {
	valid := map[string]bool{
		schema.CategoryTranslation:    true,
		schema.CategoryQuality:        true,
		schema.CategoryAnalysis:       true,
		schema.CategoryTextProcessing: true,
	}
	app := newTestApp()
	for _, entry := range app.ToolReg.CLITools() {
		assert.True(t, valid[entry.Info.Category],
			"tool %q has invalid category %q", entry.Info.Name, entry.Info.Category)
	}
}

func TestNewToolCommandsSetsGroupID(t *testing.T) {
	app := newTestApp()
	cmds := app.NewToolCommands()
	require.NotEmpty(t, cmds)

	for _, cmd := range cmds {
		assert.NotEmpty(t, cmd.GroupID,
			"command %q should have GroupID set", cmd.Use)
	}
}

func TestNewToolCommands_GeneratesExpectedTools(t *testing.T) {
	app := newTestApp()
	cmds := app.NewToolCommands()

	// Verify specific tools are present.
	names := make(map[string]bool)
	for _, cmd := range cmds {
		names[cmd.Name()] = true
	}

	expectedTools := []string{
		"translate", "pseudo-translate", "tm-leverage", "qa",
		"ai-review", "word-count", "search-replace",
		"segmentation", "script",
	}
	for _, name := range expectedTools {
		assert.True(t, names[name], "expected CLI command for %q", name)
	}

	// Internal tools should NOT be present.
	internalTools := []string{
		"create-target", "remove-target", "layer-processor",
		"span-classify", "batch",
	}
	for _, name := range internalTools {
		assert.False(t, names[name], "internal tool %q should not be a CLI command", name)
	}
}

func TestNewToolCommands_AliasesWork(t *testing.T) {
	app := newTestApp()
	cmds := app.NewToolCommands()

	aliasMap := make(map[string][]string)
	for _, cmd := range cmds {
		if len(cmd.Aliases) > 0 {
			aliasMap[cmd.Name()] = cmd.Aliases
		}
	}

	assert.Contains(t, aliasMap["pseudo-translate"], "pseudo")
	assert.Contains(t, aliasMap["word-count"], "wc")
}

func TestNewToolCommands_WritesOutputHasOutputFlag(t *testing.T) {
	app := newTestApp()
	cmds := app.NewToolCommands()

	for _, cmd := range cmds {
		info := app.ToolReg.ToolInfo(registry.ToolID(cmd.Name()))
		if info == nil {
			continue
		}
		f := cmd.Flags().Lookup("output")
		if info.WritesOutput {
			assert.NotNil(t, f, "tool %q with WritesOutput should have --output flag", cmd.Name())
		} else {
			assert.Nil(t, f, "tool %q without WritesOutput should not have --output flag", cmd.Name())
		}
	}
}

func TestNewToolCommands_CredentialFlagForAITools(t *testing.T) {
	app := newTestApp()
	cmds := app.NewToolCommands()

	for _, cmd := range cmds {
		info := app.ToolReg.ToolInfo(registry.ToolID(cmd.Name()))
		if info == nil {
			continue
		}
		needsCredentials := slices.Contains(info.Requires, "credentials")
		f := cmd.Flags().Lookup("credential")
		if needsCredentials {
			assert.NotNil(t, f, "tool %q requiring credentials should have --credential flag", cmd.Name())
		}
	}
}

func TestDefaultParallelBlocks_AITools(t *testing.T) {
	app := newTestApp()
	info := app.ToolReg.ToolInfo("translate")
	require.NotNil(t, info)
	assert.Equal(t, 5, info.DefaultParallelBlocks)
}

func TestDefaultParallelBlocks_NonAITools(t *testing.T) {
	app := newTestApp()
	info := app.ToolReg.ToolInfo("pseudo-translate")
	require.NotNil(t, info)
	assert.Equal(t, 0, info.DefaultParallelBlocks)
}

func TestAddCommandGroupsRegistersGroups(t *testing.T) {
	root := &cobra.Command{Use: "test"}
	app := &App{}
	app.AddCommandGroups(root)

	groupIDs := []string{"processing", "translation", "quality", "analysis", "text-processing", "management"}
	for _, id := range groupIDs {
		cmd := &cobra.Command{Use: "test-" + id, GroupID: id}
		assert.NotPanics(t, func() {
			root.AddCommand(cmd)
		}, "group %q should be registered", id)
	}
}

func TestCollectorFactories_WordCount(t *testing.T) {
	cf, ok := CollectorFactories["word-count"]
	require.True(t, ok, "word-count should have a collector factory")
	collector := cf()
	assert.NotNil(t, collector)
}

// TestCollectorFactories_SegmentCount guards the #721 fix: segment-count must
// have a collector factory, otherwise RunToolOnFiles aggregates nothing and
// prints empty output for every format.
func TestCollectorFactories_SegmentCount(t *testing.T) {
	cf, ok := CollectorFactories["segment-count"]
	require.True(t, ok, "segment-count should have a collector factory")
	collector := cf()
	require.NotNil(t, collector)
	_, isStreaming := collector.(flow.StreamingCollector)
	assert.True(t, isStreaming, "segment-count collector should be a streaming collector")
}
