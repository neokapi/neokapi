package commands

import (
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:   "rm <pattern> [pattern...]",
	Short: "Remove files from the project",
	Long: `Stop tracking files that match the given patterns.

If the pattern matches one you added with 'kapi add', it is removed entirely.
Otherwise the pattern is added to the exclude list so those files are skipped.

Examples:
  kapi rm "src/**/*.html"          # removes the mapping
  kapi rm "src/legacy/*.html"      # excludes files matching the pattern`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.FindProject("")
		if err != nil {
			return fmt.Errorf("no kapi project found (run 'kapi init' first): %w", err)
		}

		var result output.RmOutput

		for _, pattern := range args {
			entry := processRmPattern(proj, pattern)
			result.Entries = append(result.Entries, entry)
		}

		if err := proj.Save(); err != nil {
			return fmt.Errorf("save recipe: %w", err)
		}

		return output.Print(cmd, result)
	},
}

// processRmPattern handles a single rm pattern against the project recipe.
//
// Note: this only removes top-level bare entries. Items nested inside named
// collections are not touched (they survive as part of the collection).
func processRmPattern(proj *project.Project, pattern string) output.RmEntry {
	recipe := proj.Recipe

	// Check for exact bare entry match at the top level.
	for i, c := range recipe.Content {
		if c.IsBareEntry() && c.Path == pattern {
			format := ""
			if c.Format != nil {
				format = c.Format.Name
			}
			recipe.Content = append(recipe.Content[:i], recipe.Content[i+1:]...)
			return output.RmEntry{
				Pattern: pattern,
				Action:  "removed",
				Format:  format,
			}
		}
	}

	// Check if already in exclude list.
	for _, exc := range recipe.Defaults.Exclude {
		if exc == pattern {
			return output.RmEntry{
				Pattern: pattern,
				Action:  "already_excluded",
			}
		}
	}

	// Add to exclude list and count affected files.
	recipe.Defaults.Exclude = append(recipe.Defaults.Exclude, pattern)

	matches, _ := coreproj.ExpandGlob(proj.Root, pattern)

	return output.RmEntry{
		Pattern: pattern,
		Action:  "excluded",
		Files:   len(matches),
	}
}

func init() {
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(rmCmd) })
}
