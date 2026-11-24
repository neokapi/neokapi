package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/rootpath"
)

// Layout describes the on-disk shape of a kapi project: a `kapi.yaml` recipe
// plus an adjacent `.kapi/` cache directory, co-located at the same directory.
// Both paths are absolute.
type Layout struct {
	// Root is the directory that holds both RecipePath and StateDir.
	Root string
	// RecipePath is the absolute path to `kapi.yaml`.
	RecipePath string
	// StateDir is the absolute path to `.kapi/`, which holds only source a
	// project commits: voice profiles, a terms bundle, content memory
	// bundles, recorded decisions. It need not exist.
	StateDir string
	// LocalDir is the absolute path of the directory that holds this
	// checkout's own state: the work directory, the sync state, the vault and
	// the saved filters. Every path to that state goes through it
	// (LocalStateDir). Empty means the checkout's directory under the data
	// root (dataroot.CheckoutDir).
	LocalDir string
}

// LocalStateDir returns the directory that holds this checkout's own state:
// LocalDir when it is set; the checkout's directory under the data root, keyed
// by the checkout's location and the identity of its root directory
// (dataroot.CheckoutDir); and StateDir for a layout whose StateDir lies outside
// its root, which no checkout supplies. It never names a directory inside a
// checkout, so state a checkout carries is never read.
func (l Layout) LocalStateDir() string {
	if l.LocalDir != "" {
		return l.LocalDir
	}
	if l.Root == "" || !rootpath.IsBeneath(l.Root, l.StateDir) {
		return l.StateDir
	}
	return dataroot.CheckoutDir(l.Root)
}

// StateDirName is the directory in a checkout that holds context files a
// project commits, such as a terms bundle or a voice profile a person authors:
// `kapi store import` reads them, through ExportLayout. kapi writes nothing of
// its own there.
//
// A checkout's own state lives under the data root (LocalStateDir). The derived
// half is `work/` (WorkDirName), a cache that deleting costs a re-extraction.
// The rest sits beside it and is never rebuilt: the redaction vault
// (VaultDirName), the sync state a venue keeps (SyncDirName) and the personal
// saved filters (LocalFiltersFilename).
const StateDirName = ".kapi"

// WorkDirName is the derived subdirectory of the checkout's state directory:
// the local store and the caches. Everything in it is rebuilt from the working
// tree and the workspace, so deleting it loses nothing; what cannot be rebuilt
// sits beside it.
const WorkDirName = "work"

// WorkDir returns the absolute path of the derived directory.
func (l Layout) WorkDir() string {
	return filepath.Join(l.LocalStateDir(), WorkDirName)
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
// `kapi store import` reads them into the store: a terms bundle's concepts
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

// UnitStateDirName holds an exported decision record, one JSON Lines shard
// per document. The path sits on ExportLayout.
const UnitStateDirName = "state"

// StoreFileName is this checkout's PROJECTION: one SQLite file holding the
// block cache, the overlays a flow wrote and the extraction stamps, each
// keeping its own migration ledger (see core/projectdb).
//
// Everything in it is derived from the working tree beside it, so it belongs
// beside that tree and a second checkout of the same project keeps one of its
// own. What the project authored, the terms, the voice profiles, the content
// memory and the unit working set with its staged decisions, is in the user's
// workspace, one database per project, shared by every checkout (core/workspace).
//
// It sits at the TOP of the work directory rather than under work/cache/
// because it is rebuilt by a full re-extraction rather than by the next run
// that happens to need a parse, and removing `work/cache` must stay free.
const StoreFileName = "store.db"

// StorePath returns the project's single local store.
func (l Layout) StorePath() string {
	return filepath.Join(l.WorkDir(), StoreFileName)
}

// RecipeFileName is the fixed filename of a kapi project recipe. A plain
// YAML file, so every editor and code host (GitHub/GitLab previews and
// diffs) highlights it with zero configuration. Discovery matches this
// exact basename; the human project label lives in the recipe's `name:`
// field, not the filename.
const RecipeFileName = "kapi.yaml"

// CacheDirName is the subdirectory of WorkDir that holds all regenerable
// caches: the parse cache, extraction intermediates and overlay layers. The store
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
	return filepath.Join(l.LocalStateDir(), LocalFiltersFilename)
}

// ExtractionsDir returns the absolute path of the extractions cache root.
func (l Layout) ExtractionsDir() string {
	return filepath.Join(l.CacheDir(), ExtractionsDirName)
}

// CollectionsDir returns the absolute path of the overlay-layers cache root.
func (l Layout) CollectionsDir() string {
	return filepath.Join(l.CacheDir(), CollectionsDirName)
}

// RedactionDirName is the cache subdirectory that held per-batch redaction
// sidecars before they moved into the vault (VaultBatchesDirName). Only the
// predecessor sweep names it.
const RedactionDirName = "redaction"

// VaultDirName is the StateDir subdirectory holding withheld originals.
//
// Outside work/ on purpose. work/ is defined by being disposable: losing it
// costs CPU. The vault is defined by an EXCLUSION: a named destination must
// never read it, and losing it means redacted content can never be restored.
// Filing it under work/ made it look regenerable, which it is not, and put it
// one removing `work/` away from unrecoverable placeholders.
//
// The originals never leave the machine: the directory is written owner-only
// and kept out of version control (EnsureLocalDir), and nothing syncs it.
const VaultDirName = "vault"

// VaultBatchesDirName holds the vault's per-batch sidecars, one per
// extraction batch that withheld values.
const VaultBatchesDirName = "batches"

// VaultDir returns the absolute path of the withheld-originals root.
func (l Layout) VaultDir() string {
	return filepath.Join(l.LocalStateDir(), VaultDirName)
}

// PrepareVault creates the vault directory owner-only and keeps it out of
// version control. Every writer of withheld originals calls it first.
func (l Layout) PrepareVault() error {
	_, err := EnsureLocalDir(l, VaultDirName)
	return err
}

// SyncDirName is the StateDir subdirectory holding what a venue's push and
// pull keep between runs: the refs a checkout consumed (core/ref/refcache)
// and the sync cache with the confirmed block hashes, the decisions and
// writes last sent and an anonymous project's claim token. The claim token
// exists nowhere else, so the directory sits outside work/.
const SyncDirName = "sync"

// SyncDir returns the absolute path of the venue sync state.
func (l Layout) SyncDir() string {
	return filepath.Join(l.LocalStateDir(), SyncDirName)
}

// EnsureLocalDir creates the subdirectory name of the checkout's state
// directory (LocalStateDir), owner-only.
func EnsureLocalDir(layout Layout, name string) (string, error) {
	if err := EnsureLayout(layout); err != nil {
		return "", err
	}
	dir := filepath.Join(layout.LocalStateDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("project: create %s: %w", name, err)
	}
	// A directory an earlier layout created keeps its own mode; tighten it.
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("project: protect %s: %w", name, err)
	}
	return dir, nil
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
	return filepath.Join(l.VaultDir(), VaultBatchesDirName, batchID+".json")
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

// LayoutAt returns the Layout for a project root without touching the disk, // the pure path computation behind the two resolvers below.
//
// It exists so that "the store of the project at root" is spelled one way, and
// not hand-rolled as `filepath.Join(root, StateDirName, StoreFileName)` in
// every module, a spelling the layout cannot move without breaking at run
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
	l := Layout{
		Root:       root,
		RecipePath: abs,
		StateDir:   filepath.Join(root, StateDirName),
	}
	return l, l.CheckState()
}

// EnsureLayout creates the checkout's state directory under the data root and
// the `work/cache/` in it. It writes nothing in the checkout. Idempotent; safe
// to call on an existing project.
//
// It is the point where a checkout gets its record: a caller calls it when it
// has state to keep for the checkout, such as the project store or the parse
// cache, never to look at a project.
//
// It writes no context file and creates no directory for one. A project's
// terms, voice profiles, content memory and decisions live in the workspace.
func EnsureLayout(layout Layout) error {
	if err := layout.CheckState(); err != nil {
		return err
	}
	local := layout.LocalStateDir()
	if layout.LocalDir == "" && layout.Root != "" && rootpath.IsBeneath(layout.Root, layout.StateDir) {
		// The checkout's state lives under the data root (core/dataroot),
		// created with its record in one step. Nothing is written in the tree.
		if err := os.MkdirAll(layout.Root, 0o755); err != nil {
			return fmt.Errorf("project: create %s: %w", layout.Root, err)
		}
		dir, earlier, err := dataroot.EnsureCheckoutDir(layout.Root)
		if err != nil {
			return fmt.Errorf("project: create the state of %s: %w", layout.Root, err)
		}
		recordCheckoutProject(layout, dir)
		local = dir
		noticeEarlierAtPath(layout, earlier)
		noticeStateInTree(layout)
	}
	if err := os.MkdirAll(filepath.Join(local, WorkDirName, CacheDirName), 0o755); err != nil {
		return fmt.Errorf("project: create %s: %w", WorkDirName, err)
	}
	return nil
}

// Retired state files are not swept here. Every predecessor of the merged store
// (the files the vocabulary rename replaced, as well as the four-file
// `memory.db`/`terms.db`/`cache/blocks.db`/`work/state.db`) is removed by
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
	l := Layout{
		Root:       dir,
		RecipePath: filepath.Join(dir, RecipeFileName),
		StateDir:   filepath.Join(dir, StateDirName),
	}
	if err := checkFoundRecipe(dir, l.RecipePath); err != nil {
		return Layout{}, err
	}
	return l, l.CheckState()
}

// checkFoundRecipe refuses a recipe discovery found that is not a regular file
// inside the directory that holds it. A checkout can commit a kapi.yaml that
// is a symbolic link, and reading it would read a file outside the checkout
// as the project's recipe. A link that stays inside the directory is followed.
// A recipe a person names with -p or KAPI_PROJECT is theirs to point anywhere
// (RecipeIn), so only discovery checks.
func checkFoundRecipe(dir, recipe string) error {
	_, err := rootpath.CheckRegular(dir, recipe)
	if err == nil {
		return nil
	}
	var esc *rootpath.EscapeError
	switch {
	case errors.As(err, &esc) && errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("project: %s is a symbolic link to a file that does not exist. Replace it with the recipe file, or name the recipe with -p", recipe)
	case errors.As(err, &esc):
		return fmt.Errorf("project: %s is a symbolic link that leads outside %s, so kapi does not read it as the project's recipe. "+
			"Name the recipe with -p or %s to use it", recipe, dir, ProjectEnvVar)
	case errors.Is(err, rootpath.ErrNotRegular):
		return fmt.Errorf("project: %s is not a regular file", recipe)
	default:
		return fmt.Errorf("project: check recipe %s: %w", recipe, err)
	}
}
