package projectdb

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// Predecessor state directories are folded forward here, by two rules.
//
// RETIRED — deleted, not migrated: the four-file layout's `memory.db`,
// `terms.db`, `cache/blocks.db` with its two stamp sidecars and
// `work/state.db` (plus the earlier `tm.db`/`termbase.db` spellings), and the
// flat layout's `store.db`, its sidecar and `cache/`. Every one of them is a
// projection of a committed source, so the next command derives it again.
//
// MOVED — relocated, never deleted: whatever no source reproduces. The
// committed unit-state record and the vault's withheld originals, and every
// committed source under the `context/` umbrella, whose `decisions/` shards
// land in `state/` under the name the record now has.
//
// A staged unit is the one exception on the retire side: between a review
// landing and the next commit the working store holds its only copy, so those
// are read out into the merged store before the old file is deleted.
//
// Best-effort throughout. A predecessor that will not delete, or a shard that
// will not move, must not fail a project open; a failed move leaves the data
// where it was. Every fold is idempotent: after the first open they cost a
// handful of stats and find nothing.
//
// The retired names are spelled here rather than read from core/project's
// layout, because no live code path writes them — a constant in the layout
// would invite a new caller to reach for one.

// foldLayoutForward relocates what the previous layout kept in places the
// current one no longer reads. Filesystem only, and it runs BEFORE the store
// opens, because one of the two moves carries the committed decision record the
// working store seeds itself from — folded afterwards, the first open of an
// upgraded project would come up with an empty working set.
func foldLayoutForward(layout project.Layout) {
	moveDirContents(flatDecisionsDir(layout), layout.UnitStateDir())
	moveDirContents(flatVaultDir(layout), layout.VaultDir())
	// Before the cache root is deleted, and that ordering is the whole point.
	moveDirContents(flatRedactionDir(layout), currentRedactionDir(layout))
	retireFlatProjections(layout)
	foldContextUmbrella(layout)
	foldGovernanceFiles(layout, layout.StateDir)
}

// umbrellaContextDir is the retired directory that grouped the committed
// sources one level below `.kapi/`.
func umbrellaContextDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "context")
}

// foldContextUmbrella lifts the `context/` generation's committed sources up
// one level, into the directory that now holds them directly.
//
// Everything in there is authored — terms bundle, memory bundles, voice
// profiles, unit record, whatever notes a project kept beside them — so this
// moves and never deletes, entry by entry. The umbrella itself goes only once
// the last entry has left.
//
// The two subdirectories are folded first and by name, because both are
// renamed on the way up: `decisions/` becomes the unit-state record, and
// `memory/` has to land inside the state directory's own `memory/` rather than
// beside it. Everything else is lifted verbatim, minus the three names already
// handled.
func foldContextUmbrella(layout project.Layout) {
	umbrella := umbrellaContextDir(layout)
	if _, err := os.Stat(umbrella); err != nil {
		return
	}
	moveDirContents(filepath.Join(umbrella, "decisions"), layout.UnitStateDir())
	moveDirContents(filepath.Join(umbrella, "memory"), layout.MemoryDir())
	// The single conventional bundle, which becomes the primary inside the
	// bundle directory rather than a file of its own.
	moveFile(filepath.Join(umbrella, "memory.json"), filepath.Join(layout.MemoryDir(), "memory.json"))
	moveDirContents(umbrella, layout.StateDir, "decisions", "memory", "memory.json")
	foldGovernanceFiles(layout, umbrella)
	_ = os.Remove(umbrella)
}

// foldGovernanceFiles moves the voice profiles and per-profile term bundles of
// a flat directory into the profile tree.
//
// `brand-voice.yaml` is the project default and becomes `voice.yaml`, whose
// scope the directory now states. A `<name>-voice.yaml` or `<name>-terms.json`
// is a per-profile override named by prefix, and becomes
// `profiles/<name>/voice.yaml` or `profiles/<name>/terms.json`. Unprefixed
// `terms.json` is the project's own vocabulary and is left where it is.
//
// A recipe that bound one of these files by path still names the old location
// afterwards: authored bindings are not rewritten to chase a move.
// Conventional resolution finds the new locations with no binding at all.
func foldGovernanceFiles(layout project.Layout, from string) {
	entries, err := os.ReadDir(from)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch {
		case name == "brand-voice.yaml":
			moveFile(filepath.Join(from, name), filepath.Join(layout.StateDir, "voice.yaml"))
		case strings.HasSuffix(name, "-voice.yaml"):
			profile := strings.TrimSuffix(name, "-voice.yaml")
			moveFile(filepath.Join(from, name), filepath.Join(layout.ProfileDir(profile), "voice.yaml"))
		case strings.HasSuffix(name, "-terms.json"):
			profile := strings.TrimSuffix(name, "-terms.json")
			moveFile(filepath.Join(from, name), filepath.Join(layout.ProfileDir(profile), "terms.json"))
		}
	}
}

// sweepPredecessors carries staged decisions out of the four-file layout's
// working store and then deletes that layout's databases. Runs after the store
// is open, because the carry needs somewhere to put what it reads.
func sweepPredecessors(ctx context.Context, layout project.Layout, into *DB) {
	carryStagedForward(ctx, layout, into)

	remove := []string{oldBlockStorePath(layout), oldWorkStorePath(layout)}
	for _, name := range oldStateDirDatabases {
		remove = append(remove, filepath.Join(layout.StateDir, name))
	}
	for _, path := range remove {
		removeDatabase(path)
	}
	// The block store's stamps moved into the store_meta table; its sidecars
	// are named after the database file that no longer exists.
	_ = os.Remove(oldBlockStorePath(layout) + ".kapiversion")
	_ = os.Remove(oldBlockStorePath(layout) + ".sources.json")
	// The browser build's predecessor sidecar, and the partial write its
	// atomic-rename persist can leave behind.
	_ = os.Remove(oldWorkSidecarPath(layout))
	_ = os.Remove(oldWorkSidecarPath(layout) + ".tmp")
	// The work directory itself is NOT removed: `.kapi/work/` held only
	// `state.db` in the four-file layout, but is the current layout's home for
	// everything machine-local.
}

// oldStateDirDatabases are the top-level state-directory databases the merged
// store replaces. Both spellings are listed — `memory.db`/`terms.db` and the
// earlier `tm.db`/`termbase.db` — because a project skipping a release meets
// the older pair.
var oldStateDirDatabases = []string{"memory.db", "terms.db", "tm.db", "termbase.db"}

// flatCacheDir, flatDecisionsDir and flatVaultDir name the flat layout's
// directories — the generation that hung everything directly off `.kapi/`.
func flatCacheDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, project.CacheDirName)
}

func flatDecisionsDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "units")
}

func flatVaultDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, project.VaultDirName)
}

// flatRedactionDir is the per-batch redaction sidecar directory under the flat
// cache root, and currentRedactionDir is where it lands.
//
// These move rather than being deleted with the rest of the cache: the sidecars
// hold the ORIGINALS withheld from an extract → merge batch, so a batch still
// in flight cannot be restored without them. "Deleting the cache is free" is a
// promise about work a rerun reproduces, and a pending batch's only copy of its
// own inputs is not that.
func flatRedactionDir(layout project.Layout) string {
	return filepath.Join(flatCacheDir(layout), project.RedactionDirName)
}

func currentRedactionDir(layout project.Layout) string {
	return filepath.Join(layout.CacheDir(), project.RedactionDirName)
}

// retireFlatProjections deletes the flat layout's derived state: the store, its
// browser-build sidecar, and whatever is left of the cache root once the
// redaction sidecars have been moved out of it by the caller. Every one of them
// rebuilds from a committed source. A decision staged in the flat store does
// not, and is not carried across: this runs before any store is open, and
// opening the old one to read it would mean two pools on two schemas that are
// byte-identical apart from their path.
//
// Callers must fold anything unreproducible out of the cache root first; a new
// such directory belongs beside the existing one in foldLayoutForward, not in
// an exception carved out here.
func retireFlatProjections(layout project.Layout) {
	removeDatabase(filepath.Join(layout.StateDir, project.StoreFileName))
	sidecar := filepath.Join(layout.StateDir, project.StoreSidecarFileName)
	_ = os.Remove(sidecar)
	_ = os.Remove(sidecar + ".tmp")

	// Entry by entry rather than RemoveAll, so a redaction sidecar the move
	// left behind is not deleted by the cleanup that follows it.
	entries, err := os.ReadDir(flatCacheDir(layout))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Name() == project.RedactionDirName {
			continue
		}
		_ = os.RemoveAll(filepath.Join(flatCacheDir(layout), e.Name()))
	}
	// Succeeds only once nothing is left, which is exactly when no sidecar was
	// stranded.
	_ = os.Remove(flatCacheDir(layout))
}

// oldBlockStorePath is the four-file layout's block cache, under the flat cache
// root.
func oldBlockStorePath(layout project.Layout) string {
	return filepath.Join(flatCacheDir(layout), "blocks.db")
}

// oldWorkStorePath is the four-file layout's working store. It sits in the
// directory that is now the whole machine-state root, which is why this deletes
// a file and never its parent.
func oldWorkStorePath(layout project.Layout) string {
	return filepath.Join(layout.WorkDir(), "state.db")
}

// removeDatabase deletes a SQLite database and the WAL sidecars that outlive an
// unclean close. Leaving `-wal` behind a deleted database is how a later opener
// finds a journal for a file that is not there.
func removeDatabase(path string) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
}

// oldWorkSidecarPath is where the predecessor working store put its JSON
// sidecar on a build with no SQLite driver: beside the database it stood in
// for, named for it.
func oldWorkSidecarPath(layout project.Layout) string {
	db := oldWorkStorePath(layout)
	return strings.TrimSuffix(db, filepath.Ext(db)) + ".json"
}

// moveDirContents moves every entry of src into dst and removes src once it is
// empty. Used for the directories whose contents no source reproduces.
//
// An entry whose destination already exists is left where it is, and so is one
// whose move fails. Both leave src behind with the data still in it, which is
// resolvable by hand; a shard silently overwritten by another is not.
//
// skip names entries a caller folding a directory in several passes has
// already dealt with elsewhere. They stay in place, and keep src from being
// removed while they are still in it.
func moveDirContents(src, dst string, skip ...string) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}
	if len(entries) == 0 {
		_ = os.Remove(src)
		return
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return
	}
	for _, e := range entries {
		if slices.Contains(skip, e.Name()) {
			continue
		}
		to := filepath.Join(dst, e.Name())
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		_ = os.Rename(filepath.Join(src, e.Name()), to)
	}
	// Succeeds only when every entry moved, which is exactly when src has
	// nothing left to say.
	_ = os.Remove(src)
}

// moveFile moves one file, creating the destination's parent. Same rules as
// moveDirContents: an occupied destination keeps both copies, and a failed move
// leaves the file where it was.
func moveFile(src, dst string) {
	if info, err := os.Lstat(src); err != nil || info.IsDir() {
		return
	}
	if _, err := os.Lstat(dst); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return
	}
	_ = os.Rename(src, dst)
}

// carryStagedForward moves the decisions staged in a predecessor working store
// into the merged one. Silent on failure by design: the alternative is refusing
// to open a project because a file that is about to be deleted would not read.
func carryStagedForward(ctx context.Context, layout project.Layout, into *DB) {
	if into == nil || into.work == nil {
		return
	}
	old, ok := openPredecessorWork(ctx, layout)
	if !ok {
		return
	}
	defer func() { _ = old.Close() }()

	staged, err := old.Staged(ctx)
	if err != nil {
		return
	}
	for _, u := range staged {
		if err := into.work.Put(ctx, u); err != nil {
			return
		}
	}
}

// openPredecessorWork opens whichever predecessor working store this build left
// behind — the database, or the browser build's sidecar — without creating one
// that was not there.
func openPredecessorWork(ctx context.Context, layout project.Layout) (*state.WorkStore, bool) {
	if exists(oldWorkStorePath(layout)) {
		w, err := state.OpenWork(ctx, oldWorkStorePath(layout), layout.UnitStateDir())
		if err != nil {
			return nil, false
		}
		return w, true
	}
	if sidecar := oldWorkSidecarPath(layout); exists(sidecar) {
		w, err := state.OpenWorkSidecar(ctx, sidecar, layout.UnitStateDir())
		if err != nil {
			return nil, false
		}
		return w, true
	}
	return nil, false
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
