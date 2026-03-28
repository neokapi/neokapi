package cli

import (
	"sort"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/spf13/cobra"
)

// NewToolsCmd creates the "tools" management command (list tools).
// Bare invocation lists tools; "tools list" is an explicit alias.
func (a *App) NewToolsCmd() *cobra.Command {
	toolsCmd := &cobra.Command{
		Use:     "tools",
		Short:   "List available processing tools",
		GroupID: "management",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.listTools(cmd)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available processing tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.listTools(cmd)
		},
	}

	toolsCmd.AddCommand(listCmd)
	return toolsCmd
}

func (a *App) listTools(cmd *cobra.Command) error {
	// Collect tools exposed as top-level CLI commands.
	seen := make(map[string]bool)
	var tools []output.ToolInfo

	for _, def := range BuiltinToolCommands {
		tools = append(tools, output.ToolInfo{
			Name:        def.Use,
			Description: def.Short,
			Category:    def.Category,
			Source:      "builtin",
		})
		seen[def.Use] = true
	}

	// Add any tools from the registry that aren't already listed.
	if a.ToolReg != nil {
		names := a.ToolReg.Names()
		sort.Strings(names)
		for _, name := range names {
			if !seen[name] {
				t, err := a.ToolReg.NewTool(name)
				if err != nil {
					continue
				}
				tools = append(tools, output.ToolInfo{
					Name:        name,
					Description: t.Description(),
					Source:      "builtin",
				})
			}
		}
	}

	out := output.ToolsListOutput{
		Tools: tools,
		Total: len(tools),
	}
	return output.Print(cmd, out)
}
