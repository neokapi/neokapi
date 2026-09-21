package state_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/storage"
)

// One project's ledger is shared by every checkout of it, and two of them sit
// on different branches at the same time. These tests drive that arrangement
// with two real git worktrees and ONE database handle: what each checkout reads
// and writes is decided by the pairings its own files carry.

// twoCheckouts is a repository with two worktrees on different branches, and
// the one store they share.
type twoCheckouts struct {
	t        *testing.T
	main     string // the main worktree's record directory
	feature  string // the feature worktree's record directory
	db       *storage.DB
	mainRoot string
	featRoot string
}

func (c *twoCheckouts) git(dir string, args ...string) {
	c.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(c.t, err, "git %v: %s", args, out)
}

// openMain and openFeature return a handle per checkout over the shared pool,
// which is how a per-project store serves several checkouts at once.
func (c *twoCheckouts) openMain() *state.WorkStore    { return c.open(c.main) }
func (c *twoCheckouts) openFeature() *state.WorkStore { return c.open(c.feature) }

// open binds a checkout to the shared ledger and reads its record in, which is
// what `kapi context import` does in that checkout. Opening alone reads nothing.
func (c *twoCheckouts) open(record string) *state.WorkStore {
	c.t.Helper()
	w, err := state.OpenWorkFromDB(c.t.Context(), c.db, record)
	require.NoError(c.t, err)
	require.NoError(c.t, w.Import(c.t.Context()))
	return w
}

// newTwoCheckouts builds the fixture. Both branches hold a record for the same
// unit: `main` blesses the translation of Alpha, `feature` blesses a different
// translation of the same source.
func newTwoCheckouts(t *testing.T) *twoCheckouts {
	t.Helper()
	root := t.TempDir()
	mainRoot := filepath.Join(root, "main")
	featRoot := filepath.Join(root, "feature")
	require.NoError(t, os.MkdirAll(mainRoot, 0o755))

	c := &twoCheckouts{t: t, mainRoot: mainRoot, featRoot: featRoot}
	c.git(mainRoot, "init", "--initial-branch=main")
	c.git(mainRoot, "config", "user.email", "fixture@example.test")
	c.git(mainRoot, "config", "user.name", "Fixture")

	c.main = filepath.Join(mainRoot, ".kapi", "state")
	require.NoError(t, os.MkdirAll(filepath.Join(mainRoot, ".kapi"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mainRoot, ".kapi", ".gitignore"),
		[]byte("work/\n"), 0o644))
	require.NoError(t, state.WriteCommitted(c.main, []state.UnitState{
		paired("u1", "d-intro", "Alpha", "hash-main", "2026-01-01T00:00:00Z"),
	}))
	c.git(mainRoot, "add", ".kapi")
	c.git(mainRoot, "commit", "-m", "record on main")

	c.git(mainRoot, "branch", "feature")
	c.git(mainRoot, "worktree", "add", featRoot, "feature")
	c.feature = filepath.Join(featRoot, ".kapi", "state")
	require.NoError(t, state.WriteCommitted(c.feature, []state.UnitState{
		paired("u1", "d-intro", "Alpha", "hash-feature", "2026-01-02T00:00:00Z"),
	}))
	c.git(featRoot, "add", "-A", ".kapi")
	c.git(featRoot, "commit", "-m", "record on feature")

	db, err := storage.Open(filepath.Join(root, "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	c.db = db
	return c
}

// paired is a record bound to a specific pairing: the source it blessed and the
// translation it blessed.
func paired(id, scope, source, targetHash, at string) state.UnitState {
	return state.UnitState{
		Unit:        id,
		Variant:     model.VariantKey{Locale: "nb"},
		Scope:       scope,
		ContentHash: model.ComputeContentHash(source),
		TargetHash:  targetHash,
		Status:      model.TargetStatusReviewed,
		Decision:    state.Decision{ReviewState: "approved", At: at, Note: targetHash},
		Updated:     at,
	}
}

// Each checkout reads the entry recorded for the pairing its own files carry,
// however much the shared ledger holds about the same unit.
func TestTwoCheckouts_EachReadsItsOwnPairing(t *testing.T) {
	c := newTwoCheckouts(t)

	mainStore := c.openMain()
	defer func() { _ = mainStore.Close() }()
	featStore := c.openFeature()
	defer func() { _ = featStore.Close() }()

	got, ok := mainStore.Get(t.Context(), nbKey("d-intro", "u1"))
	require.True(t, ok)
	assert.Equal(t, "hash-main", got.Decision.Note, "main answers for the translation main holds")

	got, ok = featStore.Get(t.Context(), nbKey("d-intro", "u1"))
	require.True(t, ok)
	assert.Equal(t, "hash-feature", got.Decision.Note, "feature answers for its own")

	// And the lookup is explicit: a reader holding the file content asks for
	// exactly the pairing in front of it.
	got, ok = mainStore.Lookup(t.Context(), nbKey("d-intro", "u1"),
		model.ComputeContentHash("Alpha"), "hash-feature")
	require.True(t, ok, "one ledger holds both entries")
	assert.Equal(t, "hash-feature", got.Decision.Note)
}

// A decision recorded in one checkout does not reach the other's record. One
// ledger, two views, and the export is per view.
func TestTwoCheckouts_ADecisionDoesNotLeakIntoTheOthersRecord(t *testing.T) {
	c := newTwoCheckouts(t)

	featStore := c.openFeature()
	defer func() { _ = featStore.Close() }()
	require.NoError(t, featStore.Put(t.Context(),
		paired("u-feature-only", "d-feature", "Bravo", "hash-feature", "2026-01-03T00:00:00Z")))
	require.NoError(t, featStore.Commit(t.Context()))

	mainStore := c.openMain()
	defer func() { _ = mainStore.Close() }()
	require.NoError(t, mainStore.Commit(t.Context()))

	onMain, err := state.ReadCommitted(c.main)
	require.NoError(t, err)
	require.Len(t, onMain, 1)
	assert.Equal(t, "u1", onMain[0].Unit)
	assert.Equal(t, "hash-main", onMain[0].Decision.Note,
		"main's record still blesses main's translation")

	onFeature, err := state.ReadCommitted(c.feature)
	require.NoError(t, err)
	assert.Len(t, onFeature, 2, "the feature checkout keeps both of its own")
}

// A decision applies wherever its pairing appears. That is the same rule read
// the other way: two checkouts holding the same source and the same translation
// hold the same answer, and neither had to copy it.
func TestTwoCheckouts_TheSamePairingSharesTheAnswer(t *testing.T) {
	c := newTwoCheckouts(t)

	featStore := c.openFeature()
	defer func() { _ = featStore.Close() }()
	shared := paired("u-shared", "d-intro", "Charlie", "hash-shared", "2026-01-04T00:00:00Z")
	require.NoError(t, featStore.Put(t.Context(), shared))

	mainStore := c.openMain()
	defer func() { _ = mainStore.Close() }()
	got, ok := mainStore.Lookup(t.Context(), nbKey("d-intro", "u-shared"),
		shared.ContentHash, shared.TargetHash)
	require.True(t, ok, "the pairing is in the ledger, so it answers wherever it appears")
	assert.Equal(t, "approved", got.Decision.ReviewState)

	_, ok = mainStore.Get(t.Context(), nbKey("d-intro", "u-shared"))
	assert.False(t, ok, "and the unit is not in main's view until main holds it")
}

// Writing each checkout's record twice produces the same bytes both times.
func TestTwoCheckouts_ExportIsByteStable(t *testing.T) {
	c := newTwoCheckouts(t)

	mainStore := c.openMain()
	defer func() { _ = mainStore.Close() }()
	featStore := c.openFeature()
	defer func() { _ = featStore.Close() }()

	require.NoError(t, mainStore.Commit(t.Context()))
	require.NoError(t, featStore.Commit(t.Context()))

	mainBytes := readShardBytes(t, c.main, "d-intro.jsonl")
	featBytes := readShardBytes(t, c.feature, "d-intro.jsonl")
	require.NotEqual(t, mainBytes, featBytes, "the two branches hold different records")

	require.NoError(t, mainStore.Commit(t.Context()))
	require.NoError(t, featStore.Commit(t.Context()))

	assert.Equal(t, mainBytes, readShardBytes(t, c.main, "d-intro.jsonl"))
	assert.Equal(t, featBytes, readShardBytes(t, c.feature, "d-intro.jsonl"))
}

// A fresh clone restores its decisions from the shards alone.
func TestTwoCheckouts_AFreshCloneRestoresFromTheShards(t *testing.T) {
	c := newTwoCheckouts(t)

	clone := filepath.Join(t.TempDir(), "clone")
	c.git(c.mainRoot, "clone", c.mainRoot, clone)

	db, err := storage.Open(filepath.Join(t.TempDir(), "fresh.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	w, err := state.OpenWorkFromDB(t.Context(), db, filepath.Join(clone, ".kapi", "state"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	require.NoError(t, w.Import(t.Context()))

	got, ok := w.Get(t.Context(), nbKey("d-intro", "u1"))
	require.True(t, ok, "a clone with no store of its own reads its record back")
	assert.Equal(t, "hash-main", got.Decision.Note)
}
