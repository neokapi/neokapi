package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Layout describes the on-disk shape of a kapi project per Framework AD-008:
// a `{project}.kapi` recipe file plus an adjacent `.kapi/` state
// folder, co-located at the same directory. Both paths are absolute.
type Layout struct {
	// Root is the directory that holds both RecipePath and StateDir.
	Root string
	// RecipePath is the absolute path to `{project}.kapi`.
	RecipePath string
	// StateDir is the absolute path to `.kapi/`. The directory is
	// guaranteed to exist when returned by ResolveLayout; callers
	// that are scaffolding a fresh project should call EnsureLayout
	// instead.
	StateDir string
}

// StateDirName is the hidden directory that holds kapi's working
// state (manifest bookkeeping, TM, termbase, and the cache subdir).
const StateDirName = ".kapi"

// RecipeExt is the file extension users click to open a project.
const RecipeExt = ".kapi"

// CacheDirName is the subdirectory of StateDir that holds all regenerable
// caches: block store, extraction intermediates, overlay layers, and any
// platform-specific caches (e.g. sync caches added by extensions).
// Authoritative
// project data (TM, termbase, manifest) lives at the top level of
// StateDir so users can blow away the cache without losing translation
// work.
const CacheDirName = "cache"

// BlockStoreFilename is the SQLite block store cache file under CacheDir().
const BlockStoreFilename = "blocks.db"

// FiltersFilename / LocalFiltersFilename hold saved content filters (the
// desktop "Active Filter"): the shared set is committed; the local set is
// personal and gitignored.
const (
	FiltersFilename      = "filters.json"
	LocalFiltersFilename = "filters.local.json"
)

// CacheDir returns the absolute path to the regenerable-cache subdirectory.
func (l Layout) CacheDir() string {
	return filepath.Join(l.StateDir, CacheDirName)
}

// FiltersPath returns the path to the shared (committed) saved-filters file.
func (l Layout) FiltersPath() string {
	return filepath.Join(l.StateDir, FiltersFilename)
}

// LocalFiltersPath returns the path to the personal (gitignored) filters file.
func (l Layout) LocalFiltersPath() string {
	return filepath.Join(l.StateDir, LocalFiltersFilename)
}

// BlockStorePath returns the absolute path of the SQLite block store cache.
func (l Layout) BlockStorePath() string {
	return filepath.Join(l.CacheDir(), BlockStoreFilename)
}

// ExtractionsDir returns the absolute path of the extractions cache root.
func (l Layout) ExtractionsDir() string {
	return filepath.Join(l.CacheDir(), ExtractionsDirName)
}

// CollectionsDir returns the absolute path of the overlay-layers cache root.
func (l Layout) CollectionsDir() string {
	return filepath.Join(l.CacheDir(), CollectionsDirName)
}

// RedactionDirName is the cache subdirectory holding per-batch redaction
// vault sidecars. These contain original sensitive values and must never
// be committed — they live under the gitignored cache root.
const RedactionDirName = "redaction"

// RedactionSidecarPath returns the absolute path of the redaction vault
// sidecar for an extraction batch.
func (l Layout) RedactionSidecarPath(batchID string) string {
	return filepath.Join(l.CacheDir(), RedactionDirName, batchID+".json")
}

// ResolveLayout walks up from `start` looking for a kapi project.
// The recognised shape is exactly one `*.kapi` file at a directory
// level plus an adjacent `.kapi/` subdirectory. Multiple `.kapi`
// files at the same level return ErrAmbiguousLayout; the caller must
// resolve by passing an explicit recipe path.
//
// If only the `.kapi/` state folder is found (no sibling recipe),
// returns ErrRecipeMissing. This keeps the contract explicit:
// consuming tools know which half is missing.
func ResolveLayout(start string) (Layout, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return Layout{}, fmt.Errorf("project: resolve start path: %w", err)
	}

	// If `start` is itself a file, walk from its parent directory.
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	dir := abs
	for {
		layout, err := layoutAtDir(dir)
		if err == nil {
			return layout, nil
		}
		if !errors.Is(err, errLayoutNotHere) {
			return Layout{}, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Layout{}, ErrNoProject
		}
		dir = parent
	}
}

// LayoutFor returns the Layout for an explicit recipe file. The
// recipe must already exist; the `.kapi/` folder is auto-created
// adjacent to it if absent.
func LayoutFor(recipePath string) (Layout, error) {
	abs, err := filepath.Abs(recipePath)
	if err != nil {
		return Layout{}, fmt.Errorf("project: abs recipe path: %w", err)
	}
	if filepath.Ext(abs) != RecipeExt {
		return Layout{}, fmt.Errorf("project: recipe must end in %s, got %q", RecipeExt, abs)
	}
	if info, err := os.Stat(abs); err != nil {
		return Layout{}, fmt.Errorf("project: stat recipe: %w", err)
	} else if info.IsDir() {
		return Layout{}, fmt.Errorf("project: %q is a directory, not a recipe file", abs)
	}
	root := filepath.Dir(abs)
	return Layout{
		Root:       root,
		RecipePath: abs,
		StateDir:   filepath.Join(root, StateDirName),
	}, nil
}

// EnsureLayout creates the `.kapi/` state directory and its `cache/`
// subdirectory if they don't exist. Idempotent; safe to call on an
// existing project.
func EnsureLayout(layout Layout) error {
	if err := os.MkdirAll(layout.StateDir, 0o755); err != nil {
		return fmt.Errorf("project: create state dir: %w", err)
	}
	if err := os.MkdirAll(layout.CacheDir(), 0o755); err != nil {
		return fmt.Errorf("project: create cache dir: %w", err)
	}
	return nil
}

// ─── internals ──────────────────────────────────────────────────

var (
	// ErrNoProject is returned when walking the directory tree finds
	// no kapi project.
	ErrNoProject = errors.New("project: no kapi project found")
	// ErrAmbiguousLayout indicates multiple recipe files at the same
	// directory level — the caller must pass an explicit -p <path>.
	ErrAmbiguousLayout = errors.New("project: multiple recipe files in the same directory — pass an explicit recipe path")
	// ErrRecipeMissing indicates a `.kapi/` state dir with no sibling
	// recipe file. Means the project's identity was lost; user must
	// restore the recipe or reinitialize.
	ErrRecipeMissing = errors.New("project: .kapi/ state dir found but no adjacent *.kapi recipe file")

	errLayoutNotHere = errors.New("no layout at this directory")
)

func layoutAtDir(dir string) (Layout, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Layout{}, errLayoutNotHere
		}
		return Layout{}, fmt.Errorf("project: read dir %s: %w", dir, err)
	}

	var recipes []string
	hasState := false
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() && name == StateDirName {
			hasState = true
			continue
		}
		if !e.IsDir() && filepath.Ext(name) == RecipeExt {
			recipes = append(recipes, name)
		}
	}

	switch {
	case len(recipes) == 0 && !hasState:
		return Layout{}, errLayoutNotHere
	case len(recipes) == 0 && hasState:
		return Layout{}, ErrRecipeMissing
	case len(recipes) > 1:
		return Layout{}, ErrAmbiguousLayout
	}

	// Exactly one recipe. State dir is optional (may be scaffolded later).
	return Layout{
		Root:       dir,
		RecipePath: filepath.Join(dir, recipes[0]),
		StateDir:   filepath.Join(dir, StateDirName),
	}, nil
}
