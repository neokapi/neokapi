//go:build integration

package knowledge

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWS = "ws-1"

func newTestStore(t *testing.T) *PostgresKnowledgeStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	store, err := NewPostgresKnowledgeStore(db)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// derefOps copies a slice of stored op pointers into values for the pure
// classification helpers.
func derefOps(ops []*ChangeSetOp) []ChangeSetOp {
	out := make([]ChangeSetOp, len(ops))
	for i, op := range ops {
		out[i] = *op
	}
	return out
}

// derefReviews copies a slice of stored review pointers into values for CanMerge.
func derefReviews(reviews []*ChangeSetReview) []ChangeSetReview {
	out := make([]ChangeSetReview, len(reviews))
	for i, r := range reviews {
		out[i] = *r
	}
	return out
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

// TestGovernedChangeSetLifecycle walks the full governed flow: create → append a
// governed op → submit → approve by a second reviewer → merge.
func TestGovernedChangeSetLifecycle(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	cs := &ChangeSet{WorkspaceID: testWS, Name: "ban legacy term", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	require.NotEmpty(t, cs.ID)
	assert.Equal(t, ChangeSetDraft, cs.Status)

	// A term.status approved → preferred is governed.
	op := &ChangeSetOp{
		WorkspaceID: testWS,
		ChangesetID: cs.ID,
		Op:          OpTermStatus,
		Payload: mustJSON(t, TermStatusPayload{
			ConceptID: "c1", Locale: "en-US", Text: "sign in",
			From: model.TermApproved, To: model.TermPreferred,
		}),
		CreatedBy: "alice",
	}
	require.NoError(t, store.AppendOp(ctx, op))
	assert.Equal(t, int64(1), op.Seq)

	ops, err := store.ListOps(ctx, testWS, cs.ID)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	governed, err := ChangeSetIsGoverned(derefOps(ops))
	require.NoError(t, err)
	assert.True(t, governed)

	// draft → in_review stamps SubmittedAt.
	require.NoError(t, store.SetChangeSetStatus(ctx, testWS, cs.ID, ChangeSetInReview))
	got, err := store.GetChangeSet(ctx, testWS, cs.ID)
	require.NoError(t, err)
	assert.Equal(t, ChangeSetInReview, got.Status)
	require.NotNil(t, got.SubmittedAt)

	// Self-approval never satisfies separation of duties.
	require.NoError(t, store.AddReview(ctx, &ChangeSetReview{
		WorkspaceID: testWS, ChangesetID: cs.ID, Reviewer: "alice", Verdict: VerdictApprove,
	}))
	require.NoError(t, store.SetChangeSetStatus(ctx, testWS, cs.ID, ChangeSetApproved))
	got, err = store.GetChangeSet(ctx, testWS, cs.ID)
	require.NoError(t, err)
	reviews, err := store.ListReviews(ctx, testWS, cs.ID)
	require.NoError(t, err)
	assert.Error(t, CanMerge(*got, true, derefReviews(reviews)), "self-approval must not satisfy SoD")

	// A second reviewer's approval clears the gate.
	require.NoError(t, store.AddReview(ctx, &ChangeSetReview{
		WorkspaceID: testWS, ChangesetID: cs.ID, Reviewer: "bob", Verdict: VerdictApprove,
	}))
	reviews, err = store.ListReviews(ctx, testWS, cs.ID)
	require.NoError(t, err)
	require.Len(t, reviews, 2)
	require.NoError(t, CanMerge(*got, true, derefReviews(reviews)))

	mergedAt := time.Now().UTC()
	require.NoError(t, store.SetMergeResult(ctx, testWS, cs.ID, "bob", mergedAt))
	got, err = store.GetChangeSet(ctx, testWS, cs.ID)
	require.NoError(t, err)
	assert.Equal(t, ChangeSetMerged, got.Status)
	assert.Equal(t, "bob", got.MergedBy)
	require.NotNil(t, got.MergedAt)
	require.NotNil(t, got.SubmittedAt)
}

// TestReviewUpsertAndReopen covers reject (verdict upsert) and the in_review →
// draft reopen edge.
func TestReviewUpsertAndReopen(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	cs := &ChangeSet{WorkspaceID: testWS, Name: "rename concept", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	require.NoError(t, store.SetChangeSetStatus(ctx, testWS, cs.ID, ChangeSetInReview))

	// A reviewer rejects, then changes their mind — AddReview upserts by reviewer.
	require.NoError(t, store.AddReview(ctx, &ChangeSetReview{
		WorkspaceID: testWS, ChangesetID: cs.ID, Reviewer: "bob", Verdict: VerdictReject, Comment: "needs work",
	}))
	require.NoError(t, store.AddReview(ctx, &ChangeSetReview{
		WorkspaceID: testWS, ChangesetID: cs.ID, Reviewer: "bob", Verdict: VerdictApprove, Comment: "ok now",
	}))
	reviews, err := store.ListReviews(ctx, testWS, cs.ID)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, VerdictApprove, reviews[0].Verdict)
	assert.Equal(t, "ok now", reviews[0].Comment)

	// Reject reopens the change-set for more work.
	require.NoError(t, store.SetChangeSetStatus(ctx, testWS, cs.ID, ChangeSetDraft))
	got, err := store.GetChangeSet(ctx, testWS, cs.ID)
	require.NoError(t, err)
	assert.Equal(t, ChangeSetDraft, got.Status)
}

// TestStatusTransitionGuards verifies the store rejects illegal status edges.
func TestStatusTransitionGuards(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	cs := &ChangeSet{WorkspaceID: testWS, Name: "guards", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))

	// SetChangeSetStatus refuses to finalize a merge — merging flows through
	// SetMergeResult, which stamps merged_by/merged_at.
	assert.Error(t, store.SetChangeSetStatus(ctx, testWS, cs.ID, ChangeSetMerged))
	// An illegal edge (draft → approved) is rejected.
	assert.Error(t, store.SetChangeSetStatus(ctx, testWS, cs.ID, ChangeSetApproved))

	// From a terminal state, SetMergeResult refuses the exit (no merge-after-end).
	require.NoError(t, store.SetChangeSetStatus(ctx, testWS, cs.ID, ChangeSetAbandoned))
	assert.Error(t, store.SetMergeResult(ctx, testWS, cs.ID, "bob", time.Now()))
}

// TestOrdinaryDraftMerge exercises the ordinary fast-path: an ungoverned
// change-set clears CanMerge from draft and the store merges it straight from
// draft via SetMergeResult, with no review step. A second merge is rejected
// because merged is terminal (no double-merge).
func TestOrdinaryDraftMerge(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	cs := &ChangeSet{WorkspaceID: testWS, Name: "fast path", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	require.Equal(t, ChangeSetDraft, cs.Status)

	// The merging service gates first, then merges. An ops-less (ungoverned)
	// draft passes the gate.
	require.NoError(t, CanMerge(*cs, false, nil))

	mergedAt := time.Now().UTC()
	require.NoError(t, store.SetMergeResult(ctx, testWS, cs.ID, "alice", mergedAt))

	got, err := store.GetChangeSet(ctx, testWS, cs.ID)
	require.NoError(t, err)
	assert.Equal(t, ChangeSetMerged, got.Status)
	assert.Equal(t, "alice", got.MergedBy)
	require.NotNil(t, got.MergedAt)

	// merged is terminal: a second merge is rejected.
	assert.Error(t, store.SetMergeResult(ctx, testWS, cs.ID, "alice", time.Now()))
}

// TestOrdinaryChangeSetOps confirms an all-ordinary change-set is classified as
// ungoverned and lists ops in seq order.
func TestOrdinaryChangeSetOps(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	cs := &ChangeSet{WorkspaceID: testWS, Name: "add terms", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))

	for i, text := range []string{"alpha", "beta", "gamma"} {
		op := &ChangeSetOp{
			WorkspaceID: testWS,
			ChangesetID: cs.ID,
			Op:          OpTermAdd,
			Payload: mustJSON(t, TermAddPayload{
				ConceptID: "c1",
				Term:      termbase.Term{Text: text, Locale: "en-US", Status: model.TermApproved},
			}),
			CreatedBy: "alice",
		}
		require.NoError(t, store.AppendOp(ctx, op))
		assert.Equal(t, int64(i+1), op.Seq)
	}

	ops, err := store.ListOps(ctx, testWS, cs.ID)
	require.NoError(t, err)
	require.Len(t, ops, 3)
	governed, err := ChangeSetIsGoverned(derefOps(ops))
	require.NoError(t, err)
	assert.False(t, governed)
	require.NoError(t, CanMerge(*cs, false, nil))

	// RemoveOp drops a single op; remaining seqs are untouched.
	require.NoError(t, store.RemoveOp(ctx, testWS, cs.ID, 2))
	ops, err = store.ListOps(ctx, testWS, cs.ID)
	require.NoError(t, err)
	require.Len(t, ops, 2)
	assert.Equal(t, int64(1), ops[0].Seq)
	assert.Equal(t, int64(3), ops[1].Seq)
}

// TestRevisionsMonotonic checks auto-assigned and caller-supplied revisions.
func TestRevisionsMonotonic(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	latest, err := store.LatestRev(ctx, testWS, "c1")
	require.NoError(t, err)
	assert.Equal(t, int64(0), latest)

	r1 := &ConceptRevision{WorkspaceID: testWS, ConceptID: "c1", Actor: "alice", Snapshot: json.RawMessage(`{"id":"c1"}`)}
	require.NoError(t, store.AddRevision(ctx, r1))
	assert.Equal(t, int64(1), r1.Rev)

	r2 := &ConceptRevision{WorkspaceID: testWS, ConceptID: "c1", Actor: "bob"}
	require.NoError(t, store.AddRevision(ctx, r2))
	assert.Equal(t, int64(2), r2.Rev)

	// Caller-supplied rev (the documented LatestRev+1 path) is honored.
	r3 := &ConceptRevision{WorkspaceID: testWS, ConceptID: "c1", Rev: 3, Actor: "carol", ChangesetID: "cs-1"}
	require.NoError(t, store.AddRevision(ctx, r3))

	latest, err = store.LatestRev(ctx, testWS, "c1")
	require.NoError(t, err)
	assert.Equal(t, int64(3), latest)

	revs, err := store.ListRevisions(ctx, testWS, "c1")
	require.NoError(t, err)
	require.Len(t, revs, 3)
	assert.Equal(t, int64(1), revs[0].Rev)
	assert.Equal(t, int64(3), revs[2].Rev)
	assert.JSONEq(t, `{"id":"c1"}`, string(revs[0].Snapshot))
}

// TestMarketsLocalesRoundtrip covers market CRUD and the JSONB locales array.
func TestMarketsLocalesRoundtrip(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	m := &Market{WorkspaceID: testWS, Name: "dach", Locales: []model.LocaleID{"de-DE", "de-AT", "de-CH"}}
	require.NoError(t, store.CreateMarket(ctx, m))
	require.NotEmpty(t, m.ID)

	got, err := store.GetMarket(ctx, testWS, m.ID)
	require.NoError(t, err)
	assert.Equal(t, []model.LocaleID{"de-DE", "de-AT", "de-CH"}, got.Locales)

	m.Name = "dach-region"
	m.Locales = []model.LocaleID{"de-DE"}
	require.NoError(t, store.UpdateMarket(ctx, m))
	got, err = store.GetMarket(ctx, testWS, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "dach-region", got.Name)
	require.Len(t, got.Locales, 1)

	list, err := store.ListMarkets(ctx, testWS)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, store.DeleteMarket(ctx, testWS, m.ID))
	_, err = store.GetMarket(ctx, testWS, m.ID)
	assert.Error(t, err)
}

// TestObservations covers observation insert/list/delete.
func TestObservations(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	o := &Observation{
		WorkspaceID: testWS, ConceptID: "c1", Kind: ObservationCompetitor,
		Quote: "Sign on", Source: "acme.com", URL: "https://acme.com", CreatedBy: "alice",
	}
	require.NoError(t, store.AddObservation(ctx, o))
	require.NotEmpty(t, o.ID)

	list, err := store.ListObservationsByConcept(ctx, testWS, "c1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, ObservationCompetitor, list[0].Kind)
	assert.Equal(t, "acme.com", list[0].Source)

	require.NoError(t, store.DeleteObservation(ctx, testWS, o.ID))
	list, err = store.ListObservationsByConcept(ctx, testWS, "c1")
	require.NoError(t, err)
	assert.Empty(t, list)
}

// TestThreadedComments covers concept and change-set comment threads plus resolve.
func TestThreadedComments(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	parent := &Comment{WorkspaceID: testWS, ConceptID: "c1", Body: "top-level", Author: "alice"}
	require.NoError(t, store.AddComment(ctx, parent))
	reply := &Comment{WorkspaceID: testWS, ConceptID: "c1", ParentID: parent.ID, Body: "a reply", Author: "bob"}
	require.NoError(t, store.AddComment(ctx, reply))
	onCS := &Comment{WorkspaceID: testWS, ConceptID: "c1", ChangesetID: "cs-1", Body: "review note", Author: "alice"}
	require.NoError(t, store.AddComment(ctx, onCS))

	byConcept, err := store.ListCommentsByConcept(ctx, testWS, "c1")
	require.NoError(t, err)
	require.Len(t, byConcept, 3)
	assert.Equal(t, parent.ID, byConcept[0].ID)
	assert.Equal(t, parent.ID, byConcept[1].ParentID)

	byCS, err := store.ListCommentsByChangeset(ctx, testWS, "cs-1")
	require.NoError(t, err)
	require.Len(t, byCS, 1)
	assert.Equal(t, onCS.ID, byCS[0].ID)

	require.NoError(t, store.ResolveComment(ctx, testWS, parent.ID, true))
	byConcept, err = store.ListCommentsByConcept(ctx, testWS, "c1")
	require.NoError(t, err)
	for _, c := range byConcept {
		if c.ID == parent.ID {
			assert.True(t, c.Resolved)
		}
	}

	require.NoError(t, store.DeleteComment(ctx, testWS, reply.ID))
	byConcept, err = store.ListCommentsByConcept(ctx, testWS, "c1")
	require.NoError(t, err)
	assert.Len(t, byConcept, 2)
}

// TestPilots covers pilot binding idempotency, listing, and stream lookup.
func TestPilots(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	cs := &ChangeSet{WorkspaceID: testWS, Name: "pilot me", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))

	p := &Pilot{WorkspaceID: testWS, ChangesetID: cs.ID, ProjectID: "p1", Stream: "main", CreatedBy: "alice"}
	require.NoError(t, store.AddPilot(ctx, p))
	require.NoError(t, store.AddPilot(ctx, p)) // idempotent re-bind

	list, err := store.ListPilots(ctx, testWS, cs.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)

	byStream, err := store.ListPilotsForStream(ctx, testWS, "p1", "main")
	require.NoError(t, err)
	require.Len(t, byStream, 1)
	assert.Equal(t, cs.ID, byStream[0].ChangesetID)

	require.NoError(t, store.RemovePilot(ctx, testWS, cs.ID, "p1", "main"))
	list, err = store.ListPilots(ctx, testWS, cs.ID)
	require.NoError(t, err)
	assert.Empty(t, list)
}

// TestListChangeSetsStatusFilter checks the status filter and newest-first order.
func TestListChangeSetsStatusFilter(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	for _, name := range []string{"one", "two", "three"} {
		require.NoError(t, store.CreateChangeSet(ctx, &ChangeSet{WorkspaceID: testWS, Name: name, CreatedBy: "alice"}))
	}
	// Move one into review.
	all, err := store.ListChangeSets(ctx, testWS, "")
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.NoError(t, store.SetChangeSetStatus(ctx, testWS, all[0].ID, ChangeSetInReview))

	drafts, err := store.ListChangeSets(ctx, testWS, ChangeSetDraft)
	require.NoError(t, err)
	assert.Len(t, drafts, 2)

	inReview, err := store.ListChangeSets(ctx, testWS, ChangeSetInReview)
	require.NoError(t, err)
	require.Len(t, inReview, 1)
	assert.Equal(t, ChangeSetInReview, inReview[0].Status)
}
