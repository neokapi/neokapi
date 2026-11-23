package knowledge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/terms/termstest"
	"github.com/neokapi/neokapi/core/model"
)

// A merged change-set's op is a change to make, so it is applied to the
// concept as it stands when written: an edit saved between the merge's read
// and its write stays, with the op on top of it.
func TestMergeChangeSet_KeepsAnEditMadeMeanwhile(t *testing.T) {
	ctx := context.Background()
	ws := "ws"
	tb := termstest.Memory()
	_, err := tb.CreateConcept(ctx, concept("c1", term("widget", "en-US", model.TermApproved)), "")
	require.NoError(t, err)

	store := newMemStore()
	cs := &ChangeSet{ID: "cs1", WorkspaceID: ws, Name: "Tidy widget", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	appendOp(t, store, ws, cs.ID, 0, OpTermAdd, TermAddPayload{
		ConceptID: "c1", Term: term("widgets", "en-GB", model.TermAdmitted),
	})
	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	raced := termstest.Interleaved(tb, termstest.SetDefinition(t, "a person's edit"))
	_, err = NewEngine(nil, raced, store).MergeChangeSet(ctx, ws, store, *loaded)
	require.NoError(t, err)

	c, _, ok, err := tb.GetConceptAt(ctx, "c1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "a person's edit", c.Definition, "the person's edit stays")
	assert.Len(t, c.Terms, 2, "and the merged term is added to it")
}

// proposeDelete records an approved change-set that deletes c1, pinned to the
// revision c1 has when it is proposed.
func proposeDelete(t *testing.T, tb sqlterms.Store, store Store, ws string) ChangeSet {
	t.Helper()
	ctx := context.Background()
	cs := &ChangeSet{ID: "cs-del", WorkspaceID: ws, Name: "Retire widget", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	op := mustOp(t, 0, OpConceptDelete, ConceptDeletePayload{ConceptID: "c1"})
	op.WorkspaceID, op.ChangesetID = ws, cs.ID
	require.NoError(t, PinDeleteRevision(ctx, tb, &op))
	require.NoError(t, store.AppendOp(ctx, &op))
	driveToApproved(t, store, ws, cs.ID)
	require.NoError(t, store.AddReview(ctx, &ChangeSetReview{
		WorkspaceID: ws, ChangesetID: cs.ID, Reviewer: "bob", Verdict: VerdictApprove,
	}))
	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)
	return *loaded
}

// A reviewed deletion deletes the concept it was proposed for. One edited
// after the proposal is a conflict that carries the concept as it stands,
// whether the edit lands before the merge or between the merge's check and
// its delete, and the concept stays.
func TestMergeChangeSet_DeletesOnlyTheConceptItWasProposedFor(t *testing.T) {
	ctx := context.Background()
	const ws = "ws"
	seed := func() sqlterms.Store {
		tb := termstest.Memory()
		_, err := tb.CreateConcept(ctx, concept("c1", term("widget", "en-US", model.TermApproved)), "")
		require.NoError(t, err)
		return tb
	}
	assertConflict := func(t *testing.T, tb sqlterms.Store, res *MergeResult, err error) {
		t.Helper()
		require.ErrorIs(t, err, ErrMergeConflict)
		require.Len(t, res.Conflicts, 1)
		require.NotNil(t, res.Conflicts[0].Current, "the conflict shows the concept as it stands")
		assert.Equal(t, "a person's edit", res.Conflicts[0].Current.Definition)
		c, _, ok, gerr := tb.GetConceptAt(ctx, "c1")
		require.NoError(t, gerr)
		require.True(t, ok, "the concept stays")
		assert.Equal(t, "a person's edit", c.Definition)
	}

	t.Run("as proposed", func(t *testing.T) {
		tb, store := seed(), newMemStore()
		cs := proposeDelete(t, tb, store, ws)
		_, err := NewEngine(nil, tb, store).MergeChangeSet(ctx, ws, store, cs)
		require.NoError(t, err)
		_, _, ok, err := tb.GetConceptAt(ctx, "c1")
		require.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("edited after the proposal", func(t *testing.T) {
		tb, store := seed(), newMemStore()
		cs := proposeDelete(t, tb, store, ws)
		termstest.SetDefinition(t, "a person's edit")(ctx, tb, "c1")
		res, err := NewEngine(nil, tb, store).MergeChangeSet(ctx, ws, store, cs)
		assertConflict(t, tb, res, err)
	})
	t.Run("edited during the merge", func(t *testing.T) {
		tb, store := seed(), newMemStore()
		cs := proposeDelete(t, tb, store, ws)
		raced := termstest.Interleaved(tb, termstest.SetDefinition(t, "a person's edit"))
		res, err := NewEngine(nil, raced, store).MergeChangeSet(ctx, ws, store, cs)
		assertConflict(t, tb, res, err)
	})
}
