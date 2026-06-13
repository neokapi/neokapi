package knowledge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
)

// newSQLiteTB opens an in-memory framework SQLite termbase (the engine's live
// ConceptStore in the write-side tests). It requires the fts5 build tag.
func newSQLiteTB(t *testing.T) *termbase.SQLiteTermBase {
	t.Helper()
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = tb.Close() })
	return tb
}

// appendOp is a test helper that appends a payload-encoded op to a change-set in
// the store, with an optional base revision.
func appendOp(t *testing.T, store Store, ws, csID string, baseRev int64, opType OpType, payload any) {
	t.Helper()
	op := mustOp(t, 0, opType, payload)
	op.WorkspaceID = ws
	op.ChangesetID = csID
	op.BaseRev = baseRev
	require.NoError(t, store.AppendOp(context.Background(), &op))
}

// driveToApproved walks a change-set draft → in_review → approved in the store.
func driveToApproved(t *testing.T, store Store, ws, csID string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, store.SetChangeSetStatus(ctx, ws, csID, ChangeSetInReview))
	require.NoError(t, store.SetChangeSetStatus(ctx, ws, csID, ChangeSetApproved))
}

// eventTypes returns the set of event types in a merge result.
func eventTypes(res *MergeResult) map[EventType]bool {
	out := map[EventType]bool{}
	for _, e := range res.Events {
		out[e.Type] = true
	}
	return out
}

func TestMergeChangeSet_OrdinaryAppliesAndRecordsRevision(t *testing.T) {
	ctx := context.Background()
	ws := "ws"

	tb := newSQLiteTB(t)
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("widget", "en-US", model.TermApproved))))

	store := newMemStore()
	cs := &ChangeSet{ID: "cs1", WorkspaceID: ws, Name: "Tidy widget", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	// term.add is an ordinary op — an ordinary change-set merges straight from draft.
	appendOp(t, store, ws, cs.ID, 0, OpTermAdd, TermAddPayload{
		ConceptID: "c1", Term: term("widgets", "en-GB", model.TermAdmitted),
	})

	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	e := NewEngine(nil, tb, newFakeProfileStore(), store)
	res, err := e.MergeChangeSet(ctx, ws, store, *loaded)
	require.NoError(t, err)

	assert.Empty(t, res.Conflicts)
	assert.Equal(t, []string{"c1"}, res.ConceptsTouched)
	assert.Equal(t, 1, res.RevisionsCreated)
	assert.Equal(t, []int64{0}, res.AppliedOps)

	// The op was applied to the live termbase.
	c, ok, err := tb.GetConcept(ctx, "c1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Len(t, c.Terms, 2)

	// A revision was recorded against the touched concept.
	rev, err := store.LatestRev(ctx, ws, "c1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), rev)

	// The change-set is now merged.
	merged, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)
	assert.Equal(t, ChangeSetMerged, merged.Status)
	assert.Equal(t, "alice", merged.MergedBy)

	types := eventTypes(res)
	assert.True(t, types[EventConceptUpdated])
	assert.True(t, types[EventChangeSetMerged])
}

func TestMergeChangeSet_GovernedRejectedWithoutSoD(t *testing.T) {
	ctx := context.Background()
	ws := "ws"

	tb := newSQLiteTB(t)
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("foobar", "en-US", model.TermAdmitted))))

	store := newMemStore()
	cs := &ChangeSet{ID: "cs1", WorkspaceID: ws, Name: "Ban foobar", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	// term.status → forbidden is a governed op.
	appendOp(t, store, ws, cs.ID, 0, OpTermStatus, TermStatusPayload{
		ConceptID: "c1", Locale: "en-US", Text: "foobar",
		From: model.TermAdmitted, To: model.TermForbidden,
	})
	driveToApproved(t, store, ws, cs.ID)
	// Only a self-approval by the author — never satisfies separation of duties.
	require.NoError(t, store.AddReview(ctx, &ChangeSetReview{
		WorkspaceID: ws, ChangesetID: cs.ID, Reviewer: "alice", Verdict: VerdictApprove,
	}))

	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	e := NewEngine(nil, tb, newFakeProfileStore(), store)
	_, err = e.MergeChangeSet(ctx, ws, store, *loaded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "separation of duties")

	// No writes: the term is unchanged and no revision was recorded.
	c, ok, err := tb.GetConcept(ctx, "c1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, model.TermAdmitted, c.Terms[0].Status)
	rev, err := store.LatestRev(ctx, ws, "c1")
	require.NoError(t, err)
	assert.Equal(t, int64(0), rev)

	notMerged, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)
	assert.Equal(t, ChangeSetApproved, notMerged.Status)
}

func TestMergeChangeSet_GovernedSucceedsWithOtherUserApprove(t *testing.T) {
	ctx := context.Background()
	ws := "ws"

	tb := newSQLiteTB(t)
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("foobar", "en-US", model.TermAdmitted))))

	store := newMemStore()
	cs := &ChangeSet{ID: "cs1", WorkspaceID: ws, Name: "Ban foobar", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	appendOp(t, store, ws, cs.ID, 0, OpTermStatus, TermStatusPayload{
		ConceptID: "c1", Locale: "en-US", Text: "foobar",
		From: model.TermAdmitted, To: model.TermForbidden,
	})
	driveToApproved(t, store, ws, cs.ID)
	require.NoError(t, store.AddReview(ctx, &ChangeSetReview{
		WorkspaceID: ws, ChangesetID: cs.ID, Reviewer: "bob", Verdict: VerdictApprove,
	}))

	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	e := NewEngine(nil, tb, newFakeProfileStore(), store)
	res, err := e.MergeChangeSet(ctx, ws, store, *loaded)
	require.NoError(t, err)
	assert.Equal(t, 1, res.RevisionsCreated)

	// The governed transition was applied.
	c, _, err := tb.GetConcept(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, model.TermForbidden, c.Terms[0].Status)

	merged, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)
	assert.Equal(t, ChangeSetMerged, merged.Status)

	types := eventTypes(res)
	assert.True(t, types[EventConceptTermStatusChanged])
	assert.True(t, types[EventChangeSetMerged])
}

func TestMergeChangeSet_BaseRevConflictAbortsNoWrites(t *testing.T) {
	ctx := context.Background()
	ws := "ws"

	tb := newSQLiteTB(t)
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("widget", "en-US", model.TermApproved))))

	store := newMemStore()
	// Pretend the concept has advanced to revision 2 since the op was authored.
	require.NoError(t, store.AddRevision(ctx, &ConceptRevision{WorkspaceID: ws, ConceptID: "c1", Rev: 1, Actor: "x"}))
	require.NoError(t, store.AddRevision(ctx, &ConceptRevision{WorkspaceID: ws, ConceptID: "c1", Rev: 2, Actor: "y"}))

	cs := &ChangeSet{ID: "cs1", WorkspaceID: ws, Name: "Stale edit", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	// Authored against revision 1 (ordinary op, so the gate is not the blocker).
	appendOp(t, store, ws, cs.ID, 1, OpConceptUpdate, ConceptUpdatePayload{
		ConceptID: "c1", Definition: strPtr("new definition"),
	})

	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	e := NewEngine(nil, tb, newFakeProfileStore(), store)
	res, err := e.MergeChangeSet(ctx, ws, store, *loaded)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMergeConflict)
	require.Len(t, res.Conflicts, 1)
	assert.Equal(t, "c1", res.Conflicts[0].ConceptID)
	assert.Equal(t, int64(0), res.Conflicts[0].Seq)

	// No writes: definition untouched, no new revision, change-set still draft.
	c, _, err := tb.GetConcept(ctx, "c1")
	require.NoError(t, err)
	assert.Empty(t, c.Definition)
	rev, err := store.LatestRev(ctx, ws, "c1")
	require.NoError(t, err)
	assert.Equal(t, int64(2), rev)
	still, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)
	assert.Equal(t, ChangeSetDraft, still.Status)
}

func TestMergeChangeSet_VoiceRuleAddBumpsProfile(t *testing.T) {
	ctx := context.Background()
	ws := "ws"

	profile := &corebrand.VoiceProfile{ID: "p1", Name: "Acme", WorkspaceID: ws, Version: 3}
	profiles := newFakeProfileStore(profile)

	store := newMemStore()
	cs := &ChangeSet{ID: "cs1", WorkspaceID: ws, Name: "Forbid synergy", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	// voice.rule.add is always governed.
	appendOp(t, store, ws, cs.ID, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{
		ProfileID: "p1", List: VoiceListForbidden,
		Rule: corebrand.TermRule{Term: "synergy", Replacement: "teamwork"},
	})
	driveToApproved(t, store, ws, cs.ID)
	require.NoError(t, store.AddReview(ctx, &ChangeSetReview{
		WorkspaceID: ws, ChangesetID: cs.ID, Reviewer: "bob", Verdict: VerdictApprove,
	}))

	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	e := NewEngine(nil, termbase.NewInMemoryTermBase(), profiles, store)
	res, err := e.MergeChangeSet(ctx, ws, store, *loaded)
	require.NoError(t, err)

	assert.Equal(t, []string{"p1"}, res.ProfilesTouched)
	assert.Empty(t, res.ConceptsTouched, "a voice-only change-set touches no concepts")

	updated, err := profiles.GetProfile(ctx, "p1")
	require.NoError(t, err)
	require.Len(t, updated.Vocabulary.ForbiddenTerms, 1)
	assert.Equal(t, "synergy", updated.Vocabulary.ForbiddenTerms[0].Term)
	assert.Equal(t, "teamwork", updated.Vocabulary.ForbiddenTerms[0].Replacement)
	assert.Equal(t, 4, updated.Version, "the profile version is bumped like AD-019 promotion")
}

func TestMergeChangeSet_RemovesPilots(t *testing.T) {
	ctx := context.Background()
	ws := "ws"

	tb := newSQLiteTB(t)
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("widget", "en-US", model.TermApproved))))

	store := newMemStore()
	cs := &ChangeSet{ID: "cs1", WorkspaceID: ws, Name: "Add term", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	appendOp(t, store, ws, cs.ID, 0, OpTermAdd, TermAddPayload{
		ConceptID: "c1", Term: term("widgets", "en-GB", model.TermAdmitted),
	})

	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	e := NewEngine(nil, tb, newFakeProfileStore(), store)

	// Pilot the change-set on a stream (no voice ops → no stream binding needed).
	_, err = e.StartPilot(ctx, ws, store, *loaded, "proj1", "pilot/widgets")
	require.NoError(t, err)
	pilots, err := store.ListPilots(ctx, ws, cs.ID)
	require.NoError(t, err)
	require.Len(t, pilots, 1)
	// The shadow concept exists on the pilot stream.
	_, ok, err := tb.GetConcept(ctx, pilotConceptID(cs.ID, "pilot/widgets", "c1"))
	require.NoError(t, err)
	require.True(t, ok)

	res, err := e.MergeChangeSet(ctx, ws, store, *loaded)
	require.NoError(t, err)
	assert.Equal(t, 1, res.PilotsStopped)

	// The pilot and its shadow are gone.
	pilots, err = store.ListPilots(ctx, ws, cs.ID)
	require.NoError(t, err)
	assert.Empty(t, pilots)
	_, ok, err = tb.GetConcept(ctx, pilotConceptID(cs.ID, "pilot/widgets", "c1"))
	require.NoError(t, err)
	assert.False(t, ok)

	assert.True(t, eventTypes(res)[EventPilotStopped])
}

// TestMergeChangeSet_RelationOpBaseRevDoesNotConflict pins this regression: a
// relation op (and, by the same token, a voice op) carries no concept-revision
// pin — conceptIDOf returns "" for it — so detectConflicts must skip it even
// when it has a non-zero BaseRev and a concept in the workspace has advanced
// past that revision. The contrast sub-case shows a concept.update pinned to the
// same stale base IS flagged, proving the guard discriminates by op kind rather
// than by suppressing conflict detection wholesale.
func TestMergeChangeSet_RelationOpBaseRevDoesNotConflict(t *testing.T) {
	ctx := context.Background()
	ws := "ws"

	t.Run("relation.add with non-zero BaseRev is not flagged", func(t *testing.T) {
		tb := newSQLiteTB(t)
		require.NoError(t, tb.AddConcept(ctx, concept("c1", term("widget", "en-US", model.TermApproved))))
		require.NoError(t, tb.AddConcept(ctx, concept("c2", term("gadget", "en-US", model.TermApproved))))

		store := newMemStore()
		// c1 has advanced to revision 2 since the op was authored.
		require.NoError(t, store.AddRevision(ctx, &ConceptRevision{WorkspaceID: ws, ConceptID: "c1", Rev: 1, Actor: "x"}))
		require.NoError(t, store.AddRevision(ctx, &ConceptRevision{WorkspaceID: ws, ConceptID: "c1", Rev: 2, Actor: "y"}))

		cs := &ChangeSet{ID: "cs-rel", WorkspaceID: ws, Name: "Relate widget and gadget", CreatedBy: "alice"}
		require.NoError(t, store.CreateChangeSet(ctx, cs))
		// relation.add (RELATED is ordinary) pinned to base revision 1. It carries
		// no concept ID, so the stale base pin must not produce a conflict.
		appendOp(t, store, ws, cs.ID, 1, OpRelationAdd, RelationAddPayload{
			Relation: termbase.ConceptRelation{ID: "r1", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelRelated},
		})

		loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
		require.NoError(t, err)

		e := NewEngine(nil, tb, newFakeProfileStore(), store)
		res, err := e.MergeChangeSet(ctx, ws, store, *loaded)
		require.NoError(t, err)
		assert.Empty(t, res.Conflicts)

		// The relation was applied to the live termbase.
		rels, err := tb.RelationsOf(ctx, "c1", nil)
		require.NoError(t, err)
		require.Len(t, rels, 1)
		assert.Equal(t, "r1", rels[0].ID)

		merged, err := store.GetChangeSet(ctx, ws, cs.ID)
		require.NoError(t, err)
		assert.Equal(t, ChangeSetMerged, merged.Status)
		assert.True(t, eventTypes(res)[EventConceptRelationAdded])
	})

	t.Run("concept.update against the same stale base IS flagged", func(t *testing.T) {
		tb := newSQLiteTB(t)
		require.NoError(t, tb.AddConcept(ctx, concept("c1", term("widget", "en-US", model.TermApproved))))

		store := newMemStore()
		require.NoError(t, store.AddRevision(ctx, &ConceptRevision{WorkspaceID: ws, ConceptID: "c1", Rev: 1, Actor: "x"}))
		require.NoError(t, store.AddRevision(ctx, &ConceptRevision{WorkspaceID: ws, ConceptID: "c1", Rev: 2, Actor: "y"}))

		cs := &ChangeSet{ID: "cs-upd", WorkspaceID: ws, Name: "Stale edit", CreatedBy: "alice"}
		require.NoError(t, store.CreateChangeSet(ctx, cs))
		appendOp(t, store, ws, cs.ID, 1, OpConceptUpdate, ConceptUpdatePayload{
			ConceptID: "c1", Definition: strPtr("new definition"),
		})

		loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
		require.NoError(t, err)

		e := NewEngine(nil, tb, newFakeProfileStore(), store)
		res, err := e.MergeChangeSet(ctx, ws, store, *loaded)
		require.ErrorIs(t, err, ErrMergeConflict)
		require.Len(t, res.Conflicts, 1)
		assert.Equal(t, "c1", res.Conflicts[0].ConceptID)

		// Nothing was applied: the change-set is still a draft.
		still, err := store.GetChangeSet(ctx, ws, cs.ID)
		require.NoError(t, err)
		assert.Equal(t, ChangeSetDraft, still.Status)
	})
}

func strPtr(s string) *string { return &s }
