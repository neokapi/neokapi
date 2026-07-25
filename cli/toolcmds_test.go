package cli

import (
	"slices"
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
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
	for _, name := range []string{"translate", "pseudo-translate", "qa", "recycle", "term-check", "case-transform", "search-replace"} {
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
		"review", "search-replace",
		"segmentation", "script",
		// A built-in tool reaches the CLI only through its config factory
		// (registry.CLITools). whitespace-correct had none, so it was absent
		// from `kapi exec`, from `kapi tools list`, and from the MCP surface —
		// unreachable except as an inner flow step whose config was dropped.
		"whitespace-correct",
		// The same registration, for the rest of the family (#1476). These four
		// rewrite content, so they also declare WritesOutput — an exec run with
		// nowhere to write is the other half of the same silence.
		"create-target", "remove-target", "inline-codes-remove", "external-command",
		"dnt-check", "placeholder-check", "xml-validation",
	}
	for _, name := range expectedTools {
		assert.True(t, names[name], "expected CLI command for %q", name)
	}

	// Internal tools are absent — and absent *because they say so*, not because
	// someone forgot a config factory. The registry is the source of truth for
	// which those are (schema.ToolMeta.Internal), so this reads the declaration
	// rather than restating a list that could drift from it.
	internal := 0
	for _, info := range app.ToolReg.ListWithSchemas() {
		if !info.Internal {
			continue
		}
		internal++
		assert.False(t, names[string(info.Name)],
			"tool %q declares itself internal, so it must not be a CLI command", info.Name)
	}
	assert.Positive(t, internal, "the internal set must not be empty — it is a real distinction")
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

// A tool that rewrites content needs somewhere to put the result. `kapi exec`
// grows -o / --output-dir only for tools that declare WritesOutput, so a
// content-rewriting tool without it corrects the content in memory, exits 0,
// and writes nothing — the same silent success as the defects in #1471.
// The tool-side invariant (a content-rewriting tool must declare WritesOutput at
// all) is asserted generically over the registry in
// core/tools.TestContentRewritingCLIToolsDeclareWritesOutput; this one holds the
// CLI end — that the declaration actually becomes an -o on the exec command.
func TestExecContentWritingToolsExposeAnOutput(t *testing.T) {
	tools := execChildren(t, newTestApp())
	for _, name := range []string{
		"whitespace-correct", "pseudo-translate", "case-transform", "search-replace", "recycle",
		// #1476: registering these made them CLI-visible for the first time, which
		// is when "and it has nowhere to write" becomes reachable. redact,
		// unredact and media-refine were already visible and already had nowhere —
		// found by the generic invariant, not by inspection.
		"create-target", "remove-target", "inline-codes-remove", "external-command",
		"redact", "unredact", "media-refine",
	} {
		cmd := tools[name]
		require.NotNil(t, cmd, "expected `exec %s`", name)
		assert.NotNil(t, cmd.Flags().Lookup("output"),
			"`exec %s` rewrites content, so it must accept -o", name)
		assert.NotNil(t, cmd.Flags().Lookup("output-dir"),
			"`exec %s` rewrites content, so it must accept --output-dir", name)
	}
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
