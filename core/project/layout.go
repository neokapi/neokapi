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

// StateDirName is the ONE kapi-owned directory in a project. It is committed
// by default and holds the project's authored context alongside a single
// internal machine-state directory — the git model, where `.git` keeps its
// index inside itself rather than scattering siblings across the tree.
//
// Two lines of `.kapi/.gitignore` describe the whole rule: `work/` and
// `filters.local.json`. Everything else under here is authored and reviewed.
const StateDirName = ".kapi"

// WorkDirName is the single machine-state subdirectory of StateDir: the local
// store, the caches, and the redaction vault. Everything kapi derives or keeps
// per-machine lives under it, and nothing else does — which is what lets the
// ignore rule be one line instead of a growing pattern list with negations
// carved back out of it.
const WorkDirName = "work"

// StateGitignore is the whole ignore rule for a `.kapi/` directory, and it is
// two lines because the directory splits exactly once.
//
// `work/` is machine state: the store, the caches, the vault. `filters.local.json`
// is the one personal file that is not derived, so it cannot live under work/
// and has to be named. Everything else — manifest.yaml, flows/, context/ with
// the terms bundle, the memory seeds, the brand voice and the decision record —
// is authored and committed.
//
// The predecessor listed `cache/` and three `*.db` globs, and still needed a
// negation in the repository's own ignore file to claw the decision record back
// out from under them. A rule that has to be un-said is a rule describing the
// wrong boundary. This one says where the boundary is, and every path method
// below lands on one side of it or the other.
const StateGitignore = WorkDirName + "/\n" + LocalFiltersFilename + "\n"

// ContextDirName is the committed subdirectory of StateDir that holds the
// project's context graph sources: the terms bundle, the content-memory seeds,
// the brand voice profile, and the decision record. These are authored data —
// reviewed in the change that caused the drift, and read on a fresh clone with
// no server and no store.
const ContextDirName = "context"

// WorkDir returns the absolute path of the machine-state directory.
func (l Layout) WorkDir() string {
	return filepath.Join(l.StateDir, WorkDirName)
}

// ContextDir returns the absolute path of the committed context directory.
func (l Layout) ContextDir() string {
	return filepath.Join(l.StateDir, ContextDirName)
}

// RelContextPath returns the project-relative path of a conventional context
// source — what a recipe binding is written as when kapi scaffolds one. Always
// slash-joined through filepath so the binding matches the paths the loader
// resolves on the same platform.
func RelContextPath(name string) string {
	return filepath.Join(StateDirName, ContextDirName, name)
}

// StoreFileName is the project's single local store: one SQLite file holding
// the content memory, the terms store, the block cache and the unit working
// set, each keeping its own migration ledger (see core/projectdb).
//
// One file because the four it replaces could not be written together. An
// approve-and-promote touches the working set and the content memory in the
// same breath, and across two databases there is no transaction that covers
// both — a crash between them left a decision recorded against wording the
// memory never learned.
//
// It sits at the TOP of the work directory, not under work/cache/, because it
// carries staged decisions: deleting it costs at most the decisions staged
// since the last commit, whereas `rm -rf .kapi/work/cache` must stay free.
const StoreFileName = "store.db"

// StoreSidecarFileName is the working set's JSON stand-in on builds with no
// file-backed SQLite driver (the browser build). Same directory, same
// deletion story; only the encoding differs.
const StoreSidecarFileName = "store.json"

// StorePath returns the project's single local store.
func (l Layout) StorePath() string {
	return filepath.Join(l.WorkDir(), StoreFileName)
}

// StoreSidecarPath returns the browser build's working-set sidecar.
func (l Layout) StoreSidecarPath() string {
	return filepath.Join(l.WorkDir(), StoreSidecarFileName)
}

// RecipeFileName is the fixed filename of a kapi project recipe. A plain
// YAML file, so every editor and code host (GitHub/GitLab previews and
// diffs) highlights it with zero configuration. Discovery matches this
// exact basename; the human project label lives in the recipe's `name:`
// field, not the filename.
const RecipeFileName = "kapi.yaml"

// CacheDirName is the subdirectory of WorkDir that holds all regenerable
// caches: the parse cache, extraction intermediates, overlay layers, and any
// platform-specific caches (e.g. sync caches added by extensions). Authoritative
// project data lives in the committed context directory, and the store sits
// above the cache inside work/, so users can blow away the cache without losing
// translation work.
const CacheDirName = "cache"

// FiltersFilename / LocalFiltersFilename hold saved content filters (the
// desktop "Active Filter"): the shared set is committed; the local set is
// personal and gitignored.
const (
	FiltersFilename      = "filters.json"
	LocalFiltersFilename = "filters.local.json"
)

// CacheDir returns the absolute path to the regenerable-cache subdirectory.
func (l Layout) CacheDir() string {
	return filepath.Join(l.WorkDir(), CacheDirName)
}

// FiltersPath returns the path to the shared (committed) saved-filters file.
func (l Layout) FiltersPath() string {
	return filepath.Join(l.StateDir, FiltersFilename)
}

// LocalFiltersPath returns the path to the personal (gitignored) filters file.
func (l Layout) LocalFiltersPath() string {
	return filepath.Join(l.StateDir, LocalFiltersFilename)
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

// VaultDirName is the work subdirectory holding withheld originals.
//
// Separate from cache/ on purpose. The cache is defined by being disposable —
// losing it costs CPU. The vault is defined by an EXCLUSION: a named
// destination must never read it, and losing it means redacted content can
// never be restored. Filing it under cache/ made it look regenerable, which it
// is not, and put it one `rm -rf` away from unrecoverable placeholders.
//
// It is local-only for the same reason it is not cache: the originals never
// leave the machine, so it sits under work/ (never committed, never synced) and
// is the one thing under work/ that is never deleted on kapi's own initiative.
const VaultDirName = "vault"

// VaultDir returns the absolute path of the withheld-originals root.
func (l Layout) VaultDir() string {
	return filepath.Join(l.WorkDir(), VaultDirName)
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

// EnsureLayout creates the `.kapi/` state directory and the two subdirectories
// that give it its shape: the committed `context/` and the ignored
// `work/cache/`. Idempotent; safe to call on an existing project.
func EnsureLayout(layout Layout) error {
	if err := os.MkdirAll(layout.StateDir, 0o755); err != nil {
		return fmt.Errorf("project: create state dir: %w", err)
	}
	if err := os.MkdirAll(layout.ContextDir(), 0o755); err != nil {
		return fmt.Errorf("project: create context dir: %w", err)
	}
	if err := os.MkdirAll(layout.CacheDir(), 0o755); err != nil {
		return fmt.Errorf("project: create cache dir: %w", err)
	}
	return nil
}

// Retired state files are not swept here. Every predecessor of the merged store
// — the vocabulary-renamed `tm.db`/`termbase.db` as well as the four-file
// `memory.db`/`terms.db`/`cache/blocks.db`/`work/state.db` — is removed by
// core/projectdb when the project store opens, so one place decides what a
// state directory may contain.

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

// DecisionsDirName holds the committed decision record — one JSON Lines shard
// per document, under the context directory.
//
// It sits beside the terms bundle and the memory seeds rather than off on its
// own, because it is the same kind of thing: authored context, reviewed in the
// change that caused the drift, read on a fresh clone with no server. The
// record is who approved which wording at which content hash; the working set
// inside the store is only its staging area.
const DecisionsDirName = "decisions"

// DecisionsDir returns the absolute path of the committed decision record.
func (l Layout) DecisionsDir() string {
	return filepath.Join(l.ContextDir(), DecisionsDirName)
}
