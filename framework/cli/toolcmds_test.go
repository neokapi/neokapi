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
