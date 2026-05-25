package commands

import (
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
)

var addFormat string

var addCmd = &cobra.Command{
	Use:   "add <pattern> [pattern...]",
	Short: "Add files to the project",
	Long: `Add file patterns to this project so bowrain knows which files to process.

Patterns support ** for recursive matching.

Examples:
  kapi add "src/**/*.html"
  kapi add "locales/*.json" --format json
  kapi add "src/**/*.html" "content/**/*.md"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.FindProject("")
		if err != nil {
			return fmt.Errorf("no kapi project found (run 'kapi init' first): %w", err)
		}

		var result output.AddOutput

		for _, pattern := range args {
			// Check if pattern is already tracked. Walk both bare entries and
			// items inside named collections.
			alreadyTracked := false
			for _, it := range proj.Recipe.IterateContent() {
				if it.Item.Path == pattern {
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
			matches, _ := coreproj.ExpandGlob(proj.Root, pattern)

			// Append a bare content entry to the recipe.
			entry := coreproj.ContentCollection{
				Path: pattern,
			}
			if format != "" {
				entry.Format = &coreproj.FormatSpec{Name: format}
			}
			proj.Recipe.Content = append(proj.Recipe.Content, entry)

			result.Added = append(result.Added, output.AddEntry{
				Pattern: pattern,
				Format:  format,
				Files:   len(matches),
			})
		}

		// Save updated recipe.
		if err := proj.Save(); err != nil {
			return fmt.Errorf("save recipe: %w", err)
		}

		return output.Print(cmd, result)
	},
}

func init() {
	addCmd.Flags().StringVarP(&addFormat, "format", "f", "", "file format (e.g. html, json); auto-detected if omitted")
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(addCmd) })
}
