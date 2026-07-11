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

	// Exec-only model: NewToolCommands returns exactly one top-level
	// command — the exec group — and nothing else. Porcelain verbs
	// (translate, pseudo-translate) come from KapiCommandSet, not here.
	require.Len(t, cmds, 1, "NewToolCommands mounts only the exec group at the top level")
	execCmd := cmds[0]
	assert.Equal(t, "exec", execCmd.Name())
	assert.Equal(t, "advanced", execCmd.GroupID)
	assert.False(t, execCmd.Hidden)
}

// TestToolTiering pins the exec-only contract: every CLI-visible tool is
// reachable under `kapi exec <name>` and none mounts at the top level —
// including the former curated tier, whose jobs moved to porcelain verbs.
func TestToolTiering(t *testing.T) {
	app := newTestApp()
	cmds := NewToolCommands(app)

	var toolGroup *cobra.Command
	byName := map[string]*cobra.Command{}
	for _, cmd := range cmds {
		byName[cmd.Name()] = cmd
		if cmd.Name() == "exec" {
			toolGroup = cmd
		}
	}
	require.NotNil(t, toolGroup, "the `kapi exec` group command must exist")

	inGroup := map[string]bool{}
	for _, sub := range toolGroup.Commands() {
		inGroup[sub.Name()] = true
		assert.False(t, sub.Hidden, "exec entry %q should be visible in `kapi exec --help`", sub.Name())
	}
	// Former curated tier + spot-checked demoted tools: exec-only.
	for _, name := range []string{"translate", "pseudo-translate", "qa", "recycle", "word-count", "term-check", "content-lint", "length-check"} {
		assert.True(t, inGroup[name], "tool %q must be reachable under `kapi exec`", name)
		assert.Nil(t, byName[name], "tool %q must not mount at the top level via NewToolCommands", name)
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
		if cmd.Name() == "exec" {
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

// The old localization/analysis help-group routing retired with the curated
// tier: exec children render in one flat list, and the localization group is
// owned by the flow-backed porcelain verbs (see kapicmds.go).

// TestRecycleAlias proves the tm-leverage → recycle rename is complete: the
// canonical command is `recycle` and the old spelling is gone (hard cutover —
// no transition aliases).
// execChildren returns the exec group's children by name.
func execChildren(t *testing.T, app *App) map[string]*cobra.Command {
	t.Helper()
	byName := map[string]*cobra.Command{}
	for _, cmd := range NewToolCommands(app) {
		if cmd.Name() != "exec" {
			continue
		}
		for _, sub := range cmd.Commands() {
			byName[sub.Name()] = sub
		}
	}
	require.NotEmpty(t, byName, "exec group should host the registry tools")
	return byName
}

func TestRecycleAlias(t *testing.T) {
	tools := execChildren(t, newTestApp())
	recycle := tools["recycle"]
	require.NotNil(t, recycle, "expected `exec recycle`")
	assert.NotContains(t, recycle.Aliases, "tm-leverage",
		"the tm-leverage spelling is retired outright")
	assert.NotContains(t, tools, "tm-leverage",
		"tm-leverage must not be a command name")
}

func TestNewToolCommands_AliasesWork(t *testing.T) {
	tools := execChildren(t, newTestApp())
	require.NotNil(t, tools["pseudo-translate"])
	assert.Contains(t, tools["pseudo-translate"].Aliases, "pseudo")
	require.NotNil(t, tools["word-count"])
	assert.Contains(t, tools["word-count"].Aliases, "wc")
}

func TestNewToolCommands_WritesOutputHasOutputFlag(t *testing.T) {
	app := newTestApp()
	var cmds []*cobra.Command
	for _, c := range execChildren(t, app) {
		cmds = append(cmds, c)
	}

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
