package cli

import (
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/cli/output"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
)

// Local project content management — add/rm edit the .kapi recipe's content
// collections and exclude list. These are local project configuration only (no
// server involvement), so they live in core kapi alongside `init`, available
// with or without the bowrain plugin. The product boundary: kapi owns local
// files + project configuration; bowrain owns server sync (push/pull/status).

// NewAddCmd returns `kapi add` — add file patterns to the project's content.
func (a *App) NewAddCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:     "add <pattern> [pattern...]",
		Short:   "Add file patterns to the project's content",
		GroupID: "content",
		Long: `Add file patterns to this project's content so kapi knows which files to
process. Patterns support ** for recursive matching. Format is auto-detected
from the extension unless --format is given.

  kapi add "src/**/*.html"
  kapi add "locales/*.json" --format json
  kapi add "src/**/*.html" "content/**/*.md"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			recipePath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			proj, err := coreproj.Load(recipePath)
			if err != nil {
				return fmt.Errorf("load recipe: %w", err)
			}
			root := filepath.Dir(recipePath)

			var result output.AddOutput
			for _, pattern := range args {
				if contentTracks(proj, pattern) {
					result.Added = append(result.Added, output.AddEntry{Pattern: pattern, Skipped: true})
					continue
				}
				fmtName := format
				if fmtName == "" {
					if ext := filepath.Ext(pattern); ext != "" {
						if det, derr := a.FormatReg.DetectByExtension(ext); derr == nil {
							fmtName = string(det)
						}
					}
				}
				matches, _ := coreproj.ExpandGlob(root, pattern)
				entry := coreproj.ContentCollection{Path: pattern}
				if fmtName != "" {
					entry.Format = &coreproj.FormatSpec{Name: fmtName}
				}
				proj.Content = append(proj.Content, entry)
				result.Added = append(result.Added, output.AddEntry{Pattern: pattern, Format: fmtName, Files: len(matches)})
			}
			if err := coreproj.Save(recipePath, proj); err != nil {
				return fmt.Errorf("save recipe: %w", err)
			}
			return output.Print(cmd, result)
		},
	}
	AddProjectFlag(cmd)
	output.AddFlags(cmd)
	cmd.Flags().StringVarP(&format, "format", "f", "", "file format (e.g. html, json); auto-detected if omitted")
	return cmd
}

// NewRmCmd returns `kapi rm` — stop tracking files matching the given patterns.
func (a *App) NewRmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rm <pattern> [pattern...]",
		Short:   "Remove file patterns from the project's content",
		GroupID: "content",
		Long: `Stop tracking files matching the given patterns.

If a pattern matches one added with 'kapi add', the mapping is removed.
Otherwise the pattern is added to the exclude list so those files are skipped.

  kapi rm "src/**/*.html"       # remove the mapping
  kapi rm "src/legacy/*.html"   # exclude matching files`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			recipePath, err := RequireProjectPath(cmd)
			if err != nil {
				return err
			}
			proj, err := coreproj.Load(recipePath)
			if err != nil {
				return fmt.Errorf("load recipe: %w", err)
			}
			root := filepath.Dir(recipePath)

			var result output.RmOutput
			for _, pattern := range args {
				result.Entries = append(result.Entries, rmPattern(proj, root, pattern))
			}
			if err := coreproj.Save(recipePath, proj); err != nil {
				return fmt.Errorf("save recipe: %w", err)
			}
			return output.Print(cmd, result)
		},
	}
	AddProjectFlag(cmd)
	output.AddFlags(cmd)
	return cmd
}

// contentTracks reports whether the recipe already tracks the exact pattern.
func contentTracks(proj *coreproj.KapiProject, pattern string) bool {
	for _, it := range proj.IterateContent() {
		if it.Item.Path == pattern {
			return true
		}
	}
	return false
}

// rmPattern removes a top-level bare content entry matching the pattern, or
// (if none matches) adds the pattern to the exclude list. Items nested inside
// named collections are not touched (they survive as part of the collection).
func rmPattern(proj *coreproj.KapiProject, root, pattern string) output.RmEntry {
	for i, c := range proj.Content {
		if c.IsBareEntry() && c.Path == pattern {
			format := ""
			if c.Format != nil {
				format = c.Format.Name
			}
			proj.Content = append(proj.Content[:i], proj.Content[i+1:]...)
			return output.RmEntry{Pattern: pattern, Action: "removed", Format: format}
		}
	}
	for _, exc := range proj.Defaults.Exclude {
		if exc == pattern {
			return output.RmEntry{Pattern: pattern, Action: "already_excluded"}
		}
	}
	proj.Defaults.Exclude = append(proj.Defaults.Exclude, pattern)
	matches, _ := coreproj.ExpandGlob(root, pattern)
	return output.RmEntry{Pattern: pattern, Action: "excluded", Files: len(matches)}
}
