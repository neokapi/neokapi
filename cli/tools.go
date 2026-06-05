package cli

import (
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/spf13/cobra"
)

// NewToolsCmd creates the "tools" management command (list tools).
// Bare invocation lists tools; "tools list" is an explicit alias.
func (a *App) NewToolsCmd() *cobra.Command {
	toolsCmd := &cobra.Command{
		Use:     "tools",
		Short:   "List available processing tools",
		GroupID: "management",
		Example: "  kapi tools\n  kapi tools list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.listTools(cmd)
		},
	}

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List available processing tools",
		Example: "  kapi tools list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.listTools(cmd)
		},
	}

	schemaCmd := &cobra.Command{
		Use:     "schema [tool-name]",
		Short:   "Print the JSON Schema for a tool's parameters",
		Example: "  kapi tools schema word-count\n  kapi tools schema pseudo-translate",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.toolSchema(cmd, args[0])
		},
	}

	toolsCmd.AddCommand(listCmd, schemaCmd)
	return toolsCmd
}

func (a *App) listTools(cmd *cobra.Command) error {
	if a.ToolReg == nil {
		return output.Print(cmd, output.ToolsListOutput{})
	}

	t := a.T()
	var tools []output.ToolInfo
	for _, entry := range a.ToolReg.CLITools() {
		source := entry.Info.Source
		if source == registry.SourceBuiltIn {
			source = "builtin"
		}
		name := string(entry.Info.Name)
		scope := "tools." + name
		displayName := t.T(i18n.Scope(scope+".displayName"), entry.Info.DisplayName)
		desc := t.T(i18n.Scope(scope+".description"), entry.Info.Description)
		if desc == "" {
			desc = displayName
		}
		tools = append(tools, output.ToolInfo{
			Name:        name,
			Description: desc,
			Category:    entry.Info.Category,
			Source:      source,
		})
	}

	out := output.ToolsListOutput{
		Tools: tools,
		Total: len(tools),
	}
	return output.Print(cmd, out)
}

func (a *App) toolSchema(_ *cobra.Command, name string) error {
	if a.ToolReg != nil {
		if s := a.ToolReg.Schema(registry.ToolID(name)); s != nil {
			return printSchema(s)
		}
	}
	return fmt.Errorf("no schema found for tool %q", name)
}

func printSchema(s any) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
