package host

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

	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/output"
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

// UntrackedContent lists the readable files under root that no content
// collection tracks — what is on disk, minus what the recipe declares.
//
// It is the question a refresh asks and no other listing can answer: every
// other view of a project starts from the collections and so can only ever show
// content that is already declared, while a surface added after the recipe was
// written is governed by nothing and appears nowhere. Subtracting one from the
// other names it.
//
// A file is reported when kapi has a reader for its extension and it is matched
// by no item pattern, no target template and no exclude. Files kapi cannot read
// are not content. The recipe and the artifacts it binds as context — the voice
// profile, the terms source, the content memory source, and the per-profile
// bindings — are governance rather than governed, so they are skipped even
// though kapi reads YAML and JSON.
//
// prefixes narrows the report to a subtree, exactly as `kapi ls` does.
func (a *App) UntrackedContent(proj *coreproj.KapiProject, recipePath string, prefixes []string) (output.LsOutput, error) {
	root := filepath.Dir(recipePath)
	tracked, err := trackedPaths(proj, root)
	if err != nil {
		return output.LsOutput{}, err
	}
	for _, rel := range contextArtifacts(proj, filepath.Base(recipePath)) {
		tracked[rel] = true
	}

	out := output.LsOutput{Untracked: true}
	err = coreproj.WalkProjectDir(root, func(rel string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if tracked[rel] || !MatchesPathPrefix(rel, prefixes) {
			return nil
		}
		for _, pattern := range proj.Defaults.Exclude {
			if coreproj.MatchGlob(pattern, rel) {
				return nil
			}
		}
		ext := filepath.Ext(rel)
		if ext == "" {
			return nil
		}
		// Extension-only detection, like `kapi ls`: an extension no format
		// claims is not content kapi can offer to govern, and reporting it
		// would propose a collection that reads nothing.
		det, derr := a.FormatReg.Detect(ext, registry.DetectOptions{ExtensionOnly: true})
		if derr != nil {
			return nil
		}
		out.Files = append(out.Files, output.LsEntry{Path: rel, Format: string(det)})
		return nil
	})
	if err != nil {
		return output.LsOutput{}, err
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	out.Total = len(out.Files)
	return out, nil
}

// trackedPaths expands every content item into the set of project-relative
// paths the recipe governs: each source file a pattern matches, plus every
// target file those sources produce, so a translation a locale already carries
// is never reported as content nobody declared.
//
// A pattern that cannot expand is an error rather than a skip. The set is used
// by subtraction, so a dropped pattern would not shorten a list — it would
// report an entire declared collection as untracked and invite a second
// collection over content the recipe already governs.
func trackedPaths(proj *coreproj.KapiProject, root string) (map[string]bool, error) {
	tracked := map[string]bool{}
	// A source is claimed by the first item that matches it, so only that
	// item's targets are tracked: a later item's target for the same source is
	// a file the loop never writes, and a copy of it on disk is untracked.
	claimed := map[string]bool{}
	for _, it := range proj.IterateContent() {
		lang := string(it.Item.ResolvedSourceLanguage(it.Collection, proj.Defaults))
		pattern := coreproj.ResolvePathPattern(it.Item.Path, lang)
		rels, err := coreproj.ExpandGlob(root, pattern, proj.Defaults.Exclude...)
		if err != nil {
			where := "content"
			if it.Collection != nil && it.Collection.Name != "" {
				where = fmt.Sprintf("content collection %q", it.Collection.Name)
			}
			return nil, fmt.Errorf("%s: pattern %q cannot be expanded, so its content would resolve to nothing. Fix the pattern in the recipe: %w",
				where, it.Item.Path, err)
		}
		for _, rel := range rels {
			rel = filepath.ToSlash(rel)
			if claimed[rel] {
				continue
			}
			claimed[rel] = true
			tracked[rel] = true
			if it.Item.Target == "" {
				continue
			}
			for _, target := range it.Item.ResolvedTargetLanguages(it.Collection, proj.Defaults) {
				out := coreproj.ResolveTargetPath(pattern, it.Item.Base, it.Item.Target, rel, string(target))
				tracked[filepath.ToSlash(out)] = true
			}
		}
	}
	return tracked, nil
}

// contextArtifacts lists the project-relative files the recipe binds as
// context: the recipe itself and every governance document it points at.
func contextArtifacts(proj *coreproj.KapiProject, recipeName string) []string {
	out := []string{recipeName}
	voice := func(b *coreproj.VoiceBinding) {
		if b != nil && b.ProfileFile != "" {
			out = append(out, filepath.ToSlash(b.ProfileFile))
		}
	}
	voice(proj.Defaults.Voice)
	if proj.Defaults.TermsSource != "" {
		out = append(out, filepath.ToSlash(proj.Defaults.TermsSource))
	}
	if proj.Defaults.MemorySource != "" {
		out = append(out, filepath.ToSlash(proj.Defaults.MemorySource))
	}
	for _, prof := range proj.Profiles {
		voice(prof.Voice)
		if prof.TermStore != "" {
			out = append(out, filepath.ToSlash(prof.TermStore))
		}
	}
	return out
}

// CollectionForAdd returns the collection new patterns are appended to: nil
// when they are bare entries (no name given), the existing collection of that
// name, or a new one appended to the recipe.
//
// An existing collection keeps whatever channel it already carries. Re-adding
// to a name is how a surface grows, and silently repointing content that is
// already governed would be a governance change disguised as a file addition;
// a channel that differs is reported instead.
func CollectionForAdd(proj *coreproj.KapiProject, name, channel string) (*coreproj.Collection, error) {
	if name == "" {
		return nil, nil
	}
	for i := range proj.Collections {
		coll := &proj.Collections[i]
		if coll.Name != name {
			continue
		}
		if channel != "" && coll.Channel != "" && coll.Channel != channel {
			return nil, fmt.Errorf("collection %q is bound to channel %q. Repoint it in the recipe rather than through an add", name, coll.Channel)
		}
		if coll.Channel == "" {
			coll.Channel = channel
		}
		if coll.IsBareEntry() {
			return nil, fmt.Errorf("collection %q is a bare entry, which holds one pattern and no content list. Give the new pattern its own name", name)
		}
		return coll, nil
	}
	proj.Collections = append(proj.Collections, coreproj.Collection{Name: name, Channel: channel})
	return &proj.Collections[len(proj.Collections)-1], nil
}

// CollectionRelativePath returns the item path for a project-relative pattern
// inside coll, which is the pattern itself unless the collection declares a
// `base:` — every path under a base is relative to it, so a project-relative
// pattern written verbatim would resolve under the base twice.
func CollectionRelativePath(coll *coreproj.Collection, pattern string) (string, error) {
	if coll.Base == "" {
		return pattern, nil
	}
	base := strings.TrimSuffix(filepath.ToSlash(coll.Base), "/")
	if base == "" {
		return pattern, nil
	}
	rel, ok := strings.CutPrefix(pattern, base+"/")
	if !ok {
		return "", fmt.Errorf("collection %q reads paths under %q, and %q is not inside it. Add it to a collection whose base contains it", coll.Name, base, pattern)
	}
	return rel, nil
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
	for i, c := range proj.Collections {
		if c.IsBareEntry() && c.Path == pattern {
			format := ""
			if c.Format != nil {
				format = c.Format.Name
			}
			proj.Collections = append(proj.Collections[:i], proj.Collections[i+1:]...)
			return output.RmEntry{Pattern: pattern, Action: "removed", Format: format}
		}
	}
	if slices.Contains(proj.Defaults.Exclude, pattern) {
		return output.RmEntry{Pattern: pattern, Action: "already_excluded"}
	}
	proj.Defaults.Exclude = append(proj.Defaults.Exclude, pattern)
	// Deliberately not propagated. The count is informational for a pattern the
	// user just asked to EXCLUDE: a pattern that cannot expand excludes nothing
	// either way, so there is no content to lose and nothing downstream reads
	// this number. (Contrast `content add`, which writes the pattern into the
	// recipe and so refuses a malformed one — see cli/content_cmds.go.)
	matches, _ := coreproj.ExpandGlob(root, pattern)
	return output.RmEntry{Pattern: pattern, Action: "excluded", Files: len(matches)}
}
