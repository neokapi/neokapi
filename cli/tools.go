package cli

import "github.com/spf13/cobra"

// NewToolsCmd creates the "tools" management command (list tools).
// Bare invocation lists tools; "tools list" is an explicit alias.
func NewToolsCmd(a *App) *cobra.Command {
	toolsCmd := &cobra.Command{
		Use:     "tools",
		Short:   "List available processing tools",
		GroupID: "advanced",
		Example: "  kapi tools\n  kapi tools list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.ListTools(cmd)
		},
	}

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List available processing tools",
		Example: "  kapi tools list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.ListTools(cmd)
		},
	}

	schemaCmd := &cobra.Command{
		Use:     "schema [tool-name]",
		Short:   "Print the JSON Schema for a tool's parameters",
		Example: "  kapi tools schema translate\n  kapi tools schema pseudo-translate",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.ToolSchema(cmd, args[0])
		},
	}

	toolsCmd.AddCommand(listCmd, schemaCmd)
	return toolsCmd
}
