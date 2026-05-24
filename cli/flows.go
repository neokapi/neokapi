package cli

import (
	"github.com/spf13/cobra"
)

// NewFlowsCmd creates the "flows" management command (list flows).
// Bare invocation lists flows; "flows list" is an explicit alias.
func (a *App) NewFlowsCmd(opts FlowCmdOptions) *cobra.Command {
	flowsCmd := &cobra.Command{
		Use:     "flows",
		Short:   "List available flows",
		GroupID: "management",
		Example: "  kapi flows",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.listFlows(cmd, opts)
		},
	}

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List available flows",
		Example: "  kapi flows list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.listFlows(cmd, opts)
		},
	}

	flowsCmd.AddCommand(listCmd)
	return flowsCmd
}
