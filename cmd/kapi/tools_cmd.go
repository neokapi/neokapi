package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List available tools",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Available tools:")
		fmt.Println()
		fmt.Printf("  %-25s %-12s %s\n", "TOOL", "ALIAS", "DESCRIPTION")
		fmt.Printf("  %-25s %-12s %s\n", "----", "-----", "-----------")

		// Tools exposed as top-level commands.
		for _, def := range builtinToolCommands {
			aliases := strings.Join(def.Aliases, ", ")
			fmt.Printf("  %-25s %-12s %s\n", def.Use, aliases, def.Short)
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
			fmt.Printf("  %-25s %-12s %s\n", t.name, "", t.desc)
		}

		total := len(builtinToolCommands) + len(otherTools)
		fmt.Printf("\nTotal: %d tool(s)\n", total)
	},
}

func init() {
	rootCmd.AddCommand(toolsCmd)
}
