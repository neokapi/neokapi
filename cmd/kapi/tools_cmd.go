package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List available tools",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Available tools:")
		fmt.Println()
		fmt.Printf("  %-25s %s\n", "TOOL", "DESCRIPTION")
		fmt.Printf("  %-25s %s\n", "----", "-----------")

		builtinTools := []struct {
			name string
			desc string
		}{
			{"ai-translate", "Translate content using AI/LLM"},
			{"ai-qa", "Check translation quality using AI/LLM"},
			{"ai-terminology", "Extract terminology using AI/LLM"},
			{"ai-review", "Review translations using AI/LLM"},
			{"pseudo-translate", "Generate pseudo-translations for testing"},
		}

		for _, t := range builtinTools {
			fmt.Printf("  %-25s %s\n", t.name, t.desc)
		}
		fmt.Printf("\nTotal: %d tool(s)\n", len(builtinTools))
	},
}

func init() {
	rootCmd.AddCommand(toolsCmd)
}
