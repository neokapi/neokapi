package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// ProjectDeclaration answers from the command's own declaration, its -p flag,
// the plugin that serves it, and then its nearest parent's declaration; the
// root's declaration covers the root alone.
func TestProjectDeclaration(t *testing.T) {
	run := func(*cobra.Command, []string) {}
	root := UsesProject(&cobra.Command{Use: "kapi", Run: run})

	withP := &cobra.Command{Use: "status", Run: run}
	AddProjectFlag(withP)

	group := NoProject(&cobra.Command{Use: "plugin"})
	child := &cobra.Command{Use: "list", Run: run}
	childWithP := &cobra.Command{Use: "explain", Run: run}
	AddProjectFlag(childWithP)
	group.AddCommand(child, childWithP)

	persistent := &cobra.Command{Use: "remote"}
	persistent.PersistentFlags().StringP("project", "p", "", "")
	underPersistent := &cobra.Command{Use: "push", Run: run}
	persistent.AddCommand(underPersistent)

	served := &cobra.Command{Use: "pull", Run: run, DisableFlagParsing: true, Annotations: map[string]string{"plugin": "bowrain"}}
	declaredOverP := NoProject(&cobra.Command{Use: "init", Run: run})
	AddProjectFlag(declaredOverP)
	undeclared := &cobra.Command{Use: "stray", Run: run}

	root.AddCommand(withP, group, persistent, served, declaredOverP, undeclared)

	for _, tt := range []struct {
		cmd            *cobra.Command
		uses, declared bool
	}{
		{root, true, true},
		{withP, true, true},
		{group, false, true},
		{child, false, true},
		{childWithP, true, true},
		{underPersistent, true, true},
		{served, false, true},
		{declaredOverP, false, true},
		{undeclared, false, false},
	} {
		uses, declared := ProjectDeclaration(tt.cmd)
		assert.Equal(t, tt.uses, uses, "%s concerns a project", tt.cmd.CommandPath())
		assert.Equal(t, tt.declared, declared, "%s declares", tt.cmd.CommandPath())
	}
}
