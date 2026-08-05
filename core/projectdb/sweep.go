package projectdb

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// Two generations of state directory are folded forward here, and they are
// folded by two different rules.
//
// The four-file layout the merged store replaced — `memory.db`, `terms.db`,
// `cache/blocks.db` with its two stamp sidecars, and `work/state.db`, plus the
// `tm.db`/`termbase.db` spellings the vocabulary sweep renamed before that — is
// RETIRED. Only staged decisions are carried out of it. Everything else in those
// files is a projection of a committed source: the content memory rebuilds from
// the loop, the terms store compiles from committed sources, the block cache
// re-extracts, the working set re-seeds from the committed decision record. A
// migration would be work spent reproducing what the next command derives
// anyway. That is the standing preference for resetting over migrating, and it
// holds precisely because these files are not sources of truth.
//
// The flat layout that followed — everything hanging directly off `.kapi/` — is
// MOVED, because two of its directories hold things no source reproduces. The
// committed decision record is authored data; the vault holds the only copies of
// withheld originals. Those are relocated, never deleted. The rest of that
// generation (`store.db`, its sidecar, `cache/`) is projection again, and goes
// the way the four-file layout went.
//
// A staged decision is the one exception on the retire side, and the only one:
// between the decision and the next commit, the working store holds its only
// copy. So those are read out and put into the merged store before the old file
// is deleted.
//
// Best-effort throughout. A predecessor that will not delete, or a shard that
// will not move, is not worth failing a project open over — a failed move leaves
// the data where it was, which is the safe direction. Every fold is idempotent:
// after the first open they cost a handful of stats and find nothing.
//
// The retired names are spelled here rather than read from core/project's
// layout: they name paths no live code path writes any more, so they are this
// package's business alone. Leaving constants for them in the layout would
// invite a new caller to reach for one.

// foldLayoutForward relocates what the previous layout kept in places the
// current one no longer reads. Filesystem only, and it runs BEFORE the store
// opens, because one of the two moves carries the committed decision record the
// working store seeds itself from — folded afterwards, the first open of an
// upgraded project would come up with an empty working set.
func foldLayoutForward(layout project.Layout) {
	moveDirContents(flatDecisionsDir(layout), layout.DecisionsDir())
	moveDirContents(flatVaultDir(layout), layout.VaultDir())
	retireFlatProjections(layout)
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
	// The work directory itself is NOT removed. `.kapi/work/` was the four-file
	// layout's home for `state.db` and is the current layout's home for
	// everything machine-local, so the same path means two different things
	// either side of this sweep — and the live one is the meaning that counts.
}

// oldStateDirDatabases are the top-level state-directory databases the merged
// store replaces, in both spellings they ever had: `memory.db`/`terms.db` from
// the four-file layout, and the `tm.db`/`termbase.db` the vocabulary sweep
// renamed before that. A project skipping a release meets the older pair, so
// both generations are listed.
var oldStateDirDatabases = []string{"memory.db", "terms.db", "tm.db", "termbase.db"}

// flatCacheDir, flatDecisionsDir and flatVaultDir name the flat layout's
// directories: the generation that hung everything directly off `.kapi/`,
// before `work/` collected the machine state and `context/` collected the
// committed sources.
func flatCacheDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, project.CacheDirName)
}

func flatDecisionsDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "units")
}

func flatVaultDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, project.VaultDirName)
}

// retireFlatProjections deletes the flat layout's derived state: the store, its
// browser-build sidecar, and the cache root. Every one of them rebuilds from a
// committed source, and the one thing in the flat store that did not — a staged
// decision — is not carried across: the fold runs before any store is open, and
// opening the old one to read it would mean two pools on two schemas that are
// byte-identical apart from their path. The trade is deliberate and bounded at
// the decisions staged between two releases a day apart.
func retireFlatProjections(layout project.Layout) {
	removeDatabase(filepath.Join(layout.StateDir, project.StoreFileName))
	sidecar := filepath.Join(layout.StateDir, project.StoreSidecarFileName)
	_ = os.Remove(sidecar)
	_ = os.Remove(sidecar + ".tmp")
	_ = os.RemoveAll(flatCacheDir(layout))
}

// oldBlockStorePath is the four-file layout's block cache, under the FLAT cache
// root — the only cache root that layout ever had.
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
// empty. Used for the two directories whose contents no source reproduces.
//
// An entry whose destination already exists is left where it is, and so is one
// whose move fails. Both leave src behind, visibly, with the data still in it —
// which is the point. This function's job is to lose nothing; a directory that
// outlives the fold is something a person can look at and resolve, a shard
// silently overwritten by another is not.
func moveDirContents(src, dst string) {
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

// carryStagedForward moves the decisions staged in a predecessor working store
// into the merged one. Silent on failure by design: the alternative is refusing
// to open a project because a file that is about to be deleted would not read,
// and the decisions at stake are staged ones — the same set an interrupted
// command loses today.
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
		w, err := state.OpenWork(ctx, oldWorkStorePath(layout), layout.DecisionsDir())
		if err != nil {
			return nil, false
		}
		return w, true
	}
	if sidecar := oldWorkSidecarPath(layout); exists(sidecar) {
		w, err := state.OpenWorkSidecar(ctx, sidecar, layout.DecisionsDir())
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
