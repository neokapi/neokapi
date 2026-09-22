package host

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms/ktb"
)

// The `.kapi/` context layout: the terms bundle, the content-memory bundles,
// the voice profiles and the decision record.
//
// A project's context lives in the user's workspace and is read from there.
// These files are artifacts on either side of that store: `kapi context
// export` and `kapi context snapshot` write them, `kapi context import` reads
// them, and nothing else opens one. The addresses are here so both commands
// answer with the same list, and so a read path has no way to reach them.

// MetaVoiceBindings is the store-metadata key holding where each voice profile
// in the store is authored — a JSON object of project-relative slash path to
// profile id.
//
// A snapshot writes the store back out as files, and the store keys profiles
// by id while a layout keys them by path. This map is the one place the two
// are tied together.
const MetaVoiceBindings = "context.voiceBindings"

// contextSource is one context file in a `.kapi/` layout.
type contextSource struct {
	// path is absolute; rel is the project-relative slash form, which names
	// the file in a report and keys its voice binding.
	path string
	rel  string
	kind contextSourceKind
}

type contextSourceKind int

const (
	sourceKindTerms contextSourceKind = iota
	sourceKindMemory
	sourceKindVoice
)

// readContextSource reads one file through the importer its kind calls for,
// returning the number of concepts or entries it carried.
func (a *App) readContextSource(ctx context.Context, db *projectdb.DB, root string, src contextSource) (int, error) {
	switch src.kind {
	case sourceKindTerms:
		tb := db.Terms()
		if tb == nil {
			return 0, fmt.Errorf("read terms: %w", projectdb.ErrNoStore)
		}
		f, err := os.Open(src.path)
		if err != nil {
			return 0, fmt.Errorf("open terms source: %w", err)
		}
		defer f.Close()
		n, err := ImportKTBFile(ctx, tb, f)
		if err != nil {
			return 0, fmt.Errorf("read terms %s: %w", src.rel, err)
		}
		return n, nil
	case sourceKindMemory:
		n, err := a.compileMemoryBundle(ctx, db, root, src.path)
		if err != nil {
			return 0, fmt.Errorf("read content memory %s: %w", src.rel, err)
		}
		return n, nil
	case sourceKindVoice:
		if err := a.compileVoiceSource(ctx, db, src); err != nil {
			return 0, fmt.Errorf("read voice profile %s: %w", src.rel, err)
		}
		return 1, nil
	}
	return 0, fmt.Errorf("read context source %s: unknown kind", src.rel)
}

// storeHolds reports whether this build's store carries the subsystem a source
// reads into.
func storeHolds(db *projectdb.DB, kind contextSourceKind) bool {
	switch kind {
	case sourceKindTerms:
		return db.Terms() != nil
	case sourceKindMemory:
		return db.Memory() != nil
	case sourceKindVoice:
		return db.Voice() != nil
	}
	return false
}

// committedContextSources lists the context files a `.kapi/` layout holds, in a
// stable order (terms, then memory bundles by path, then voice profiles by
// path) so two passes over one tree do the same work in the same sequence. A
// bound source that does not exist is left out rather than reported: a recipe
// may bind a file nobody has written.
//
// Each kind is listed where the layout keeps it. The terms bundle and the
// content memory sit at their conventional places under `.kapi/` and wherever
// a recipe's `terms_source:` or `memory_source:` names; a profile keeps its
// own terms and voice under `.kapi/profiles/<name>/`.
//
// proj may be nil, for a layout that belongs to no loaded recipe. Only the
// recipe-bound paths are left out then; the conventional ones still answer.
func committedContextSources(proj *project.KapiProject, layout project.Layout) ([]contextSource, error) {
	export := layout.Export()
	var out []contextSource
	seen := map[string]bool{}

	add := func(path string, kind contextSourceKind) error {
		abs := resolveUnder(layout.Root, path)
		if seen[abs] {
			return nil
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("stat context source %s: %w", path, err)
		}
		if info.IsDir() {
			return nil
		}
		seen[abs] = true
		out = append(out, contextSource{path: abs, rel: relSlash(layout.Root, abs), kind: kind})
		return nil
	}

	if proj != nil {
		if bound := proj.Defaults.TermsSource; bound != "" {
			if err := add(bound, sourceKindTerms); err != nil {
				return nil, err
			}
		}
	}
	if err := add(filepath.Join(layout.StateDir, ktb.ConventionalName), sourceKindTerms); err != nil {
		return nil, err
	}
	for _, name := range profileDirNames(export) {
		if err := add(filepath.Join(export.ProfileDir(name), ktb.ConventionalName), sourceKindTerms); err != nil {
			return nil, err
		}
	}

	memoryPaths, err := bundlePathsIn(export.MemoryDir())
	if err != nil {
		return nil, err
	}
	if proj != nil && proj.Defaults.MemorySource != "" {
		memoryPaths = append(memoryPaths, resolveUnder(layout.Root, proj.Defaults.MemorySource))
	}
	for _, p := range memoryPaths {
		if err := add(p, sourceKindMemory); err != nil {
			return nil, err
		}
	}

	for _, p := range voiceProfilePaths(proj, layout) {
		if err := add(p, sourceKindVoice); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// bundlePathsIn lists the content-memory bundles in dir, sorted. A missing
// directory holds no bundles, which is not an error.
func bundlePathsIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read content-memory bundles: %w", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !kmb.IsBundlePath(e.Name()) {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// voiceProfilePaths lists the voice profiles a `.kapi/` layout holds: the
// project default first, then one per profile directory in name order, then
// anything a recipe binds elsewhere. The order puts the default ahead of the
// overrides, so an id both of them claim is settled the way a layout's own
// order settles it.
func voiceProfilePaths(proj *project.KapiProject, layout project.Layout) []string {
	export := layout.Export()
	out := []string{filepath.Join(layout.StateDir, VoiceConventionalName)}
	for _, n := range profileDirNames(export) {
		out = append(out, filepath.Join(export.ProfileDir(n), VoiceConventionalName))
	}
	if proj == nil {
		return out
	}
	if b := proj.Defaults.Voice; b != nil && b.ProfileFile != "" {
		out = append(out, resolveUnder(layout.Root, b.ProfileFile))
	}
	for _, name := range slices.Sorted(maps.Keys(proj.Profiles)) {
		if b := proj.Profiles[name].Voice; b != nil && b.ProfileFile != "" {
			out = append(out, resolveUnder(layout.Root, b.ProfileFile))
		}
	}
	return out
}

// profileDirNames lists the per-profile override directories a layout holds,
// in name order. A layout with none yields none.
func profileDirNames(export project.ExportLayout) []string {
	entries, err := os.ReadDir(export.ProfilesDir())
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// recordShardsIn lists the decision-record shards in dir, sorted. A missing
// directory holds none, which is not an error.
func recordShardsIn(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != state.CommittedExt {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out
}

// relSlash renders path relative to root in slash form, falling back to the
// base name when the two share no prefix (a bound source outside the project).
func relSlash(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

// projectStoreExists reports whether the project already has a store file. It
// stats rather than opens, because opening creates one — the distinction a dry
// run depends on. path is the recipe or the directory holding it: LayoutFor
// resolves either, so a caller with a project root asks the same question.
func projectStoreExists(path string) bool {
	layout, err := project.LayoutFor(path)
	if err != nil {
		return false
	}
	_, statErr := os.Stat(layout.StorePath())
	return statErr == nil
}
