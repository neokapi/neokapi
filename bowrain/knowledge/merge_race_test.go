package knowledge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
