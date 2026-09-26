package state_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// The address covers what an entry asserts and nothing else, so two parties
// reaching the same decision about the same pairing write the same entry.
func TestAddress_CoversWhatTheEntryAsserts(t *testing.T) {
	base := paired("u1", "d-intro", "Alpha", "t1", "2026-01-01T00:00:00Z")

	first, err := state.Address(base, "reviewer@example.test", false)
	require.NoError(t, err)
	again, err := state.Address(base, "reviewer@example.test", false)
	require.NoError(t, err)
	assert.Equal(t, first, again, "the same assertion addresses the same")

	other, err := state.Address(base, "someone-else@example.test", false)
	require.NoError(t, err)
	assert.NotEqual(t, first, other, "who decided is part of what the entry says")

	revoked, err := state.Address(base, "reviewer@example.test", true)
	require.NoError(t, err)
	assert.NotEqual(t, first, revoked, "and so is a withdrawal")

	moved := base
	moved.TargetHash = "t2"
	atOther, err := state.Address(moved, "reviewer@example.test", false)
	require.NoError(t, err)
	assert.NotEqual(t, first, atOther, "a decision about another translation is another entry")
}

// Recording the same decision twice holds it once, whichever route it arrives
// by. A pull that serves the venue's whole ledger on every page depends on it.
func TestWorkStore_RecordingTheSameDecisionTwiceHoldsItOnce(t *testing.T) {
	w, _ := openWork(t)
	u := paired("u1", "d-intro", "Alpha", "t1", "2026-01-01T00:00:00Z")

	require.NoError(t, w.Put(t.Context(), u))
	require.NoError(t, w.RecordEntry(t.Context(), u, u.Decision.By, state.OriginVenue))
	require.NoError(t, w.Put(t.Context(), u))

	entries, err := w.Entries(t.Context(), nbKey("d-intro", "u1"))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, state.OriginLocal, entries[0].Origin,
		"the route that got there first is the one on record")
}

// A newer entry at one pairing supersedes the one before it, and the one before
// it is still readable.
func TestWorkStore_TheNewestEntryAtAPairingAnswers(t *testing.T) {
	w, _ := openWork(t)

	approved := paired("u1", "d-intro", "Alpha", "t1", "2026-01-01T00:00:00Z")
	require.NoError(t, w.Put(t.Context(), approved))

	rejected := approved
	rejected.Status = model.TargetStatusTranslated
	rejected.Decision = state.Decision{ReviewState: "rejected", At: "2026-01-02T00:00:00Z"}
	rejected.Updated = "2026-01-02T00:00:00Z"
	require.NoError(t, w.Put(t.Context(), rejected))

	got, ok := w.Get(t.Context(), nbKey("d-intro", "u1"))
	require.True(t, ok)
	assert.Equal(t, "rejected", got.Decision.ReviewState)

	entries, err := w.Entries(t.Context(), nbKey("d-intro", "u1"))
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "rejected", entries[0].State.Decision.ReviewState)
	assert.Equal(t, "approved", entries[1].State.Decision.ReviewState)
}

// An import records what a line says, ordered by the record's own stamp rather
// than by the moment of the import, so reading the shards back cannot overtake
// a decision made since.
func TestWorkStore_AnImportDoesNotOvertakeALaterDecision(t *testing.T) {
	w, committed := openWork(t)

	newer := paired("u1", "d-intro", "Alpha", "t1", "2026-06-01T00:00:00Z")
	newer.Decision.Note = "decided here"
	require.NoError(t, w.Put(t.Context(), newer))

	older := paired("u1", "d-intro", "Alpha", "t1", "2026-01-01T00:00:00Z")
	older.Decision.Note = "decided elsewhere"
	require.NoError(t, state.WriteCommitted(committed, []state.UnitState{older}))
	require.NoError(t, w.Import(t.Context()))

	got, ok := w.Get(t.Context(), nbKey("d-intro", "u1"))
	require.True(t, ok)
	assert.Equal(t, "decided here", got.Decision.Note,
		"the later decision still answers for the pairing")
}

// Every write goes through the policy, so an actor class with narrower rights
// is a change in one function rather than at each call site.
func TestWorkStore_PolicyRefusesATransition(t *testing.T) {
	w, _ := openWork(t)

	refused := errors.New("agents may not reject")
	w.SetPolicy(func(tr state.Transition) error {
		if tr.Proposed.Decision.ReviewState == "rejected" && tr.Actor == "agent/lab" {
			return refused
		}
		return nil
	})

	allowed := paired("u1", "d-intro", "Alpha", "t1", "2026-01-01T00:00:00Z")
	require.NoError(t, w.RecordEntry(t.Context(), allowed, "agent/lab", state.OriginLocal))

	blocked := allowed
	blocked.Decision.ReviewState = "rejected"
	err := w.RecordEntry(t.Context(), blocked, "agent/lab", state.OriginLocal)
	require.ErrorIs(t, err, refused)

	got, ok := w.Get(t.Context(), nbKey("d-intro", "u1"))
	require.True(t, ok)
	assert.Equal(t, "approved", got.Decision.ReviewState, "the refused entry was not recorded")
}

// The policy sees what applies at the pairing, which is what a rule about a
// transition needs.
func TestWorkStore_PolicySeesWhatApplies(t *testing.T) {
	w, _ := openWork(t)
	first := paired("u1", "d-intro", "Alpha", "t1", "2026-01-01T00:00:00Z")
	require.NoError(t, w.Put(t.Context(), first))

	var seen state.Transition
	w.SetPolicy(func(tr state.Transition) error {
		seen = tr
		return nil
	})
	second := first
	second.Decision.Note = "second look"
	second.Updated = "2026-01-02T00:00:00Z"
	require.NoError(t, w.RecordEntry(t.Context(), second, "reviewer", state.OriginLocal))

	assert.True(t, seen.Applied)
	assert.Equal(t, "approved", seen.Applies.Decision.ReviewState)
	assert.Equal(t, "reviewer", seen.Actor)
	assert.Equal(t, state.OriginLocal, seen.Origin)
	assert.Equal(t, first.Pairing(), seen.Pairing)
}
