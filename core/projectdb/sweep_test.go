package projectdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
)

// The predecessor layout, spelled here as the test's own fixture. The names are
// no longer constants anywhere: core/projectdb keeps private copies precisely so
// nothing live can reach for one, and a test that borrowed them would assert
// against the implementation rather than against the disk.
func oldBlockStorePath(layout project.Layout) string {
	return filepath.Join(layout.CacheDir(), "blocks.db")
}

func oldWorkDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "work")
}

func oldWorkStorePath(layout project.Layout) string {
	return filepath.Join(oldWorkDir(layout), "state.db")
}

// predecessorFiles lists everything the four-file layout left in a state
// directory, plus the two spellings the vocabulary sweep retired before it.
func predecessorFiles(layout project.Layout) []string {
	blocks := oldBlockStorePath(layout)
	files := []string{
		blocks,
		blocks + "-wal",
		blocks + ".kapiversion",
		blocks + ".sources.json",
		oldWorkStorePath(layout),
		oldWorkStorePath(layout) + "-wal",
	}
	for _, name := range []string{"memory.db", "terms.db", "tm.db", "termbase.db"} {
		files = append(files,
			filepath.Join(layout.StateDir, name),
			filepath.Join(layout.StateDir, name+"-wal"))
	}
	return files
}

// writePredecessorLayout materialises the four-file layout. The contents do not
// matter: nothing in them is migrated, so the sweep never opens them except for
// the working store — which a caller that cares about staged decisions has
// already written for real, and is left alone here.
func writePredecessorLayout(t *testing.T, layout project.Layout) {
	t.Helper()
	require.NoError(t, os.MkdirAll(layout.CacheDir(), 0o755))
	require.NoError(t, os.MkdirAll(oldWorkDir(layout), 0o755))
	for _, path := range predecessorFiles(layout) {
		if _, err := os.Stat(path); err == nil {
			continue
		}
		require.NoError(t, os.WriteFile(path, []byte("predecessor"), 0o644))
	}
}

func assertPredecessorsGone(t *testing.T, layout project.Layout) {
	t.Helper()
	for _, path := range predecessorFiles(layout) {
		assert.NoFileExists(t, path)
	}
	assert.NoDirExists(t, oldWorkDir(layout), "the work directory goes once it is empty")
}

// A staged decision is the one thing a predecessor working store holds that no
// committed source can reproduce, so it is the one thing the sweep carries.
func TestSweep_CarriesStagedDecisionsForward(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(layout.StateDir, 0o755))

	// A committed record the new working store will re-seed from, so the test
	// can tell a carried decision from a re-derived one.
	require.NoError(t, state.WriteCommitted(layout.UnitsDir(), []state.UnitState{
		unit("u-committed", "d-intro", "Already committed"),
	}))

	old, err := state.OpenWork(t.Context(), oldWorkStorePath(layout), layout.UnitsDir())
	require.NoError(t, err)
	require.NoError(t, old.Put(t.Context(), unit("u-staged", "d-intro", "Staged, never committed")))
	pending, err := old.Pending(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, pending, "the seeded unit is not staged; the new decision is")
	require.NoError(t, old.Close())

	writePredecessorLayout(t, layout)

	db := openStore(t, layout)

	staged, err := db.Work().Staged(t.Context())
	require.NoError(t, err)
	require.Len(t, staged, 1, "exactly the staged decision came across")
	assert.Equal(t, "u-staged", staged[0].Unit)
	assert.Equal(t, "approved", staged[0].Decision.ReviewState)

	// The committed unit is present too, but as a re-seed from the committed
	// shards rather than a carried decision.
	all, err := db.Work().All(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 2)
	_, ok := db.Work().Get(t.Context(), state.Key{Unit: "u-committed", Variant: model.Variant("nb")})
	assert.True(t, ok)

	assertPredecessorsGone(t, layout)
}

// The browser build's predecessor is a JSON sidecar beside the database it
// stood in for. Same contract: staged decisions come across, the file goes.
func TestSweep_CarriesStagedDecisionsFromSidecar(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(oldWorkDir(layout), 0o755))

	sidecar := filepath.Join(oldWorkDir(layout), "state.json")
	old, err := state.OpenWorkSidecar(t.Context(), sidecar, layout.UnitsDir())
	require.NoError(t, err)
	require.NoError(t, old.Put(t.Context(), unit("u-sidecar", "d-intro", "Staged in the browser")))
	require.NoError(t, old.Close())
	require.FileExists(t, sidecar)

	db := openStore(t, layout)

	staged, err := db.Work().Staged(t.Context())
	require.NoError(t, err)
	require.Len(t, staged, 1)
	assert.Equal(t, "u-sidecar", staged[0].Unit)

	assert.NoFileExists(t, sidecar)
	assert.NoDirExists(t, oldWorkDir(layout))
}

// No predecessor is the ordinary case, and the common one after the first
// open: the sweep must be a no-op that costs a few stats.
func TestSweep_NoPredecessorIsNoOp(t *testing.T) {
	layout := newLayout(t)
	db := openStore(t, layout)

	pending, err := db.Work().Pending(t.Context())
	require.NoError(t, err)
	assert.Zero(t, pending)
	assert.NoDirExists(t, oldWorkDir(layout))

	// Re-opening finds nothing to sweep and changes nothing.
	require.NoError(t, db.Close())
	again := openStore(t, layout)
	pending, err = again.Work().Pending(t.Context())
	require.NoError(t, err)
	assert.Zero(t, pending)
}

// The sweep runs on every open, so a predecessor that reappears (a stale file
// restored from a backup, a branch switch) is swept the next time too.
func TestSweep_RunsOnEveryOpen(t *testing.T) {
	layout := newLayout(t)
	db := openStore(t, layout)
	require.NoError(t, db.Close())

	writePredecessorLayout(t, layout)
	openStore(t, layout)
	assertPredecessorsGone(t, layout)
}

// A work directory holding something the sweep does not own stays, along with
// what is in it: deleting an unrecognised file is not the sweep's business.
func TestSweep_LeavesUnrecognisedWorkDirContents(t *testing.T) {
	layout := newLayout(t)
	writePredecessorLayout(t, layout)
	stray := filepath.Join(oldWorkDir(layout), "notes.txt")
	require.NoError(t, os.WriteFile(stray, []byte("mine"), 0o644))

	openStore(t, layout)

	assert.FileExists(t, stray)
	assert.DirExists(t, oldWorkDir(layout))
	assert.NoFileExists(t, oldWorkStorePath(layout), "the store this package owns still goes")
}

// A predecessor working store that will not open must not fail the project
// open: the alternative is a project that cannot be used because of a file
// about to be deleted.
func TestSweep_UnreadablePredecessorDoesNotFailOpen(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(oldWorkDir(layout), 0o755))
	require.NoError(t, os.WriteFile(oldWorkStorePath(layout), []byte("this is not a database"), 0o644))

	db, err := projectdb.Open(t.Context(), layout)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NotNil(t, db.Work())
	assert.NoFileExists(t, oldWorkStorePath(layout))
}
