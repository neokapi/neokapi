package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
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

// NewLsCmd returns `kapi ls` — list the files the project's content tracks.
// With --stats it adds per-file block and word counts. Sync state
// (changed-vs-server) is reported by the platform's `status` command, not here.
func (a *App) NewLsCmd() *cobra.Command {
	var stats bool
	cmd := &cobra.Command{
		Use:     "ls [path...]",
		Short:   "List the files the project's content tracks",
		GroupID: "content",
		Long: `List the files matched by the project's content collections (honoring the
exclude list). With --stats, also show per-file block and word counts.

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
				rels, gerr := coreproj.ExpandGlob(root, pattern, proj.Defaults.Exclude...)
				if gerr != nil {
					continue
				}
				for _, rp := range rels {
					if seen[rp] || !matchesPathPrefix(rp, args) {
						continue
					}
					fmtName := ""
					if it.Item.Format != nil {
						fmtName = coreproj.ResolveFormat(it.Item.Format.Name)
					}
					if fmtName == "" {
						if ext := filepath.Ext(rp); ext != "" {
							if det, derr := a.FormatReg.DetectByExtension(ext); derr == nil {
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
						blocks, words, _ := a.countFileBlocks(ctx, filepath.Join(root, rp), registry.FormatID(fmtName), proj.Defaults.SourceLanguage)
						entry.Blocks, entry.Words = blocks, words
						out.Blocks += blocks
						out.Words += words
					}
					out.Files = append(out.Files, entry)
				}
			}
			sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
			out.Total = len(out.Files)
			return output.Print(cmd, out)
		},
	}
	AddProjectFlag(cmd)
	output.AddFlags(cmd)
	cmd.Flags().BoolVarP(&stats, "stats", "s", false, "show per-file block and word counts")
	return cmd
}

// countFileBlocks reads a source file through its format reader and returns its
// translatable block count and total source word count (for `kapi ls --stats`).
func (a *App) countFileBlocks(ctx context.Context, absPath string, fmtID registry.FormatID, srcLocale model.LocaleID) (blocks, words int, err error) {
	reader, err := a.FormatReg.NewReader(fmtID)
	if err != nil {
		return 0, 0, err
	}
	defer reader.Close()
	data, err := os.ReadFile(absPath)
	if err != nil {
		return 0, 0, err
	}
	doc := &model.RawDocument{URI: absPath, SourceLocale: srcLocale, Reader: io.NopCloser(bytes.NewReader(data))}
	if err := reader.Open(ctx, doc); err != nil {
		return 0, 0, err
	}
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return 0, 0, res.Error
		}
		if b, ok := res.Part.Resource.(*model.Block); ok && b != nil && b.Translatable {
			blocks++
			words += b.WordCount()
		}
	}
	return blocks, words, nil
}

// matchesPathPrefix reports whether rel matches any of the given path prefixes
// (trailing slash ignored), or true when no prefixes are given.
func matchesPathPrefix(rel string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, p := range prefixes {
		if strings.HasPrefix(rel, strings.TrimRight(p, "/")) {
			return true
		}
	}
	return false
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
	if slices.Contains(proj.Defaults.Exclude, pattern) {
		return output.RmEntry{Pattern: pattern, Action: "already_excluded"}
	}
	proj.Defaults.Exclude = append(proj.Defaults.Exclude, pattern)
	matches, _ := coreproj.ExpandGlob(root, pattern)
	return output.RmEntry{Pattern: pattern, Action: "excluded", Files: len(matches)}
}
