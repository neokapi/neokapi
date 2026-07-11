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
	cmds := NewToolCommands(app)
	require.NotEmpty(t, cmds)

	// The curated tier and the `tool` group render in root help groups; every
	// other tool keeps a PERMANENT hidden top-level shortcut — dispatchable,
	// but absent from help/completion/reference (surface A3, shortcut form).
	for _, cmd := range cmds {
		if TopLevelTools[cmd.Name()] || cmd.Name() == "tool" {
			assert.NotEmpty(t, cmd.GroupID, "visible command %q should have GroupID set", cmd.Use)
			assert.False(t, cmd.Hidden, "curated command %q must stay visible", cmd.Use)
			continue
		}
		assert.True(t, cmd.Hidden, "shortcut %q must stay hidden", cmd.Use)
		assert.Empty(t, cmd.GroupID, "shortcut %q must not claim a help group", cmd.Use)
	}
}

// TestToolTiering pins the surface-A3 contract: every CLI-visible tool is
// reachable under `kapi tool <name>`, and only the curated TopLevelTools tier
// is visible at the top level.
func TestToolTiering(t *testing.T) {
	app := newTestApp()
	cmds := NewToolCommands(app)

	var toolGroup *cobra.Command
	byName := map[string]*cobra.Command{}
	for _, cmd := range cmds {
		byName[cmd.Name()] = cmd
		if cmd.Name() == "tool" {
			toolGroup = cmd
		}
	}
	require.NotNil(t, toolGroup, "the `kapi tool` group command must exist")

	inGroup := map[string]bool{}
	for _, sub := range toolGroup.Commands() {
		inGroup[sub.Name()] = true
		assert.False(t, sub.Hidden, "tool-group entry %q should be visible in `kapi tool --help`", sub.Name())
	}
	for name := range TopLevelTools {
		assert.True(t, inGroup[name], "curated tool %q must also be reachable under `kapi tool`", name)
		if c := byName[name]; assert.NotNil(t, c, "curated tool %q missing at top level", name) {
			assert.False(t, c.Hidden)
		}
	}
	// Spot-check demoted tools: documented under `tool`, plus a hidden
	// top-level shortcut that dispatches identically.
	for _, name := range []string{"term-check", "content-lint", "length-check"} {
		assert.True(t, inGroup[name], "tool %q must be reachable under `kapi tool`", name)
		if c := byName[name]; assert.NotNil(t, c, "tool %q should keep a hidden top-level shortcut", name) {
			assert.True(t, c.Hidden, "shortcut %q must be hidden", name)
		}
	}
}

func TestNewToolCommands_GeneratesExpectedTools(t *testing.T) {
	app := newTestApp()
	cmds := NewToolCommands(app)

	// Verify specific tools are present (the full set lives under `kapi
	// tool`; the curated tier additionally mounts top-level).
	names := make(map[string]bool)
	for _, cmd := range cmds {
		names[cmd.Name()] = true
		if cmd.Name() == "tool" {
			for _, sub := range cmd.Commands() {
				names[sub.Name()] = true
			}
		}
	}

	expectedTools := []string{
		"translate", "pseudo-translate", "recycle", "qa",
		"review", "word-count", "search-replace",
		"segmentation", "script",
	}
	for _, name := range expectedTools {
		assert.True(t, names[name], "expected CLI command for %q", name)
	}

	// Internal tools should NOT be present anywhere.
	internalTools := []string{
		"create-target", "remove-target", "layer-processor",
		"span-classify", "batch",
	}
	for _, name := range internalTools {
		assert.False(t, names[name], "internal tool %q should not be a CLI command", name)
	}
}

// TestLocalizationGroupRouting verifies that l10n-tagged tools collapse under
// the "localization" help group while generic, format-aware tools keep their
// category-named group. The schema Category is untouched (see
// TestCLIToolCategories) — only the cobra GroupID is rerouted.
func TestLocalizationGroupRouting(t *testing.T) {
	app := newTestApp()
	cmds := NewToolCommands(app)
	require.NotEmpty(t, cmds)

	groupByName := make(map[string]string)
	for _, cmd := range cmds {
		groupByName[cmd.Name()] = cmd.GroupID
	}

	// Localization toolchain → "localization", regardless of schema Category.
	// Group routing is only observable on the curated tier now (demoted
	// aliases are hidden and carry no group — see TestToolTiering).
	for _, name := range []string{"recycle", "translate", "pseudo-translate", "qa"} {
		assert.Equal(t, "localization", groupByName[name],
			"l10n tool %q should route to the localization group", name)
	}

	// Generic / deliberately-untagged curated tools keep their category group.
	assert.Equal(t, schema.CategoryAnalysis, groupByName["word-count"],
		"generic tool word-count should keep its category group")
}

// TestRecycleAlias proves the tm-leverage → recycle rename is complete: the
// canonical command is `recycle` and the old spelling is gone (hard cutover —
// no transition aliases).
func TestRecycleAlias(t *testing.T) {
	app := newTestApp()
	cmds := NewToolCommands(app)

	var recycle *cobra.Command
	for _, cmd := range cmds {
		if cmd.Name() == "recycle" {
			recycle = cmd
		}
		assert.NotEqual(t, "tm-leverage", cmd.Name(),
			"tm-leverage must not be a primary command name — it is an alias")
	}
	require.NotNil(t, recycle, "expected a `recycle` command")
	assert.NotContains(t, recycle.Aliases, "tm-leverage",
		"the tm-leverage spelling is retired outright")
}

func TestNewToolCommands_AliasesWork(t *testing.T) {
	app := newTestApp()
	cmds := NewToolCommands(app)

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
	cmds := NewToolCommands(app)

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
	cmds := NewToolCommands(app)

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
	AddCommandGroups(app, root)

	groupIDs := []string{"work", "assets", "localization", "analysis", "advanced"}
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
