package host

import (
	"bytes"
	"context"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
)

// Local project content management — add/rm edit the .kapi recipe's content
// collections and exclude list. These are local project configuration only (no
// server involvement), so they live in core kapi alongside `init`, available
// with or without the bowrain plugin. The product boundary: kapi owns local
// files + project configuration; bowrain owns server sync (push/pull/status).

// CountFileBlocks reads a source file through its format reader and returns its
// translatable block count and total source word count (for `kapi ls --stats`).
func (a *App) CountFileBlocks(ctx context.Context, absPath string, fmtID registry.FormatID, srcLocale model.LocaleID) (blocks, words int, err error) {
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

// MatchesPathPrefix reports whether rel matches any of the given path prefixes
// (trailing slash ignored), or true when no prefixes are given.
func MatchesPathPrefix(rel string, prefixes []string) bool {
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

// ContentTracks reports whether the recipe already tracks the exact pattern.
func ContentTracks(proj *coreproj.KapiProject, pattern string) bool {
	for _, it := range proj.IterateContent() {
		if it.Item.Path == pattern {
			return true
		}
	}
	return false
}

// RmPattern removes a top-level bare content entry matching the pattern, or
// (if none matches) adds the pattern to the exclude list. Items nested inside
// named collections are not touched (they survive as part of the collection).
func RmPattern(proj *coreproj.KapiProject, root, pattern string) output.RmEntry {
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
