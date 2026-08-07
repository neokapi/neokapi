package cli

import (
	"fmt"
	"path/filepath"
	"sort"

	coreproj "github.com/neokapi/neokapi/core/project"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// NewAddCmd returns `kapi add` — add file patterns to the project's content.
func NewAddCmd(a *App) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:     "add <pattern> [pattern...]",
		Short:   "Add file patterns to the project's content",
		GroupID: "work",
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
				if ContentTracks(proj, pattern) {
					result.Added = append(result.Added, output.AddEntry{Pattern: pattern, Skipped: true})
					continue
				}
				fmtName := format
				if fmtName == "" {
					if ext := filepath.Ext(pattern); ext != "" {
						if det, derr := a.FormatReg.Detect(ext, registry.DetectOptions{ExtensionOnly: true}); derr == nil {
							fmtName = string(det)
						}
					}
				}
				// Refuse to write a pattern the recipe could never expand: this
				// is where the malformed pattern that item 2 has to cope with
				// downstream gets authored, and "0 files" reads as "nothing
				// matches yet" rather than "this pattern is broken".
				matches, gerr := coreproj.ExpandGlob(root, pattern)
				if gerr != nil {
					return fmt.Errorf("pattern %q cannot be expanded, so it would track nothing — fix it before adding it: %w", pattern, gerr)
				}
				entry := coreproj.Collection{Path: pattern}
				if fmtName != "" {
					entry.Format = &coreproj.FormatSpec{Name: fmtName}
				}
				proj.Collections = append(proj.Collections, entry)
				result.Added = append(result.Added, output.AddEntry{Pattern: pattern, Format: fmtName, Files: len(matches)})
			}
			if err := coreproj.Save(recipePath, proj); err != nil {
				return fmt.Errorf("save recipe: %w", err)
			}
			return output.Print(cmd, result)
		},
	}
	AddProjectFlag(cmd)
	output.AddFlags(cmd.Flags())
	cmd.Flags().StringVarP(&format, "format", "f", "", "file format (e.g. html, json); auto-detected if omitted")
	return cmd
}

// NewLsCmd returns `kapi ls` — list the files the project's content tracks.
// With --stats it adds per-file block and word counts. Sync state
// (changed-vs-server) is reported by the platform's `status` command, not here.
func NewLsCmd(a *App) *cobra.Command {
	var stats bool
	cmd := &cobra.Command{
		Use:     "ls [path...]",
		Short:   "List the files the project's content tracks",
		GroupID: "work",
		Long: `List the files matched by the project's content collections (honoring the
exclude list). With --stats, also show per-file block and word counts.

In a server-connected project (a recipe with a bowrain: block, with the
bowrain plugin installed) a SYNC column reports each file's pending-push
standing ("2 to push" / "synced"), derived from the sync cache.

  kapi ls
  kapi ls src/
  kapi ls --stats`,
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
			ctx := cmd.Context()

			out := output.LsOutput{HasStats: stats}
			seen := map[string]bool{}
			for _, it := range proj.IterateContent() {
				lang := string(it.Item.ResolvedSourceLanguage(it.Collection, proj.Defaults))
				pattern := coreproj.ResolvePathPattern(it.Item.Path, lang)
				// `ls` does not go through project.ResolveContent, so it carried
				// its own copy of the same swallow: a pattern that cannot be
				// expanded dropped its collection and `ls` printed a short list
				// with exit 0 — the listing a user checks to confirm the recipe
				// tracks what they think it does.
				rels, gerr := coreproj.ExpandGlob(root, pattern, proj.Defaults.Exclude...)
				if gerr != nil {
					where := "content"
					if it.Collection != nil && it.Collection.Name != "" {
						where = fmt.Sprintf("content collection %q", it.Collection.Name)
					}
					return fmt.Errorf("%s: pattern %q cannot be expanded, so its content would resolve to nothing — fix the pattern in the recipe: %w",
						where, it.Item.Path, gerr)
				}
				for _, rp := range rels {
					if seen[rp] || !MatchesPathPrefix(rp, args) {
						continue
					}
					fmtName := ""
					if it.Item.Format != nil {
						fmtName = coreproj.ResolveFormat(it.Item.Format.Name)
					}
					if fmtName == "" {
						if ext := filepath.Ext(rp); ext != "" {
							if det, derr := a.FormatReg.Detect(ext, registry.DetectOptions{ExtensionOnly: true}); derr == nil {
								fmtName = string(det)
							}
						}
					}
					if fmtName == "" {
						continue
					}
					seen[rp] = true
					entry := output.LsEntry{Path: rp, Format: fmtName}
					if stats {
						blocks, words, _ := a.CountFileBlocks(ctx, filepath.Join(root, rp), registry.FormatID(fmtName), proj.Defaults.SourceLanguage)
						entry.Blocks, entry.Words = blocks, words
						out.Blocks += blocks
						out.Words += words
					}
					out.Files = append(out.Files, entry)
				}
			}
			sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
			out.Total = len(out.Files)
			// Connected project: fold in the per-file sync standing via the
			// plugin's server-ls plumbing (a SYNC column, like status's
			// server section) — the built-in owns the verb in every install.
			a.MergeServerLs(cmd, proj, &out, args)
			return output.Print(cmd, out)
		},
	}
	AddProjectFlag(cmd)
	output.AddFlags(cmd.Flags())
	cmd.Flags().BoolVarP(&stats, "stats", "s", false, "show per-file block and word counts")
	return cmd
}

// NewRmCmd returns `kapi rm` — stop tracking files matching the given patterns.
func NewRmCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rm <pattern> [pattern...]",
		Short:   "Remove file patterns from the project's content",
		GroupID: "work",
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
				result.Entries = append(result.Entries, RmPattern(proj, root, pattern))
			}
			if err := coreproj.Save(recipePath, proj); err != nil {
				return fmt.Errorf("save recipe: %w", err)
			}
			return output.Print(cmd, result)
		},
	}
	AddProjectFlag(cmd)
	output.AddFlags(cmd.Flags())
	return cmd
}
