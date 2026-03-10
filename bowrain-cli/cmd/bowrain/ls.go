package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gokapi/gokapi/bowrain-cli/cmd/bowrain/output"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

var (
	lsStats bool
	lsDirty bool
)

var lsCmd = &cobra.Command{
	Use:   "ls [paths...]",
	Short: "List tracked files",
	Long: `List all files tracked by this project.

Without flags, shows file paths and detected formats. Use --stats for block
and word counts, --dirty to show only files with local changes.

Examples:
  bowrain ls
  bowrain ls src/
  bowrain ls --stats
  bowrain ls --dirty`,
	RunE: runLs,
}

func runLs(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return fmt.Errorf("no .bowrain/ project found (run 'bowrain init' first): %w", err)
	}

	if lsStats || lsDirty {
		return runLsWithStats(cmd, proj, args)
	}
	return runLsFast(cmd, proj, args)
}

// runLsFast lists files without parsing — just glob + format detection.
func runLsFast(cmd *cobra.Command, proj *project.Project, filterPaths []string) error {
	var out output.LsOutput

	srcLang := string(proj.Config.SourceLocale())
	for _, ce := range proj.Config.Content {
		pattern := project.ResolvePathPattern(ce.Path, srcLang)
		relPaths, err := project.ExpandGlob(proj.Root, pattern, proj.Config.Exclude...)
		if err != nil {
			continue
		}
		for _, rp := range relPaths {
			if !matchesPathFilter(rp, filterPaths) {
				continue
			}

			formatName := project.ResolveFormat(ce.Format)
			if formatName == "" {
				ext := filepath.Ext(rp)
				if ext != "" {
					formatName, _ = app.FormatReg.Detector().DetectByExtension(ext)
				}
			}
			if formatName == "" {
				continue
			}

			out.Files = append(out.Files, output.LsEntry{
				Path:   rp,
				Format: formatName,
			})
		}
	}

	out.Total = len(out.Files)
	return output.Print(cmd, out)
}

// runLsWithStats lists files with block/word counts and optional dirty detection.
func runLsWithStats(cmd *cobra.Command, proj *project.Project, filterPaths []string) error {
	conn := project.NewLocalConnector(proj, app.FormatReg)

	files, err := conn.ListFiles(cmd.Context(), nil)
	if err != nil {
		return err
	}

	var out output.LsOutput
	out.HasStats = true
	out.HasDirty = lsDirty

	for _, f := range files {
		if !matchesPathFilter(f.Path, filterPaths) {
			continue
		}
		if lsDirty && f.DirtyCount == 0 {
			continue
		}

		out.Files = append(out.Files, output.LsEntry{
			Path:   f.Path,
			Format: f.Format,
			Blocks: f.BlockCount,
			Words:  f.WordCount,
			Dirty:  f.DirtyCount,
		})
		out.Blocks += f.BlockCount
		out.Words += f.WordCount
		out.Changed += f.DirtyCount
	}
	out.Total = len(out.Files)

	return output.Print(cmd, out)
}

// matchesPathFilter returns true if relPath matches any of the given path prefixes,
// or if no filter paths are specified.
func matchesPathFilter(relPath string, filterPaths []string) bool {
	if len(filterPaths) == 0 {
		return true
	}
	for _, prefix := range filterPaths {
		// Normalize: strip trailing slash.
		prefix = strings.TrimRight(prefix, "/")
		if strings.HasPrefix(relPath, prefix) {
			return true
		}
	}
	return false
}

func init() {
	lsCmd.Flags().BoolVarP(&lsStats, "stats", "s", false, "show block and word counts")
	lsCmd.Flags().BoolVarP(&lsDirty, "dirty", "d", false, "show only files with local changes")
	rootCmd.AddCommand(lsCmd)
}
