package main

import (
	"fmt"

	"github.com/gokapi/gokapi/kapi/cmd/kapi/output"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var rmCmd = &cobra.Command{
	Use:   "rm <pattern> [pattern...]",
	Short: "Remove or exclude source file patterns",
	Long: `Remove source file patterns from the project mappings, or add them to the
exclude list in .kapi/config.yaml.

If the pattern exactly matches an existing mapping, the mapping is removed.
Otherwise the pattern is added to the exclude list so matching files are
skipped during scanning.

Examples:
  kapi rm "src/**/*.html"          # removes the mapping
  kapi rm "src/legacy/*.html"      # excludes files matching the pattern`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := project.FindProject("")
		if err != nil {
			return fmt.Errorf("no .kapi/ project found (run 'kapi init' first): %w", err)
		}

		var result output.RmOutput

		for _, pattern := range args {
			entry := processRmPattern(proj, pattern)
			result.Entries = append(result.Entries, entry)
		}

		if err := project.SaveConfig(proj.KapiDir, proj.Config); err != nil {
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
