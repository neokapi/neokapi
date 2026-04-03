package main

import (
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/bowrain/cli/cmd/bowrain/output"
	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/spf13/cobra"
)

var addFormat string

var addCmd = &cobra.Command{
	Use:   "add <pattern> [pattern...]",
	Short: "Add files to the project",
	Long: `Add file patterns to this project so bowrain knows which files to process.

Patterns support ** for recursive matching.

Examples:
  bowrain add "src/**/*.html"
  bowrain add "locales/*.json" --format json
  bowrain add "src/**/*.html" "content/**/*.md"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.FindProject("")
		if err != nil {
			return fmt.Errorf("no .bowrain/ project found (run 'bowrain init' first): %w", err)
		}

		var result output.AddOutput

		for _, pattern := range args {
			// Check if pattern is already tracked.
			alreadyTracked := false
			for _, m := range proj.Config.Content {
				if m.Path == pattern {
					alreadyTracked = true
					break
				}
			}
			if alreadyTracked {
				result.Added = append(result.Added, output.AddEntry{
					Pattern: pattern,
					Skipped: true,
				})
				continue
			}

			// Detect format.
			format := addFormat
			if format == "" {
				ext := filepath.Ext(pattern)
				if ext != "" {
					detected, err := app.FormatReg.Detector().DetectByExtension(ext)
					if err == nil {
						format = detected
					}
				}
			}

			// Count matching files.
			matches, _ := project.ExpandGlob(proj.Root, pattern)

			// Append mapping to config.
			proj.Config.Content = append(proj.Config.Content, project.ContentEntry{
				Path:   pattern,
				Format: format,
			})

			result.Added = append(result.Added, output.AddEntry{
				Pattern: pattern,
				Format:  format,
				Files:   len(matches),
			})
		}

		// Save updated config.
		if err := project.SaveConfig(proj.ConfigDir, proj.Config); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		return output.Print(cmd, result)
	},
}

func init() {
	addCmd.Flags().StringVarP(&addFormat, "format", "f", "", "file format (e.g. html, json); auto-detected if omitted")
	rootCmd.AddCommand(addCmd)
}
