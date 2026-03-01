package main

import (
	"fmt"

	"github.com/gokapi/gokapi/brain/cmd/brain/output"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:   "rm <pattern> [pattern...]",
	Short: "Remove files from the project",
	Long: `Stop tracking files that match the given patterns.

If the pattern matches one you added with 'brain add', it is removed entirely.
Otherwise the pattern is added to the exclude list so those files are skipped.

Examples:
  brain rm "src/**/*.html"          # removes the mapping
  brain rm "src/legacy/*.html"      # excludes files matching the pattern`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.FindProject("")
		if err != nil {
			return fmt.Errorf("no .brain/ project found (run 'brain init' first): %w", err)
		}

		var result output.RmOutput

		for _, pattern := range args {
			entry := processRmPattern(proj, pattern)
			result.Entries = append(result.Entries, entry)
		}

		if err := project.SaveConfig(proj.ConfigDir, proj.Config); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		return output.Print(cmd, result)
	},
}

// processRmPattern handles a single rm pattern against the project config.
func processRmPattern(proj *project.Project, pattern string) output.RmEntry {
	// Check for exact mapping match.
	for i, m := range proj.Config.Mappings {
		if m.Local == pattern {
			proj.Config.Mappings = append(proj.Config.Mappings[:i], proj.Config.Mappings[i+1:]...)
			return output.RmEntry{
				Pattern: pattern,
				Action:  "removed",
				Format:  m.Format,
			}
		}
	}

	// Check if already in exclude list.
	for _, exc := range proj.Config.Exclude {
		if exc == pattern {
			return output.RmEntry{
				Pattern: pattern,
				Action:  "already_excluded",
			}
		}
	}

	// Add to exclude list and count affected files.
	proj.Config.Exclude = append(proj.Config.Exclude, pattern)

	files := 0
	matches, _ := project.ExpandGlob(proj.Root, pattern)
	files = len(matches)

	return output.RmEntry{
		Pattern: pattern,
		Action:  "excluded",
		Files:   files,
	}
}

func init() {
	rootCmd.AddCommand(rmCmd)
}
