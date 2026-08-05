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

// The two predecessor layouts, spelled here as the test's own fixtures. The
// names are no longer constants anywhere: core/projectdb keeps private copies
// precisely so nothing live can reach for one, and a test that borrowed them
// would assert against the implementation rather than against the disk.
func flatCacheDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "cache")
}

func flatStorePath(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "store.db")
}

func flatDecisionsDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "units")
}

func flatVaultDir(layout project.Layout) string {
	return filepath.Join(layout.StateDir, "vault")
}

func oldBlockStorePath(layout project.Layout) string {
	return filepath.Join(flatCacheDir(layout), "blocks.db")
}

// oldWorkStorePath is the four-file layout's working store. Its directory is
// the one the current layout keeps ALL machine state in, so the file goes and
// the directory stays — the one asymmetry these tests exist to pin.
func oldWorkStorePath(layout project.Layout) string {
	return filepath.Join(layout.WorkDir(), "state.db")
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
	require.NoError(t, os.MkdirAll(flatCacheDir(layout), 0o755))
	require.NoError(t, os.MkdirAll(layout.WorkDir(), 0o755))
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
	assert.NoDirExists(t, flatCacheDir(layout), "the flat cache root goes whole")
	assert.DirExists(t, layout.WorkDir(), "the work directory is the live machine-state root, not a leftover")
}

// A staged decision is the one thing a predecessor working store holds that no
// committed source can reproduce, so it is the one thing the sweep carries.
func TestSweep_CarriesStagedDecisionsForward(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(layout.StateDir, 0o755))

	// A committed record the new working store will re-seed from, so the test
	// can tell a carried decision from a re-derived one.
	require.NoError(t, state.WriteCommitted(layout.DecisionsDir(), []state.UnitState{
		unit("u-committed", "d-intro", "Already committed"),
	}))

	old, err := state.OpenWork(t.Context(), oldWorkStorePath(layout), layout.DecisionsDir())
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
	require.NoError(t, os.MkdirAll(layout.WorkDir(), 0o755))

	sidecar := filepath.Join(layout.WorkDir(), "state.json")
	old, err := state.OpenWorkSidecar(t.Context(), sidecar, layout.DecisionsDir())
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
}

// No predecessor is the ordinary case, and the common one after the first
// open: the sweep must be a no-op that costs a few stats.
func TestSweep_NoPredecessorIsNoOp(t *testing.T) {
	layout := newLayout(t)
	db := openStore(t, layout)

	pending, err := db.Work().Pending(t.Context())
	require.NoError(t, err)
	assert.Zero(t, pending)
	assert.NoFileExists(t, oldWorkStorePath(layout))

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
	stray := filepath.Join(layout.WorkDir(), "notes.txt")
	require.NoError(t, os.WriteFile(stray, []byte("mine"), 0o644))

	openStore(t, layout)

	assert.FileExists(t, stray)
	assert.NoFileExists(t, oldWorkStorePath(layout), "the store this package owns still goes")
}

// A predecessor working store that will not open must not fail the project
// open: the alternative is a project that cannot be used because of a file
// about to be deleted.
func TestSweep_UnreadablePredecessorDoesNotFailOpen(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(layout.WorkDir(), 0o755))
	require.NoError(t, os.WriteFile(oldWorkStorePath(layout), []byte("this is not a database"), 0o644))

	db, err := projectdb.Open(t.Context(), layout)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NotNil(t, db.Work())
	assert.NoFileExists(t, oldWorkStorePath(layout))
}

// ─── the flat layout: `.kapi/units`, `.kapi/vault`, `.kapi/store.db` ─────────

// The committed decision record is authored data. It MOVES, byte for byte:
// a shard that arrived rewritten, or arrived not at all, is review history
// silently rewritten by a directory rename.
func TestFold_MovesCommittedDecisionsIntoContext(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, state.WriteCommitted(flatDecisionsDir(layout), []state.UnitState{
		unit("u-flat", "d-intro", "Committed under the flat layout"),
	}))

	shards, err := os.ReadDir(flatDecisionsDir(layout))
	require.NoError(t, err)
	require.Len(t, shards, 1)
	before, err := os.ReadFile(filepath.Join(flatDecisionsDir(layout), shards[0].Name()))
	require.NoError(t, err)

	db := openStore(t, layout)

	after, err := os.ReadFile(filepath.Join(layout.DecisionsDir(), shards[0].Name()))
	require.NoError(t, err, "the shard landed under .kapi/context/decisions/")
	assert.Equal(t, before, after, "the committed record moves byte-identical")
	assert.NoDirExists(t, flatDecisionsDir(layout), "an emptied source directory goes")

	// And it is a record, not just a file: the working store seeded from it,
	// which is why the fold has to precede the open.
	_, ok := db.Work().Get(t.Context(), state.Key{Unit: "u-flat", Variant: model.Variant("nb")})
	assert.True(t, ok, "the moved record seeded the working set on this same open")
}

// The vault holds the only copy of a withheld original. It moves for the same
// reason the decision record does, and is the one thing under work/ that kapi
// never deletes on its own initiative.
func TestFold_MovesVaultIntoWork(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(filepath.Join(flatVaultDir(layout), "batches"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(flatVaultDir(layout), "redaction.json"),
		[]byte(`{"secret":"the original"}`), 0o600))
	require.NoError(t, os.WriteFile(
		filepath.Join(flatVaultDir(layout), "batches", "b-1.json"),
		[]byte(`{"secret":"nested"}`), 0o600))

	openStore(t, layout)

	moved, err := os.ReadFile(layout.RedactionVaultPath())
	require.NoError(t, err, "the project vault landed under .kapi/work/vault/")
	assert.JSONEq(t, `{"secret":"the original"}`, string(moved))

	nested, err := os.ReadFile(filepath.Join(layout.VaultDir(), "batches", "b-1.json"))
	require.NoError(t, err, "a subdirectory of the vault moves whole")
	assert.JSONEq(t, `{"secret":"nested"}`, string(nested))

	assert.NoDirExists(t, flatVaultDir(layout))
}

// The flat store and cache are projections under a path nothing reads any
// more. They go; nothing is carried out of them.
func TestFold_RetiresFlatStoreAndCache(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, os.MkdirAll(filepath.Join(flatCacheDir(layout), "extractions"), 0o755))
	for _, path := range []string{
		flatStorePath(layout),
		flatStorePath(layout) + "-wal",
		flatStorePath(layout) + "-shm",
		filepath.Join(layout.StateDir, "store.json"),
		filepath.Join(flatCacheDir(layout), "extractions", "b-1.json"),
	} {
		require.NoError(t, os.WriteFile(path, []byte("flat"), 0o644))
	}

	openStore(t, layout)

	for _, path := range []string{
		flatStorePath(layout),
		flatStorePath(layout) + "-wal",
		flatStorePath(layout) + "-shm",
		filepath.Join(layout.StateDir, "store.json"),
	} {
		assert.NoFileExists(t, path)
	}
	assert.NoDirExists(t, flatCacheDir(layout))
	assert.FileExists(t, layout.StorePath(), "the live store is the one under work/")
}

// The second open must find nothing to do. A fold that ran twice on a project
// that had already been folded would be a fold that could move live data.
func TestFold_SecondOpenIsANoOp(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, state.WriteCommitted(flatDecisionsDir(layout), []state.UnitState{
		unit("u-flat", "d-intro", "Committed under the flat layout"),
	}))
	require.NoError(t, os.MkdirAll(flatVaultDir(layout), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(flatVaultDir(layout), "redaction.json"), []byte(`{"a":1}`), 0o600))

	db := openStore(t, layout)
	require.NoError(t, db.Close())

	shards, err := os.ReadDir(layout.DecisionsDir())
	require.NoError(t, err)
	require.Len(t, shards, 1)
	firstShard, err := os.ReadFile(filepath.Join(layout.DecisionsDir(), shards[0].Name()))
	require.NoError(t, err)
	firstVault, err := os.ReadFile(layout.RedactionVaultPath())
	require.NoError(t, err)

	openStore(t, layout)

	secondShard, err := os.ReadFile(filepath.Join(layout.DecisionsDir(), shards[0].Name()))
	require.NoError(t, err)
	secondVault, err := os.ReadFile(layout.RedactionVaultPath())
	require.NoError(t, err)
	assert.Equal(t, firstShard, secondShard)
	assert.Equal(t, firstVault, secondVault)
	assert.NoDirExists(t, flatDecisionsDir(layout))
	assert.NoDirExists(t, flatVaultDir(layout))
}

// A collision is the case where losing something is possible, so it is the case
// the fold refuses to resolve: the destination wins, the source stays put, and
// the leftover directory is what tells a person there was a conflict at all.
func TestFold_CollisionKeepsBothCopies(t *testing.T) {
	layout := newLayout(t)
	require.NoError(t, state.WriteCommitted(flatDecisionsDir(layout), []state.UnitState{
		unit("u-old", "d-intro", "The flat copy"),
	}))
	shards, err := os.ReadDir(flatDecisionsDir(layout))
	require.NoError(t, err)
	require.Len(t, shards, 1)

	// A shard already at the destination, holding a different decision for the
	// same document — the shape a half-migrated project would arrive in.
	require.NoError(t, state.WriteCommitted(layout.DecisionsDir(), []state.UnitState{
		unit("u-current", "d-intro", "The current copy"),
	}))
	occupied := filepath.Join(layout.DecisionsDir(), shards[0].Name())
	before, err := os.ReadFile(occupied)
	require.NoError(t, err)

	openStore(t, layout)

	kept, err := os.ReadFile(occupied)
	require.NoError(t, err)
	assert.Equal(t, before, kept, "the destination is never overwritten")
	assert.FileExists(t, filepath.Join(flatDecisionsDir(layout), shards[0].Name()),
		"and the source stays, visibly, rather than being deleted unread")
}
