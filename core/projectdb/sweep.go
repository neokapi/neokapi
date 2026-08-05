package projectdb

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
)

// sweepPredecessors retires the four-file layout the merged store replaces:
// `memory.db`, `terms.db`, `cache/blocks.db` with its two stamp sidecars, and
// `work/state.db` — plus `tm.db` and `termbase.db`, the spellings the
// vocabulary sweep renamed out of existence one layout earlier.
//
// Only staged decisions are carried forward. Everything else in those files is
// a projection of a committed source — the content memory rebuilds from the
// loop, the terms store compiles from committed sources, the block cache
// re-extracts, the working set re-seeds from `.kapi/units/` — so a migration
// would be work spent reproducing what the next command derives anyway. That is
// the standing preference for resetting over migrating; it holds precisely
// because these files are not sources of truth.
//
// A staged decision is the exception, and the only one: between the decision and
// the next commit, the working store holds its only copy. So those are read out
// and put into the merged store before the old file is deleted.
//
// Best-effort throughout. A predecessor file that will not delete is not worth
// failing a project open over, and the deletions are all idempotent — the sweep
// costs four stats on every subsequent open and finds nothing.
//
// The retired names are spelled here rather than read from core/project's
// layout: they name files no live code path writes any more, so they are this
// package's business alone. Leaving constants for them in the layout would
// invite a new caller to reach for one.
func sweepPredecessors(ctx context.Context, layout project.Layout, into *DB) {
	carryStagedForward(ctx, layout, into)

	blocks := oldBlockStorePath(layout)
	remove := []string{blocks, oldWorkStorePath(layout)}
	for _, name := range oldStateDirDatabases {
		remove = append(remove, filepath.Join(layout.StateDir, name))
	}
	for _, path := range remove {
		removeDatabase(path)
	}
	// The block store's stamps moved into the store_meta table; its sidecars
	// are named after the database file that no longer exists.
	_ = os.Remove(blocks + ".kapiversion")
	_ = os.Remove(blocks + ".sources.json")
	// The browser build's predecessor sidecar, and the partial write its
	// atomic-rename persist can leave behind.
	_ = os.Remove(oldWorkSidecarPath(layout))
	_ = os.Remove(oldWorkSidecarPath(layout) + ".tmp")
	// Removes the work directory only once it is empty, which is the point:
	// anything else that ended up in there is not ours to delete.
	_ = os.Remove(oldWorkDir(layout))
}

// oldStateDirDatabases are the top-level state-directory databases the merged
// store replaces, in both spellings they ever had: `memory.db`/`terms.db` from
// the four-file layout, and the `tm.db`/`termbase.db` the vocabulary sweep
// renamed before that. A project skipping a release meets the older pair, so
// both generations are listed.
var oldStateDirDatabases = []string{"memory.db", "terms.db", "tm.db", "termbase.db"}

const oldWorkDirName = "work"

func oldBlockStorePath(layout project.Layout) string {
	return filepath.Join(layout.CacheDir(), "blocks.db")
}

func oldWorkDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, oldWorkDirName)
}

func oldWorkStorePath(layout project.Layout) string {
	return filepath.Join(oldWorkDir(layout), "state.db")
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
		w, err := state.OpenWork(ctx, oldWorkStorePath(layout), layout.UnitsDir())
		if err != nil {
			return nil, false
		}
		return w, true
	}
	if sidecar := oldWorkSidecarPath(layout); exists(sidecar) {
		w, err := state.OpenWorkSidecar(ctx, sidecar, layout.UnitsDir())
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
