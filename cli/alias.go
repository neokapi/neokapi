package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// deprecatedAlias converts cmd into a hidden one-release alias: it is removed
// from help (Hidden, no help group) and every runnable command in its tree
// prints the given one-line pointer on stderr before running. The command
// keeps its full flag and subcommand surface, so existing invocations behave
// identically apart from the note. Used for the #1078 C1 verb folds
// (verify → check --ship, ollama → models ollama, presets → init --preset).
func deprecatedAlias(cmd *cobra.Command, note string) *cobra.Command {
	cmd.Hidden = true
	cmd.GroupID = ""
	var wrap func(c *cobra.Command)
	wrap = func(c *cobra.Command) {
		if inner := c.RunE; inner != nil {
			c.RunE = func(cc *cobra.Command, args []string) error {
				fmt.Fprintln(cc.ErrOrStderr(), note)
				return inner(cc, args)
			}
		}
		for _, sub := range c.Commands() {
			wrap(sub)
		}
	}
	wrap(cmd)
	return cmd
}
