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

func nbKey(scope, id string) state.Key {
	return state.Key{Scope: scope, Unit: id, Variant: model.VariantKey{Locale: "nb"}}
}

// A decision is durable where it is recorded. Writing the committed record is a
// separate act, and until it runs the record on disk says nothing about the
// decision.
func TestWorkStore_RecordsDurablyAndExportsOnCommit(t *testing.T) {
	w, committed := openWork(t)

	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Put(t.Context(), unit("u2", "d-intro", "Bravo")))

	got, ok := w.Get(t.Context(), nbKey("d-intro", "u1"))
	require.True(t, ok, "the decision answers for the unit at once")
	assert.Equal(t, "approved", got.Decision.ReviewState)

	diff, err := w.RecordDiff(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, diff.Changed(), "both records are missing from the committed shards")

	onDisk, err := state.ReadCommitted(committed)
	require.NoError(t, err)
	assert.Empty(t, onDisk, "nothing reaches the shards until the record is written")

	require.NoError(t, w.Commit(t.Context()))

	onDisk, err = state.ReadCommitted(committed)
	require.NoError(t, err)
	assert.Len(t, onDisk, 2)

	diff, err = w.RecordDiff(t.Context())
	require.NoError(t, err)
	assert.Zero(t, diff.Changed(), "the shards now carry what the checkout holds")
}

// Writing an unchanged project must produce the same bytes and leave the files
// alone, or every no-op run would show up in git.
func TestWorkStore_CommitIsByteStableAcrossRuns(t *testing.T) {
	w, committed := openWork(t)
	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Put(t.Context(), unit("u2", "d-guide", "Bravo")))
	require.NoError(t, w.Commit(t.Context()))

	before := readShardBytes(t, committed, "d-intro.jsonl")
	beforeStat := shardModTime(t, committed, "d-intro.jsonl")

	require.NoError(t, w.Commit(t.Context()))
	assert.Equal(t, before, readShardBytes(t, committed, "d-intro.jsonl"))
	assert.Equal(t, beforeStat, shardModTime(t, committed, "d-intro.jsonl"),
		"an unchanged shard is not rewritten")
}

// The ledger is the authority and the shards are an export of it, so throwing
// the database away costs nothing that has been written out.
func TestWorkStore_RebuildsFromTheCommittedRecord(t *testing.T) {
	dir := t.TempDir()
	committed := filepath.Join(dir, "units")
	dbPath := filepath.Join(dir, "work", "state.db")

	w, err := state.OpenWork(t.Context(), dbPath, committed)
	require.NoError(t, err)
	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Commit(t.Context()))
	require.NoError(t, w.Close())

	// Throw the store away entirely.
	require.NoError(t, removeAll(dbPath))

	reopened, err := state.OpenWork(t.Context(), dbPath, committed)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })

	got, ok := reopened.Get(t.Context(), nbKey("d-intro", "u1"))
	require.True(t, ok, "a written decision survives losing the store")
	assert.Equal(t, "approved", got.Decision.ReviewState)

	diff, err := reopened.RecordDiff(t.Context())
	require.NoError(t, err)
	assert.Zero(t, diff.Changed(), "an imported record is already what the shards carry")
}

// Importing the same shards again must not grow the ledger: an entry is
// addressed by what it says.
func TestWorkStore_ImportIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	committed := filepath.Join(dir, "units")
	require.NoError(t, state.WriteCommitted(committed, []state.UnitState{
		stamped(unit("u1", "d-intro", "Alpha"), "2026-01-01T00:00:00Z"),
	}))
	dbPath := filepath.Join(dir, "work", "state.db")

	w, err := state.OpenWork(t.Context(), dbPath, committed)
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	entries, err := w.Entries(t.Context(), nbKey("d-intro", "u1"))
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// Rewrite the same bytes with a different modification time, which moves
	// nothing the digest covers, then force a re-read by writing them again.
	require.NoError(t, state.WriteCommitted(committed, []state.UnitState{
		stamped(unit("u1", "d-intro", "Alpha"), "2026-01-01T00:00:00Z"),
		stamped(unit("u2", "d-intro", "Bravo"), "2026-01-01T00:00:00Z"),
	}))
	require.NoError(t, w.Import(t.Context()))
	require.NoError(t, w.Import(t.Context()))

	entries, err = w.Entries(t.Context(), nbKey("d-intro", "u1"))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the line the ledger already holds is left alone")
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

// A unit is addressed, read and withdrawn by the same identity, so one
// document's namesake cannot be reached, or dropped, through another's.
func TestWorkStore_UnitsAreAddressedByDocument(t *testing.T) {
	w, _ := openWork(t)
	intro := unit("p", "d-intro", "Alpha")
	intro.Decision.Note = "intro"
	require.NoError(t, w.Put(t.Context(), intro))
	require.NoError(t, w.Put(t.Context(), unit("p", "d-guide", "Bravo")))

	got, ok := w.Get(t.Context(), nbKey("d-intro", "p"))
	require.True(t, ok, "the intro's decision is addressable by its own document")
	assert.Equal(t, "intro", got.Decision.Note)
	assert.Equal(t, model.ComputeContentHash("Alpha"), got.ContentHash,
		"and still carries the source it was decided against")

	require.NoError(t, w.Delete(t.Context(), nbKey("d-intro", "p")))
	left, err := w.All(t.Context())
	require.NoError(t, err)
	require.Len(t, left, 1, "one document's unit is withdrawn, not every unit of that id")
	assert.Equal(t, "d-guide", left[0].Scope)
}

// A withdrawal is an entry of its own: the unit stops answering, and the
// history of what was decided about it is still there to read.
func TestWorkStore_DeleteRevokesWithoutErasing(t *testing.T) {
	w, _ := openWork(t)
	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Delete(t.Context(), nbKey("d-intro", "u1")))

	_, ok := w.Get(t.Context(), nbKey("d-intro", "u1"))
	assert.False(t, ok, "nothing applies to the unit any more")

	entries, err := w.Entries(t.Context(), nbKey("d-intro", "u1"))
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.True(t, entries[0].Revoked, "the withdrawal is the most recent entry")
	assert.False(t, entries[1].Revoked, "and the decision it withdrew is still on record")
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

	require.NoError(t, w.Delete(t.Context(), nbKey("d-guide", "u2")))
	require.NoError(t, w.Commit(t.Context()))

	assert.Equal(t, []string{"d-intro.jsonl"}, shardNames(t, committed))
}
