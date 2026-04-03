package cli

import (
	"encoding/json"
	"fmt"
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

	schemaCmd := &cobra.Command{
		Use:   "schema [tool-name]",
		Short: "Print the JSON Schema for a tool's parameters",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.toolSchema(cmd, args[0])
		},
	}

	toolsCmd.AddCommand(listCmd, schemaCmd)
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

func (a *App) toolSchema(_ *cobra.Command, name string) error {
	// Check ToolCommandDefs first
	for _, def := range BuiltinToolCommands {
		if def.Use == name && def.Schema != nil {
			return printSchema(def.Schema)
		}
	}

	// Check registry
	if a.ToolReg != nil {
		if s := a.ToolReg.GetSchema(name); s != nil {
			return printSchema(s)
		}
	}

	return fmt.Errorf("no schema found for tool %q", name)
}

func printSchema(s interface{}) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
