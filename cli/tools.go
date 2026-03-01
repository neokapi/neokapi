package cli

import (
	"strings"

	"github.com/gokapi/gokapi/cli/output"
	"github.com/spf13/cobra"
)

// NewToolsCmd creates the tools listing command.
func (a *App) NewToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools",
		Short: "List available processing tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			var tools []output.ToolInfo

			for _, def := range BuiltinToolCommands {
				tools = append(tools, output.ToolInfo{
					Name:        def.Use,
					DisplayName: strings.Join(def.Aliases, ", "),
					Description: def.Short,
					Source:      "builtin",
				})
			}

			otherTools := []struct {
				name string
				desc string
			}{
				{"ai-translate", "Translate content using AI/LLM"},
				{"ai-qa", "Check translation quality using AI/LLM"},
				{"ai-terminology", "Extract terminology using AI/LLM"},
				{"ai-review", "Review translations using AI/LLM"},
				{"pseudo-translate", "Generate pseudo-translations for testing"},
			}

			for _, t := range otherTools {
				tools = append(tools, output.ToolInfo{
					Name:        t.name,
					Description: t.desc,
					Source:      "builtin",
				})
			}

			out := output.ToolsListOutput{
				Tools: tools,
				Total: len(tools),
			}
			return output.Print(cmd, out)
		},
	}
}
