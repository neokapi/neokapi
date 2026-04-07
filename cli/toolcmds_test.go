package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllBuiltinToolCommandsHaveCategory(t *testing.T) {
	for _, def := range BuiltinToolCommands {
		assert.NotEmpty(t, def.Category, "tool %q has no category", def.Use)
	}
}

func TestBuiltinToolCommandCategories(t *testing.T) {
	valid := map[string]bool{
		"translation":     true,
		"quality":         true,
		"analysis":        true,
		"text-processing": true,
	}
	for _, def := range BuiltinToolCommands {
		assert.True(t, valid[def.Category],
			"tool %q has invalid category %q", def.Use, def.Category)
	}
}

func TestNewToolCommandsSetsGroupID(t *testing.T) {
	app := &App{}
	cmds := app.NewToolCommands()
	require.NotEmpty(t, cmds)

	for _, cmd := range cmds {
		assert.NotEmpty(t, cmd.GroupID,
			"command %q should have GroupID set", cmd.Use)
	}
}

func TestLookupToolCommand_ByName(t *testing.T) {
	def := LookupToolCommand("pseudo-translate")
	require.NotNil(t, def)
	assert.Equal(t, "pseudo-translate", def.Use)
	assert.Equal(t, "qps", def.DefaultTargetLang)
}

func TestLookupToolCommand_ByAlias(t *testing.T) {
	def := LookupToolCommand("translate")
	require.NotNil(t, def)
	assert.Equal(t, "ai-translate", def.Use)
}

func TestLookupToolCommand_NotFound(t *testing.T) {
	def := LookupToolCommand("nonexistent-tool")
	assert.Nil(t, def)
}

func TestLookupToolCommand_AllHaveFactory(t *testing.T) {
	for _, def := range BuiltinToolCommands {
		assert.True(t, def.NewToolFromConfig != nil || def.NewTool != nil,
			"tool %q has no factory function", def.Use)
	}
}

func TestDefaultParallelBlocks_AITools(t *testing.T) {
	def := LookupToolCommand("ai-translate")
	require.NotNil(t, def)
	assert.Equal(t, 5, def.DefaultParallelBlocks)
}

func TestDefaultParallelBlocks_NonAITools(t *testing.T) {
	def := LookupToolCommand("pseudo-translate")
	require.NotNil(t, def)
	assert.Equal(t, 0, def.DefaultParallelBlocks)
}

func TestAddCommandGroupsRegistersGroups(t *testing.T) {
	root := &cobra.Command{Use: "test"}
	app := &App{}
	app.AddCommandGroups(root)

	// Verify groups are registered by adding a command with each GroupID.
	groupIDs := []string{"processing", "translation", "quality", "analysis", "text-processing", "management"}
	for _, id := range groupIDs {
		cmd := &cobra.Command{Use: "test-" + id, GroupID: id}
		assert.NotPanics(t, func() {
			root.AddCommand(cmd)
		}, "group %q should be registered", id)
	}
}
