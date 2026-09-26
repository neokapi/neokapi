package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Layout describes the on-disk shape of a kapi project: a `kapi.yaml` recipe
// plus an adjacent `.kapi/` cache directory, co-located at the same directory.
// Both paths are absolute.
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

// StateDirName is the one kapi-owned directory in a checkout, and it is a
// disposable cache for that checkout only.
//
// The recipe is the project's committed configuration. The project's context
// (its terms, voice profiles, content memory and recorded decisions) lives in
// the user's workspace, one store per project, shared by every checkout. What
// kapi writes under `.kapi/` is derived from the working tree beside it or
// belongs to this machine: the block store, the caches, the redaction vault and
// the personal saved filters. Deleting the directory costs a re-extraction, except for
// the vault (see VaultDirName).
//
// A person may still keep context files here, such as a terms bundle or a voice
// profile they author: `kapi context import` reads them, through ExportLayout.
// A project that commits such files keeps its own `.kapi/.gitignore`, which
// EnsureLayout never overwrites.
const StateDirName = ".kapi"

// WorkDirName is the machine-state subdirectory of StateDir: the local store,
// the caches, and the redaction vault.
const WorkDirName = "work"

// StateGitignore is the ignore rule EnsureLayout writes into a new `.kapi/`. It
// ignores the whole directory, the rule file included, because nothing kapi
// writes there belongs in version control.
const StateGitignore = "*\n"

// StateGitignoreFilename is where StateGitignore is written, inside `.kapi/`.
const StateGitignoreFilename = ".gitignore"

// GitignoreCovers reports whether an ignore file's content keeps name out of
// version control, either by naming it on a line of its own or by ignoring
// everything. It reads the plain forms kapi writes and makes no attempt to
// evaluate arbitrary patterns.
func GitignoreCovers(content, name string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		switch strings.TrimSpace(line) {
		case "*", "/*", name, "/" + name:
			return true
		}
	}
	return false
}

// WorkDir returns the absolute path of the machine-state directory.
func (l Layout) WorkDir() string {
	return filepath.Join(l.StateDir, WorkDirName)
}

// RelStatePath returns the project-relative path of a context file inside
// `.kapi/`, the path an import records a file under. Joined through filepath
// so it matches the paths the loader resolves on the same platform.
func RelStatePath(parts ...string) string {
	return filepath.Join(append([]string{StateDirName}, parts...)...)
}

// ProfilesDirName holds the context files a person authors for one profile:
// one subdirectory per profile, named for its key under `profiles:`, holding
// that profile's own voice profile and terms bundle.
//
// The filesystem mirrors the recipe. A recipe states its default governance
// under `defaults:` and its exceptions under `profiles:`; the default's files
// therefore sit flat in `.kapi/` and each profile's sit in a directory of its
// own.
//
// `kapi context import` reads them into the store: a terms bundle's concepts
// scoped to the profile, and a voice profile stored by its id, bound under
// `profiles.<name>.voice` when that id is not the profile's name. Governance is
// resolved from the store from then on, so the path sits on ExportLayout.
const ProfilesDirName = "profiles"

// MemoryDirName holds a project's exported content-memory bundles.
//
// A directory rather than a file because a project keeps as many bundles as it
// has content surfaces. The terms bundle is a single file for the opposite
// reason: a project has one vocabulary. The path sits on ExportLayout.
const MemoryDirName = "memory"

// UnitStateDirName holds an exported decision record — one JSON Lines shard
// per document. The path sits on ExportLayout.
const UnitStateDirName = "state"

// StoreFileName is this checkout's PROJECTION: one SQLite file holding the
// block cache, the overlays a flow wrote and the extraction stamps, each
// keeping its own migration ledger (see core/projectdb).
//
// Everything in it is derived from the working tree beside it, so it belongs
// beside that tree and a second checkout of the same project keeps one of its
// own. What the project authored — the terms, the voice profiles, the content
// memory and the unit working set with its staged decisions — is in the user's
// workspace, one database per project, shared by every checkout (core/workspace).
//
// It sits at the TOP of the work directory rather than under work/cache/
// because it is rebuilt by a full re-extraction rather than by the next run
// that happens to need a parse, and `rm -rf .kapi/work/cache` must stay free.
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
// platform-specific caches (e.g. sync caches added by extensions). The store
// sits above the cache inside work/, so deleting the cache costs only the next
// parse.
const CacheDirName = "cache"

// LocalFiltersFilename holds this checkout's personal saved content filters
// (the desktop "Active Filter") and which one is active. The filters a team
// shares are a setting of the project, kept in its context store
// (projectdb.SettingSavedFilters).
const LocalFiltersFilename = "filters.local.json"

// CacheDir returns the absolute path to the regenerable-cache subdirectory.
func (l Layout) CacheDir() string {
	return filepath.Join(l.WorkDir(), CacheDirName)
}

// LocalFiltersPath returns the path to the personal saved-filters file.
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

// LayoutAt returns the Layout for a project root without touching the disk —
// the pure path computation behind the two resolvers below.
//
// It exists so that "the store of the project at root" is spelled one way, and
// not hand-rolled as `filepath.Join(root, StateDirName, StoreFileName)` in
// every module — a spelling the layout cannot move without breaking at run
// time in a package that still compiles. Callers that have a recipe path or a
// working directory want LayoutFor or ResolveLayout; this is for the ones that
// already know the root.
func LayoutAt(root string) Layout {
	return Layout{
		Root:       root,
		RecipePath: filepath.Join(root, RecipeFileName),
		StateDir:   filepath.Join(root, StateDirName),
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

// EnsureLayout creates the `.kapi/` directory and the `work/cache/` under it.
// Idempotent; safe to call on an existing project.
//
// It writes no context file and creates no directory for one. A project's
// terms, voice profiles, content memory and decisions live in the workspace.
func EnsureLayout(layout Layout) error {
	fresh := cacheOnly(layout.StateDir)
	for _, dir := range []string{
		layout.StateDir,
		layout.CacheDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("project: create %s: %w", filepath.Base(dir), err)
		}
	}

	// The ignore rule belongs to the layout rather than to one command: every
	// surface that creates the directory arrives here, and without the rule a
	// first `git add -A` stages the store, the caches and the redaction vault.
	//
	// It is written only into a directory that holds nothing but what kapi
	// itself keeps there. A `.kapi/` that already carries other files, such as
	// context files a project commits, has an ignore arrangement of its own,
	// and a rule that ignored everything would silently keep the next of those
	// files out of the commit. A rule already present is never rewritten.
	ignorePath := filepath.Join(layout.StateDir, StateGitignoreFilename)
	if _, err := os.Stat(ignorePath); errors.Is(err, os.ErrNotExist) && fresh {
		if err := os.WriteFile(ignorePath, []byte(StateGitignore), 0o644); err != nil {
			return fmt.Errorf("project: write %s: %w", StateGitignoreFilename, err)
		}
	}
	return nil
}

// cacheOnly reports whether dir is absent or holds only what kapi writes into
// a checkout's cache directory.
func cacheOnly(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	for _, e := range entries {
		switch e.Name() {
		case WorkDirName, LocalFiltersFilename, StateGitignoreFilename:
		default:
			return false
		}
	}
	return true
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
