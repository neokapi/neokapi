package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Layout describes the on-disk shape of a kapi project per Framework AD-008:
// a `kapi.yaml` recipe file plus an adjacent `.kapi/` state folder,
// co-located at the same directory. Both paths are absolute.
type Layout struct {
	// Root is the directory that holds both RecipePath and StateDir.
	Root string
	// RecipePath is the absolute path to `kapi.yaml`.
	RecipePath string
	// StateDir is the absolute path to `.kapi/`. The directory is
	// guaranteed to exist when returned by ResolveLayout; callers
	// that are scaffolding a fresh project should call EnsureLayout
	// instead.
	StateDir string
}

// StateDirName is the hidden directory that holds kapi's working
// state (manifest bookkeeping, content memory, terms, and the cache subdir).
const StateDirName = ".kapi"

// MemoryFileName and TermsFileName are the content-memory and terms stores
// inside StateDir.
//
// The spellings match the concepts: content memory is `memory.db`, the terms
// store is `terms.db`. The old `tm.db` / `termbase.db` were the last of the
// retired vocabulary left in the tree after #1462 (Go identifiers) and #1504
// (source filenames and the sync protobuf); this finishes that programme.
//
// Both files are **regenerable state**, which is why renaming them was cheap:
// the terms store compiles from committed sources and the content memory
// rebuilds from the loop. Neither is a source of truth, so there is nothing to
// migrate — a stale file is simply rebuilt under the new name.
const (
	MemoryFileName = "memory.db"
	TermsFileName  = "terms.db"
)

// RecipeFileName is the fixed filename of a kapi project recipe. A plain
// YAML file, so every editor and code host (GitHub/GitLab previews and
// diffs) highlights it with zero configuration. Discovery matches this
// exact basename; the human project label lives in the recipe's `name:`
// field, not the filename.
const RecipeFileName = "kapi.yaml"

// CacheDirName is the subdirectory of StateDir that holds all regenerable
// caches: block store, extraction intermediates, overlay layers, and any
// platform-specific caches (e.g. sync caches added by extensions).
// Authoritative
// project data (content memory, terms, manifest) lives at the top level of
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

// VaultDirName is the state subdirectory holding withheld originals.
//
// Separate from cache/ on purpose. The cache is defined by being disposable —
// losing it costs CPU. The vault is defined by an EXCLUSION: a named
// destination must never read it, and losing it means redacted content can
// never be restored. Filing it under cache/ made it look regenerable, which it
// is not, and put it one `rm -rf` away from unrecoverable placeholders.
const VaultDirName = "vault"

// VaultDir returns the absolute path of the withheld-originals root.
func (l Layout) VaultDir() string {
	return filepath.Join(l.StateDir, VaultDirName)
}

// RedactionVaultPath returns the project-scoped redaction vault.
//
// Project-scoped rather than per-batch because ingest redaction is continuous:
// a push redacts whatever it reads, whenever it reads it, and a later restore
// has no batch id to look under. The per-batch sidecars below remain for the
// extract → external tool → merge round trip, which genuinely is a batch.
func (l Layout) RedactionVaultPath() string {
	return filepath.Join(l.VaultDir(), "redaction.json")
}

// RedactionSidecarPath returns the absolute path of the redaction vault
// sidecar for an extraction batch.
func (l Layout) RedactionSidecarPath(batchID string) string {
	return filepath.Join(l.CacheDir(), RedactionDirName, batchID+".json")
}

// ResolveLayout walks up from `start` looking for a kapi project.
// The recognised shape is a `kapi.yaml` recipe file at a directory
// level plus an adjacent `.kapi/` subdirectory.
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
			// Every load path passes here, which is what makes the sweep
			// reliable: EnsureLayout alone only covered the writing verbs, and
			// the plugin daemon's read path never called it.
			sweepRetiredStateFiles(layout)
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

// LayoutFor returns the Layout for an explicit recipe path (as passed via
// -p / --project). The path may be either the recipe file itself or a
// project directory containing a `kapi.yaml`; in the directory case the
// recipe inside it is resolved. The recipe must already exist; the `.kapi/`
// folder is auto-created adjacent to it if absent.
//
// Unlike auto-discovery, an explicit path is trusted: the recipe file need
// not be named `kapi.yaml` (a caller pointing at `-p variant.yaml` is taken
// at its word), matching the convention of tools like `docker compose -f`.
func LayoutFor(recipePath string) (Layout, error) {
	abs, err := filepath.Abs(recipePath)
	if err != nil {
		return Layout{}, fmt.Errorf("project: abs recipe path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Layout{}, fmt.Errorf("project: stat recipe: %w", err)
	}
	if info.IsDir() {
		// Allow pointing -p at a project directory; resolve kapi.yaml inside.
		recipe := filepath.Join(abs, RecipeFileName)
		if _, err := os.Stat(recipe); err != nil {
			return Layout{}, fmt.Errorf("project: no %s in %q: %w", RecipeFileName, abs, err)
		}
		abs = recipe
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
	sweepRetiredStateFiles(layout)
	return nil
}

// sweepRetiredStateFiles removes the store files the vocabulary sweep renamed
// out of existence (`tm.db` → `memory.db`, `termbase.db` → `terms.db`). Left
// in place they were a live footgun: retired names sitting beside the live
// stores, indistinguishable from state, and nothing declaring which file was
// authoritative. Both were regenerable projections, so deleting them loses
// nothing — the same reasoning that made the rename itself cheap. Best-effort:
// a file that will not delete is not worth failing a project open over.
func sweepRetiredStateFiles(layout Layout) {
	for _, retired := range []string{"tm.db", "termbase.db"} {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(filepath.Join(layout.StateDir, retired+suffix))
		}
	}
}

// ─── internals ──────────────────────────────────────────────────

var (
	// ErrNoProject is returned when walking the directory tree finds
	// no kapi project.
	ErrNoProject = errors.New("project: no kapi project found")
	// ErrRecipeMissing indicates a `.kapi/` state dir with no sibling
	// recipe file. Means the project's identity was lost; user must
	// restore the recipe or reinitialize.
	ErrRecipeMissing = errors.New("project: .kapi/ state dir found but no adjacent kapi.yaml recipe file")

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

	hasRecipe := false
	hasState := false
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() && name == StateDirName {
			hasState = true
			continue
		}
		if !e.IsDir() && name == RecipeFileName {
			hasRecipe = true
		}
	}

	switch {
	case !hasRecipe && !hasState:
		return Layout{}, errLayoutNotHere
	case !hasRecipe && hasState:
		return Layout{}, ErrRecipeMissing
	}

	// Recipe present. State dir is optional (may be scaffolded later).
	return Layout{
		Root:       dir,
		RecipePath: filepath.Join(dir, RecipeFileName),
		StateDir:   filepath.Join(dir, StateDirName),
	}, nil
}

// WorkDirName is the state subdirectory holding the derived working set.
//
// Gitignored, and disposable ONCE COMMITTED: the working store is an index over
// the committed record, so deleting it costs nothing that has been serialized.
// It is not filed under cache/ because until a decision is committed this holds
// the only copy of it, and cache means regenerable.
const WorkDirName = "work"

// WorkDir returns the absolute path of the derived working-set root.
func (l Layout) WorkDir() string {
	return filepath.Join(l.StateDir, WorkDirName)
}

// WorkStorePath returns the working store database.
func (l Layout) WorkStorePath() string {
	return filepath.Join(l.WorkDir(), "state.db")
}

// UnitsDirName holds the committed record — one JSON Lines shard per document.
//
// Tracked, unlike everything else under the state directory: this is authored
// decision data, so it belongs in the review that caused the drift and must
// survive a fresh clone with no server.
const UnitsDirName = "units"

// UnitsDir returns the absolute path of the committed unit record.
func (l Layout) UnitsDir() string {
	return filepath.Join(l.StateDir, UnitsDirName)
}
