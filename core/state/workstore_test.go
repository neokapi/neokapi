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
	w, err := state.OpenWork(t.Context(), filepath.Join(dir, "work", "state.db"), committed)
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

	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Put(t.Context(), unit("u2", "d-intro", "Bravo")))

	n, err := w.Pending(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, n, "both decisions are staged")

	onDisk, err := state.ReadCommitted(committed)
	require.NoError(t, err)
	assert.Empty(t, onDisk, "nothing is committed until Commit runs")

	require.NoError(t, w.Commit(t.Context()))

	onDisk, err = state.ReadCommitted(committed)
	require.NoError(t, err)
	assert.Len(t, onDisk, 2)

	n, err = w.Pending(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n, "committing clears the staged flag")
}

// Committing nothing must not rewrite the record, or every no-op run would show
// up as a change in git.
func TestWorkStore_CommitIsANoOpWhenNothingStaged(t *testing.T) {
	w, committed := openWork(t)
	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Commit(t.Context()))

	before := readShardBytes(t, committed, "d-intro.jsonl")
	require.NoError(t, w.Commit(t.Context()))
	assert.Equal(t, before, readShardBytes(t, committed, "d-intro.jsonl"))
}

// A working store is derived. Deleting it must cost nothing that was committed —
// this is what makes the directory safe to treat as disposable once committed.
func TestWorkStore_RebuildsFromTheCommittedRecord(t *testing.T) {
	dir := t.TempDir()
	committed := filepath.Join(dir, "units")
	dbPath := filepath.Join(dir, "work", "state.db")

	w, err := state.OpenWork(t.Context(), dbPath, committed)
	require.NoError(t, err)
	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Commit(t.Context()))
	require.NoError(t, w.Close())

	// Throw the working store away entirely.
	require.NoError(t, removeAll(dbPath))

	reopened, err := state.OpenWork(t.Context(), dbPath, committed)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })

	got, ok := reopened.Get(t.Context(), state.Key{Scope: "d-intro", Unit: "u1", Variant: model.VariantKey{Locale: "nb"}})
	require.True(t, ok, "a committed decision survives losing the working store")
	assert.Equal(t, "approved", got.Decision.ReviewState)

	n, err := reopened.Pending(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n, "a rebuilt store has nothing staged — it is already committed")
}

// Identity rides with the decision, so reconcile can read priors back out.
func TestWorkStore_PriorsAreScopedToTheDocument(t *testing.T) {
	w, _ := openWork(t)
	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Put(t.Context(), unit("u2", "d-guide", "Bravo")))

	priors, err := w.Priors(t.Context(), "d-intro")
	require.NoError(t, err)
	require.Len(t, priors, 1)
	assert.Equal(t, "u1", priors[0].Unit)
	assert.Equal(t, model.ComputeContentHash("Alpha"), priors[0].ContentHash)
}

// A unit id is unique inside its document and nowhere wider: every markdown page
// in a collection carries an `h`, a `p`, a `fm_title`. A store keyed on less than
// (document, unit, variant) lets the second document's decision overwrite the
// first's, and the record then holds one decision where two were made.
func TestWorkStore_SameUnitIDInTwoDocuments(t *testing.T) {
	w, committed := openWork(t)

	intro := unit("p", "d-intro", "Alpha")
	intro.Decision.Note = "intro"
	guide := unit("p", "d-guide", "Bravo")
	guide.Decision.Note = "guide"
	require.NoError(t, w.Put(t.Context(), intro))
	require.NoError(t, w.Put(t.Context(), guide))

	n, err := w.Pending(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, n, "one decision per document, not per id")

	all, err := w.All(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 2)
	notes := map[string]string{}
	for _, u := range all {
		notes[u.Scope] = u.Decision.Note
	}
	assert.Equal(t, map[string]string{"d-intro": "intro", "d-guide": "guide"}, notes,
		"each document keeps the decision made in it")

	require.NoError(t, w.Commit(t.Context()))
	onDisk, err := state.ReadCommitted(committed)
	require.NoError(t, err)
	assert.Len(t, onDisk, 2, "both documents' decisions reach the committed record")
}

// A unit is addressed, read and deleted by the same identity, so one document's
// namesake cannot be reached — or removed — through another's.
func TestWorkStore_UnitsAreAddressedByDocument(t *testing.T) {
	w, _ := openWork(t)
	intro := unit("p", "d-intro", "Alpha")
	intro.Decision.Note = "intro"
	require.NoError(t, w.Put(t.Context(), intro))
	require.NoError(t, w.Put(t.Context(), unit("p", "d-guide", "Bravo")))

	got, ok := w.Get(t.Context(), state.Key{Scope: "d-intro", Unit: "p", Variant: model.VariantKey{Locale: "nb"}})
	require.True(t, ok, "the intro's decision is addressable by its own document")
	assert.Equal(t, "intro", got.Decision.Note)
	assert.Equal(t, model.ComputeContentHash("Alpha"), got.ContentHash,
		"and still carries the source it was decided against")

	require.NoError(t, w.Delete(t.Context(), state.Key{Scope: "d-intro", Unit: "p", Variant: model.VariantKey{Locale: "nb"}}))
	left, err := w.All(t.Context())
	require.NoError(t, err)
	require.Len(t, left, 1, "one document's unit is deleted, not every unit of that id")
	assert.Equal(t, "d-guide", left[0].Scope)
}

// One file per document, so editing the docs does not rewrite the shard holding
// the interface strings.
func TestWorkStore_ShardsByDocument(t *testing.T) {
	w, committed := openWork(t)
	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Put(t.Context(), unit("u2", "d-guide", "Bravo")))
	require.NoError(t, w.Commit(t.Context()))

	assert.FileExists(t, filepath.Join(committed, "d-intro.jsonl"))
	assert.FileExists(t, filepath.Join(committed, "d-guide.jsonl"))

	// Touching one document leaves the other's bytes alone.
	before := readShardBytes(t, committed, "d-guide.jsonl")
	u := unit("u1", "d-intro", "Alpha revised")
	u.Decision.Note = "reworded"
	require.NoError(t, w.Put(t.Context(), u))
	require.NoError(t, w.Commit(t.Context()))

	assert.Equal(t, before, readShardBytes(t, committed, "d-guide.jsonl"),
		"an unrelated document's shard must not churn")
}

// A scope key is opaque and must never be able to escape the state directory.
func TestWorkStore_ScopeCannotEscapeTheStateDirectory(t *testing.T) {
	w, committed := openWork(t)
	require.NoError(t, w.Put(t.Context(), unit("u1", "../../etc/passwd", "Alpha")))
	require.NoError(t, w.Commit(t.Context()))

	entries := shardNames(t, committed)
	require.Len(t, entries, 1)
	assert.NotContains(t, entries[0], "..")
	assert.NotContains(t, entries[0], "/")
}

// A shard whose units are all gone must not linger claiming them.
func TestWorkStore_PrunesEmptiedShards(t *testing.T) {
	w, committed := openWork(t)
	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Put(t.Context(), unit("u2", "d-guide", "Bravo")))
	require.NoError(t, w.Commit(t.Context()))
	require.Len(t, shardNames(t, committed), 2)

	require.NoError(t, w.Delete(t.Context(), state.Key{Scope: "d-guide", Unit: "u2", Variant: model.VariantKey{Locale: "nb"}}))
	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Commit(t.Context()))

	assert.Equal(t, []string{"d-intro.jsonl"}, shardNames(t, committed))
}
