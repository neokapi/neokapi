package main

import (
	"strings"

	"github.com/gokapi/gokapi/kapi/cmd/kapi/output"
	"github.com/spf13/cobra"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List available processing tools",
	RunE: func(cmd *cobra.Command, args []string) error {
		var tools []output.ToolInfo

		// Tools exposed as top-level commands.
		for _, def := range builtinToolCommands {
			tools = append(tools, output.ToolInfo{
				Name:        def.Use,
				DisplayName: strings.Join(def.Aliases, ", "),
				Description: def.Short,
				Source:      "builtin",
			})
		}

		// Other tools available only via `flow run`.
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

func init() {
	rootCmd.AddCommand(toolsCmd)
}
