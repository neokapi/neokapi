package state_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

func openWork(t *testing.T) (*state.WorkStore, string) {
	t.Helper()
	dir := t.TempDir()
	committed := filepath.Join(dir, "units")
	w, err := state.OpenWork(filepath.Join(dir, "work", "state.db"), committed)
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	return w, committed
}

func unit(id, scope, text string) state.UnitState {
	return state.UnitState{
		Unit:        id,
		Variant:     model.VariantKey{Locale: "nb"},
		Scope:       scope,
		ContentHash: model.ComputeContentHash(text),
		ContextHash: "ctx-" + id,
		Decision:    state.Decision{ReviewState: "approved"},
	}
}

// The point of the working store: a decision is recorded without touching the
// committed record, and one write covers all of them.
func TestWorkStore_StagesUntilCommit(t *testing.T) {
	w, committed := openWork(t)

	require.NoError(t, w.Put(unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Put(unit("u2", "d-intro", "Bravo")))

	n, err := w.Pending()
	require.NoError(t, err)
	assert.Equal(t, 2, n, "both decisions are staged")

	onDisk, err := state.ReadCommitted(committed)
	require.NoError(t, err)
	assert.Empty(t, onDisk, "nothing is committed until Commit runs")

	require.NoError(t, w.Commit())

	onDisk, err = state.ReadCommitted(committed)
	require.NoError(t, err)
	assert.Len(t, onDisk, 2)

	n, err = w.Pending()
	require.NoError(t, err)
	assert.Zero(t, n, "committing clears the staged flag")
}

// Committing nothing must not rewrite the record, or every no-op run would show
// up as a change in git.
func TestWorkStore_CommitIsANoOpWhenNothingStaged(t *testing.T) {
	w, committed := openWork(t)
	require.NoError(t, w.Put(unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Commit())

	before := readShardBytes(t, committed, "d-intro.jsonl")
	require.NoError(t, w.Commit())
	assert.Equal(t, before, readShardBytes(t, committed, "d-intro.jsonl"))
}

// A working store is derived. Deleting it must cost nothing that was committed —
// this is what makes the directory safe to treat as disposable once committed.
func TestWorkStore_RebuildsFromTheCommittedRecord(t *testing.T) {
	dir := t.TempDir()
	committed := filepath.Join(dir, "units")
	dbPath := filepath.Join(dir, "work", "state.db")

	w, err := state.OpenWork(dbPath, committed)
	require.NoError(t, err)
	require.NoError(t, w.Put(unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Commit())
	require.NoError(t, w.Close())

	// Throw the working store away entirely.
	require.NoError(t, removeAll(dbPath))

	reopened, err := state.OpenWork(dbPath, committed)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })

	got, ok := reopened.Get(state.Key{Unit: "u1", Variant: model.VariantKey{Locale: "nb"}})
	require.True(t, ok, "a committed decision survives losing the working store")
	assert.Equal(t, "approved", got.Decision.ReviewState)

	n, err := reopened.Pending()
	require.NoError(t, err)
	assert.Zero(t, n, "a rebuilt store has nothing staged — it is already committed")
}

// Identity rides with the decision, so reconcile can read priors back out.
func TestWorkStore_PriorsAreScopedToTheDocument(t *testing.T) {
	w, _ := openWork(t)
	require.NoError(t, w.Put(unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Put(unit("u2", "d-guide", "Bravo")))

	priors, err := w.Priors("d-intro")
	require.NoError(t, err)
	require.Len(t, priors, 1)
	assert.Equal(t, "u1", priors[0].Unit)
	assert.Equal(t, model.ComputeContentHash("Alpha"), priors[0].ContentHash)
}

// One file per document, so editing the docs does not rewrite the shard holding
// the interface strings.
func TestWorkStore_ShardsByDocument(t *testing.T) {
	w, committed := openWork(t)
	require.NoError(t, w.Put(unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Put(unit("u2", "d-guide", "Bravo")))
	require.NoError(t, w.Commit())

	assert.FileExists(t, filepath.Join(committed, "d-intro.jsonl"))
	assert.FileExists(t, filepath.Join(committed, "d-guide.jsonl"))

	// Touching one document leaves the other's bytes alone.
	before := readShardBytes(t, committed, "d-guide.jsonl")
	u := unit("u1", "d-intro", "Alpha revised")
	u.Decision.Note = "reworded"
	require.NoError(t, w.Put(u))
	require.NoError(t, w.Commit())

	assert.Equal(t, before, readShardBytes(t, committed, "d-guide.jsonl"),
		"an unrelated document's shard must not churn")
}

// A scope key is opaque and must never be able to escape the state directory.
func TestWorkStore_ScopeCannotEscapeTheStateDirectory(t *testing.T) {
	w, committed := openWork(t)
	require.NoError(t, w.Put(unit("u1", "../../etc/passwd", "Alpha")))
	require.NoError(t, w.Commit())

	entries := shardNames(t, committed)
	require.Len(t, entries, 1)
	assert.NotContains(t, entries[0], "..")
	assert.NotContains(t, entries[0], "/")
}

// A shard whose units are all gone must not linger claiming them.
func TestWorkStore_PrunesEmptiedShards(t *testing.T) {
	w, committed := openWork(t)
	require.NoError(t, w.Put(unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Put(unit("u2", "d-guide", "Bravo")))
	require.NoError(t, w.Commit())
	require.Len(t, shardNames(t, committed), 2)

	require.NoError(t, w.Delete(state.Key{Unit: "u2", Variant: model.VariantKey{Locale: "nb"}}))
	require.NoError(t, w.Put(unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Commit())

	assert.Equal(t, []string{"d-intro.jsonl"}, shardNames(t, committed))
}
