package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// seedDraftedProject seeds one block whose French translation an AI drafted,
// as a convergence run leaves it: status draft, waiting for review.
func seedDraftedProject(t *testing.T, cs *bstore.PostgresStore) (pid, bid string) {
	t.Helper()
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Save changes")
	b.SetTargetText("fr", "Enregistrer les modifications")
	b.StampTargetProvenance("fr", model.TargetStatusDraft, model.Origin{Kind: model.OriginAI})
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	return pid, ids["Save changes"]
}

// ledgerRow is the decision the ledger holds for a block's French translation.
func ledgerRow(t *testing.T, cs *bstore.PostgresStore, pid, bid string) *venue.UnitDecision {
	t.Helper()
	sb, err := cs.GetBlock(t.Context(), pid, "main", bid)
	require.NoError(t, err)
	d, err := cs.GetUnitDecision(t.Context(), pid, "main", sb.ItemName, sb.SourceID, "fr")
	require.NoError(t, err)
	return d
}

// rejectWith rejects the block's French translation with reasons and a note.
func rejectWith(t *testing.T, srv *Server, cs *bstore.PostgresStore, pid, bid string, who changeCaller, note string, reasons ...string) change.Result {
	t.Helper()
	op := decide(at("greetings.txt", bid, "fr"), targetRev(t, cs, pid, bid, "fr"), change.OutcomeReject)
	op.Body.(*change.Decide).Reasons = reasons
	rec, res := sendChanges(t, srv, pid, who, change.Set{Ops: []change.Op{op}, Note: note})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return res
}

var secondReviewer = changeCaller{user: "user-3", name: "Grace", perms: fullCaller.perms}

// A rejection of a drafted translation, the status an AI or MT draft holds,
// moves no rung. It is recorded all the same, with the person, the reasons
// and the note; the same person's same rejection again is unchanged, and a
// rejection with other reasons, or by someone else, is recorded anew.
func TestReviewDecision_ARejectionOfADraftIsRecordedWithItsReasons(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	pid, bid := seedDraftedProject(t, cs)

	res := rejectWith(t, srv, cs, pid, bid, fullCaller, "Says the opposite.", "meaning")
	assert.Equal(t, change.OpApplied, res.Ops[0].Status, "a rejection that moves no rung is still a decision")
	row := ledgerRow(t, cs, pid, bid)
	require.NotNil(t, row)
	assert.Equal(t, venue.ReviewStateRejected, row.ReviewState)
	assert.Equal(t, []string{"meaning"}, row.Reasons)
	assert.Equal(t, "Says the opposite.", row.Note)
	assert.NotEmpty(t, row.DecidedBy)
	assert.Equal(t, model.TargetStatusDraft, targetStatusOf(t, getStoredBlock(t, cs, pid, bid), "fr"))

	res = rejectWith(t, srv, cs, pid, bid, fullCaller, "Says the opposite.", "meaning")
	assert.Equal(t, change.OpUnchanged, res.Ops[0].Status, "the same person's same rejection already stands")

	res = rejectWith(t, srv, cs, pid, bid, fullCaller, "Wrong term and tone.", "wrong_term", "tone")
	assert.Equal(t, change.OpApplied, res.Ops[0].Status, "other reasons are a decision of their own")
	row = ledgerRow(t, cs, pid, bid)
	assert.Equal(t, []string{"wrong_term", "tone"}, row.Reasons)
	assert.Equal(t, "Wrong term and tone.", row.Note)

	first := row.DecidedBy
	res = rejectWith(t, srv, cs, pid, bid, secondReviewer, "Wrong term and tone.", "wrong_term", "tone")
	assert.Equal(t, change.OpApplied, res.Ops[0].Status, "the same rejection by someone else is recorded")
	assert.NotEqual(t, first, ledgerRow(t, cs, pid, bid).DecidedBy)
}

// A rejection that moves a translated target to draft keeps its reasons and
// note in the ledger too.
func TestReviewDecision_ARejectionThatMovesTheRungKeepsItsReasons(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	res := rejectWith(t, srv, cs, pid, bid, fullCaller, "Too formal.", "tone")
	assert.Equal(t, change.OpApplied, res.Ops[0].Status)
	row := ledgerRow(t, cs, pid, bid)
	require.NotNil(t, row)
	assert.Equal(t, []string{"tone"}, row.Reasons)
	assert.Equal(t, "Too formal.", row.Note)
}

// An approval keeps the note written with it.
func TestReviewDecision_AnApprovalKeepsItsNote(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	pid, bid := seedDraftedProject(t, cs)
	op := decide(at("greetings.txt", bid, "fr"), targetRev(t, cs, pid, bid, "fr"), change.OutcomeEstablish)
	rec, res := sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{op}, Note: "Checked against the style guide."})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, change.OpApplied, res.Ops[0].Status)
	row := ledgerRow(t, cs, pid, bid)
	require.NotNil(t, row)
	assert.Equal(t, venue.ReviewStateApproved, row.ReviewState)
	assert.Equal(t, "Checked against the style guide.", row.Note)
}

// A correction approved in the same set keeps its reasons and note with the
// approval.
func TestReviewDecision_ACorrectionKeepsItsReasons(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	pid, bid := seedDraftedProject(t, cs)
	rev := targetRev(t, cs, pid, bid, "fr")
	text := "Enregistrer"
	set := change.Set{
		Ops: []change.Op{
			{Kind: change.KindSetContent, At: at("greetings.txt", bid, "fr"), IfMatch: rev,
				Body: &change.SetContent{Text: &text}},
			{Kind: change.KindDecide, At: at("greetings.txt", bid, "fr"), IfMatch: rev,
				Body: &change.Decide{Outcome: change.OutcomeEstablish, Reasons: []string{"wrong_term"}}},
		},
		Note: "The product calls it Save.",
	}
	rec, res := sendChanges(t, srv, pid, fullCaller, set)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, res.Ops, 2)
	assert.Equal(t, change.OpApplied, res.Ops[1].Status)
	row := ledgerRow(t, cs, pid, bid)
	require.NotNil(t, row)
	assert.Equal(t, venue.ReviewStateApproved, row.ReviewState)
	assert.Equal(t, []string{"wrong_term"}, row.Reasons)
	assert.Equal(t, "The product calls it Save.", row.Note)
}
