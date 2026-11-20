package knowledge

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSameOps(t *testing.T) {
	op := func(kind OpType, payload string) *ChangeSetOp {
		return &ChangeSetOp{Op: kind, Payload: json.RawMessage(payload)}
	}
	t.Run("key order and whitespace do not count", func(t *testing.T) {
		a := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"x","note":"n"}`)}
		b := []*ChangeSetOp{op(OpConceptDelete, `{ "note": "n", "concept_id": "x" }`)}
		assert.True(t, SameOps(a, b))
	})
	t.Run("a differing payload counts", func(t *testing.T) {
		a := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"x"}`)}
		b := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"y"}`)}
		assert.False(t, SameOps(a, b))
	})
	t.Run("op order counts", func(t *testing.T) {
		a := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"x"}`), op(OpConceptDelete, `{"concept_id":"y"}`)}
		b := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"y"}`), op(OpConceptDelete, `{"concept_id":"x"}`)}
		assert.False(t, SameOps(a, b))
	})
	t.Run("length counts", func(t *testing.T) {
		a := []*ChangeSetOp{op(OpConceptDelete, `{"concept_id":"x"}`)}
		assert.False(t, SameOps(a, nil))
	})
}

func TestSupersededTransitions(t *testing.T) {
	require.NoError(t, ValidateStatusTransition(ChangeSetDraft, ChangeSetSuperseded))
	require.NoError(t, ValidateStatusTransition(ChangeSetInReview, ChangeSetSuperseded))
	// Decided change-sets are history, not candidates for replacement.
	require.Error(t, ValidateStatusTransition(ChangeSetMerged, ChangeSetSuperseded))
	require.Error(t, ValidateStatusTransition(ChangeSetAbandoned, ChangeSetSuperseded))
	// Superseded is terminal.
	require.Error(t, ValidateStatusTransition(ChangeSetSuperseded, ChangeSetInReview))
}

func TestSubmitRetiresOnlyReplacedPilots(t *testing.T) {
	for _, tc := range []struct {
		name        string
		replacement string
		unchanged   bool
	}{
		{"superseded proposal", "different", false},
		{"duplicate draft", "original", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			st := newMemStore()
			tb := newSQLiteTB(t)
			engine := NewEngine(nil, tb, st)
			makeDraft := func(id, conceptID string) ChangeSet {
				t.Helper()
				cs := ChangeSet{ID: id, WorkspaceID: "ws", Origin: "terms-push", CreatedBy: "author"}
				require.NoError(t, st.CreateChangeSet(ctx, &cs))
				appendOp(t, st, "ws", cs.ID, 0, OpConceptCreate, ConceptCreatePayload{Concept: concept(conceptID)})
				return cs
			}
			prior := makeDraft("prior", "original")
			require.NoError(t, st.SetChangeSetStatus(ctx, "ws", prior.ID, ChangeSetInReview))
			draft := makeDraft("draft", tc.replacement)
			for _, cs := range []ChangeSet{prior, draft} {
				_, err := engine.StartPilot(ctx, "ws", st, cs, "project", "preview")
				require.NoError(t, err)
			}
			ops, err := st.ListOps(ctx, "ws", draft.ID)
			require.NoError(t, err)
			_, err = SubmitChangeSet(ctx, st, &draft, ops)
			require.ErrorContains(t, err, "requires the workspace terms store")
			before, total, err := tb.SearchForStream(ctx, "", "", "", "preview", nil, 0, 10)
			require.NoError(t, err)
			require.Len(t, before, 2)
			require.Equal(t, 2, total)

			result, err := engine.SubmitChangeSet(ctx, &draft, ops)
			require.NoError(t, err)
			assert.Equal(t, tc.unchanged, result.Unchanged)
			retired, kept, retiredStatus := prior, draft, ChangeSetSuperseded
			if tc.unchanged {
				retired, kept, retiredStatus = draft, prior, ChangeSetAbandoned
			}
			assert.Equal(t, kept.ID, result.ID)
			got, err := st.GetChangeSet(ctx, "ws", retired.ID)
			require.NoError(t, err)
			assert.Equal(t, retiredStatus, got.Status)
			pilots, err := st.ListPilots(ctx, "ws", retired.ID)
			require.NoError(t, err)
			assert.Empty(t, pilots)
			pilots, err = st.ListPilots(ctx, "ws", kept.ID)
			require.NoError(t, err)
			require.Len(t, pilots, 1)
			after, total, err := tb.SearchForStream(ctx, "", "", "", "preview", nil, 0, 10)
			require.NoError(t, err)
			require.Len(t, after, 1)
			assert.Equal(t, 1, total)
			assert.Contains(t, after[0].ID, pilots[0].shadowNamespace("c"))
			events := result.RetirementEvents("submitter", "owner")
			wantEvents := []MergeEvent{{Type: EventPilotStopped, WorkspaceID: "ws", ChangesetID: retired.ID, ProjectID: "project", Stream: "preview", Actor: "submitter", OnBehalfOf: "owner"}}
			if !tc.unchanged {
				wantEvents = append(wantEvents, MergeEvent{Type: EventChangeSetSuperseded, WorkspaceID: "ws", ChangesetID: retired.ID, Actor: "submitter", OnBehalfOf: "owner"})
			}
			assert.Equal(t, wantEvents, events)
		})
	}
}

func TestSubmitKeepsEquivalentOpenProposalAsSuccessor(t *testing.T) {
	ctx := t.Context()
	st := newMemStore()
	draft := func(id, conceptID string) ChangeSet {
		t.Helper()
		cs := ChangeSet{ID: id, WorkspaceID: "ws", Origin: "push"}
		require.NoError(t, st.CreateChangeSet(ctx, &cs))
		appendOp(t, st, "ws", id, 0, OpConceptCreate, ConceptCreatePayload{Concept: concept(conceptID)})
		return cs
	}
	retired := draft("older", "old")
	kept := draft("review", "same")
	require.NoError(t, st.SetChangeSetStatus(ctx, "ws", kept.ID, ChangeSetInReview))
	incoming := draft("incoming", "same")
	ops, err := st.ListOps(ctx, "ws", incoming.ID)
	require.NoError(t, err)
	res, err := SubmitChangeSet(ctx, st, &incoming, ops)
	require.NoError(t, err)
	assert.True(t, res.Unchanged)
	assert.Equal(t, kept.ID, res.ID)
	prior, err := st.GetChangeSet(ctx, "ws", retired.ID)
	require.NoError(t, err)
	assert.Equal(t, kept.ID, prior.SupersededBy)
	open, err := st.ListOpenChangeSetsByOrigin(ctx, "ws", "push")
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, kept.ID, open[0].ID)
}
