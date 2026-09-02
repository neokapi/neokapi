package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the governance around a review decision: who may approve,
// whether they may approve their own writing, and what each decision leaves
// behind in the decision ledger and the audit log. They run against the real
// PostgreSQL content store and auth store, because the authorship separation of
// duties reads lives in block_history and the policy lives in the auth store.

const (
	testTranslator = platauth.PermViewContent | platauth.PermTranslate
	testReviewer   = platauth.PermViewContent | platauth.PermTranslate | platauth.PermReview
)

// attributeTarget rewrites a block's target as author, so the content store
// files an attributed content change. That row is what separation of duties
// reads: it names who wrote the wording now up for approval. An empty author
// leaves the target machine-authored.
func attributeTarget(t *testing.T, s *Server, projID, blockID, locale, author, text string) {
	t.Helper()
	ctx := bstore.WithChangeContext(context.Background(), bstore.ChangeContext{Actor: author})
	sb, err := s.ContentStore.GetBlock(ctx, projID, "main", blockID)
	require.NoError(t, err)
	loc := model.LocaleID(locale)
	sb.Block.SetTargetText(loc, text)
	sb.Block.Target(loc).Status = model.TargetStatusDraft
	require.NoError(t, s.ContentStore.StoreBlocks(ctx, projID, "main", []*model.Block{sb.Block}))
}

// callReviewBlockGoverned invokes HandleReviewBlock the way the router would,
// with a workspace and an acting user on the context so the governance gates
// have something to judge.
func callReviewBlockGoverned(t *testing.T, s *Server, wsID, projID, blockID, body string, perms platauth.Permission, userID string) *httptest.ResponseRecorder {
	t.Helper()
	e := s.GetEcho()
	r := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref", "bid")
	c.SetParamValues("rc", projID, "main", blockID)
	c.Set("workspace_id", wsID)
	c.Set("user_id", userID)
	c.Set("project_permissions", perms)
	_ = s.HandleReviewBlock(c)
	return rec
}

// approveBody is the request body for approving one locale.
func approveBody(locale string) string {
	return fmt.Sprintf(`{"target_locale":%q,"reviewed":true,"item_name":"greetings.txt"}`, locale)
}

// targetStatus reads a block's stored rung for a locale.
func targetStatus(t *testing.T, s *Server, projID, blockID, locale string) model.TargetStatus {
	t.Helper()
	sb, err := s.ContentStore.GetBlock(context.Background(), projID, "main", blockID)
	require.NoError(t, err)
	target := sb.Block.Target(model.LocaleID(locale))
	require.NotNil(t, target)
	return target.Status
}

// TestReviewApproveNeedsReviewPermission: approving is PermReview for the
// language. Withdrawing an approval stays with PermTranslate, so a translator
// can still take back their own work.
func TestReviewApproveNeedsReviewPermission(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
	})
	bid := ids["Hello"]

	rec := callReviewBlockGoverned(t, s, wsID, projID, bid, approveBody("fr"), testTranslator, "u-translator")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusDraft, targetStatus(t, s, projID, bid, "fr"),
		"a refused approval must leave the rung alone")

	rec = callReviewBlockGoverned(t, s, wsID, projID, bid, approveBody("fr"), testReviewer, "u-reviewer")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, projID, bid, "fr"))

	// Withdrawing that approval needs only the translate permission.
	rec = callReviewBlockGoverned(t, s, wsID, projID, bid,
		`{"target_locale":"fr","reviewed":false,"item_name":"greetings.txt"}`, testTranslator, "u-translator")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, projID, bid, "fr"))
}

// TestReviewApproveSoDBlocksOwnWork: with the workspace policy set to block, a
// reviewer approving the translation they wrote is refused with the
// separation-of-duties sentence, the rung is untouched, and the violation is
// recorded.
func TestReviewApproveSoDBlocksOwnWork(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	require.NoError(t, s.AuthStore.SetSoDMode(context.Background(), wsID, platauth.SoDBlock))

	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
	})
	bid := ids["Hello"]
	attributeTarget(t, s, projID, bid, "fr", "u-reviewer", "Bonjour à tous")

	// The premise: the store attributes this target to the reviewer.
	authors, err := s.ContentStore.(platstore.TargetAuthorStore).
		LastTargetAuthors(context.Background(), projID, "main", []string{bid}, []string{"fr"})
	require.NoError(t, err)
	require.Equal(t, "u-reviewer", authors[platstore.TargetRef{BlockID: bid, Locale: "fr"}])

	snapshot, stop := collectEvents(t, s)
	defer stop()

	rec := callReviewBlockGoverned(t, s, wsID, projID, bid, approveBody("fr"), testReviewer, "u-reviewer")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), sodRefusal)
	assert.Equal(t, model.TargetStatusDraft, targetStatus(t, s, projID, bid, "fr"),
		"a blocked self-approval must leave the rung alone")

	require.Eventually(t, func() bool {
		_, ok := findEvent(snapshot(), platev.EventType("sod.violation"))
		return ok
	}, 2*time.Second, 20*time.Millisecond, "the refusal is recorded as a violation")

	// Someone else may approve the same target.
	rec = callReviewBlockGoverned(t, s, wsID, projID, bid, approveBody("fr"), testReviewer, "u-other")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, projID, bid, "fr"))
}

// TestReviewApproveSoDWarnAllowsOwnWork: under warn the same approval goes
// through, and the violation is still recorded.
func TestReviewApproveSoDWarnAllowsOwnWork(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	require.NoError(t, s.AuthStore.SetSoDMode(context.Background(), wsID, platauth.SoDWarn))

	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
	})
	bid := ids["Hello"]
	attributeTarget(t, s, projID, bid, "fr", "u-reviewer", "Bonjour à tous")

	snapshot, stop := collectEvents(t, s)
	defer stop()

	rec := callReviewBlockGoverned(t, s, wsID, projID, bid, approveBody("fr"), testReviewer, "u-reviewer")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, projID, bid, "fr"))

	require.Eventually(t, func() bool {
		ev, ok := findEvent(snapshot(), platev.EventType("sod.violation"))
		return ok && ev.Data["mode"] == string(platauth.SoDWarn)
	}, 2*time.Second, 20*time.Millisecond, "warn records the violation and allows the approval")
}

// TestReviewApproveMachineAuthoredPassesSoD: a draft a run produced carries no
// human author, so the one person in a small workspace approves it even under a
// blocking policy.
func TestReviewApproveMachineAuthoredPassesSoD(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	require.NoError(t, s.AuthStore.SetSoDMode(context.Background(), wsID, platauth.SoDBlock))

	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
	})
	bid := ids["Hello"]
	attributeTarget(t, s, projID, bid, "fr", "", "Bonjour à tous") // no acting user

	rec := callReviewBlockGoverned(t, s, wsID, projID, bid, approveBody("fr"), testReviewer, "u-reviewer")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, projID, bid, "fr"))
}

// TestReviewDecisionIsAudited: an approval and a rejection each leave a
// review.decided record naming the block, the locale and the rungs moved
// between.
func TestReviewDecisionIsAudited(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
	})
	bid := ids["Hello"]

	snapshot, stop := collectEvents(t, s)
	defer stop()

	rec := callReviewBlockGoverned(t, s, wsID, projID, bid, approveBody("fr"), testReviewer, "u-reviewer")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Eventually(t, func() bool {
		ev, ok := findEvent(snapshot(), platev.EventReviewDecided)
		return ok && ev.Data["decision"] == "approved"
	}, 2*time.Second, 20*time.Millisecond)

	ev, _ := findEvent(snapshot(), platev.EventReviewDecided)
	assert.Equal(t, "u-reviewer", ev.Actor)
	assert.Equal(t, projID, ev.ProjectID)
	assert.Equal(t, "block", ev.ResourceType)
	assert.Equal(t, bid, ev.ResourceID)
	assert.Equal(t, "fr", ev.Data["locale"])
	assert.Equal(t, "main", ev.Data["stream"])
	assert.Equal(t, string(model.TargetStatusDraft), ev.Before["status"])
	assert.Equal(t, string(model.TargetStatusReviewed), ev.After["status"])

	// A rejection is the same record with the other verdict.
	rec = callReviewBlockGoverned(t, s, wsID, projID, bid,
		`{"target_locale":"fr","reviewed":false,"status":"draft","item_name":"greetings.txt"}`,
		testReviewer, "u-reviewer")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Eventually(t, func() bool {
		for _, e := range snapshot() {
			if e.Type == platev.EventReviewDecided && e.Data["decision"] == "rejected" &&
				e.After["status"] == string(model.TargetStatusDraft) {
				return true
			}
		}
		return false
	}, 2*time.Second, 20*time.Millisecond)
}

// callBulkReviewGoverned invokes HandleBulkReviewBlocks the way the router
// would.
func callBulkReviewGoverned(t *testing.T, s *Server, wsID, projID, body string, perms platauth.Permission, userID string) *httptest.ResponseRecorder {
	t.Helper()
	e := s.GetEcho()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("rc", projID, "main")
	c.Set("workspace_id", wsID)
	c.Set("user_id", userID)
	c.Set("project_permissions", perms)
	_ = s.HandleBulkReviewBlocks(c)
	return rec
}

// TestBulkReviewApproveGates: the bulk route gates approval the way the single
// route does, and a block the caller wrote is refused against that block rather
// than failing the batch.
func TestBulkReviewApproveGates(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	require.NoError(t, s.AuthStore.SetSoDMode(context.Background(), wsID, platauth.SoDBlock))

	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
		pendingFrBlock("b2", "Goodbye", "Au revoir"),
	})
	mine, theirs := ids["Hello"], ids["Goodbye"]
	attributeTarget(t, s, projID, mine, "fr", "u-reviewer", "Bonjour à tous")
	attributeTarget(t, s, projID, theirs, "fr", "u-translator", "Au revoir à tous")

	body := fmt.Sprintf(`{"block_ids":[%q,%q],"target_locale":"fr","approve":true}`, mine, theirs)

	rec := callBulkReviewGoverned(t, s, wsID, projID, body, testTranslator, "u-translator")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())

	rec = callBulkReviewGoverned(t, s, wsID, projID, body, testReviewer, "u-reviewer")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	res := decodeJSON[BulkReviewResponse](t, rec)
	assert.Equal(t, 1, res.Succeeded)
	assert.Equal(t, 1, res.Failed)
	for _, r := range res.Results {
		if r.BlockID == mine {
			assert.False(t, r.OK)
			assert.Contains(t, r.Error, sodRefusal)
		} else {
			assert.True(t, r.OK, r.Error)
		}
	}
	assert.Equal(t, model.TargetStatusDraft, targetStatus(t, s, projID, mine, "fr"))
	assert.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, projID, theirs, "fr"))
}

// TestApprovePassingRecordsDecisionsAndAudits: a bulk pass files every approval
// in the decision ledger with the decider and the time, and records the pass in
// the audit log.
func TestApprovePassingRecordsDecisionsAndAudits(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	projID, _ := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
		pendingFrBlock("b2", "Goodbye", "Au revoir"),
	})

	snapshot, stop := collectEvents(t, s)
	defer stop()

	rec, res := callApprovePassing(t, s, wsID, projID, ownerID, `{}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 2, res.Approved)
	assert.Equal(t, 0, res.SkippedSelfAuthored)

	decisions, err := s.ContentStore.(platstore.DecisionStore).
		ListUnitDecisions(context.Background(), projID, "main")
	require.NoError(t, err)
	require.Len(t, decisions, 2, "every bulk approval reaches the ledger")
	for _, d := range decisions {
		assert.Equal(t, "approved", d.ReviewState)
		assert.Equal(t, "fr", d.Variant)
		assert.Equal(t, "owner@rc.test", d.DecidedBy, "the ledger names the decider")
		assert.NotEmpty(t, d.DecidedAt)
		assert.NotEmpty(t, d.TargetHash, "a decision is bound to the wording it blesses")
	}

	require.Eventually(t, func() bool {
		ev, ok := findEvent(snapshot(), platev.EventReviewBulkApproved)
		return ok && ev.Data["locale"] == "fr" && ev.Data["approved"] == "2"
	}, 2*time.Second, 20*time.Millisecond, "the pass is recorded in the audit log")
}

// TestApprovePassingSkipsSelfAuthored: the bulk pass respects the same policy
// the per-block route does, and names the targets it left pending for it.
func TestApprovePassingSkipsSelfAuthored(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	require.NoError(t, s.AuthStore.SetSoDMode(context.Background(), wsID, platauth.SoDBlock))

	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
		pendingFrBlock("b2", "Goodbye", "Au revoir"),
	})
	mine, theirs := ids["Hello"], ids["Goodbye"]
	attributeTarget(t, s, projID, mine, "fr", ownerID, "Bonjour à tous")
	attributeTarget(t, s, projID, theirs, "fr", "u-translator", "Au revoir à tous")

	rec, res := callApprovePassing(t, s, wsID, projID, ownerID, `{}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, res.Approved)
	assert.Equal(t, 1, res.Skipped)
	assert.Equal(t, 1, res.SkippedSelfAuthored)
	assert.Equal(t, res.Skipped,
		res.SkippedFailingChecks+res.SkippedTermViolations+res.SkippedBelowVoiceBar+res.SkippedSelfAuthored,
		"every skip is attributed to exactly one bar")
	assert.Equal(t, model.TargetStatusDraft, targetStatus(t, s, projID, mine, "fr"))
	assert.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, projID, theirs, "fr"))
}

// TestApprovePassingRecordsOneSoDViolationForThePass: a pass over a corpus the
// caller wrote files one violation carrying the count, not one per block. The
// event bus drops what it cannot keep up with, so a record per block would push
// out every other subscriber's events and still not be a complete trail.
func TestApprovePassingRecordsOneSoDViolationForThePass(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	require.NoError(t, s.AuthStore.SetSoDMode(context.Background(), wsID, platauth.SoDWarn))

	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
		pendingFrBlock("b2", "Goodbye", "Au revoir"),
	})
	attributeTarget(t, s, projID, ids["Hello"], "fr", ownerID, "Bonjour à tous")
	attributeTarget(t, s, projID, ids["Goodbye"], "fr", ownerID, "Au revoir à tous")

	snapshot, stop := collectEvents(t, s)
	defer stop()

	rec, res := callApprovePassing(t, s, wsID, projID, ownerID, `{}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 2, res.Approved, "warn allows the approvals")
	assert.Equal(t, 0, res.SkippedSelfAuthored)

	require.Eventually(t, func() bool {
		ev, ok := findEvent(snapshot(), platev.EventType("sod.violation"))
		return ok && ev.Data["targets"] == "2"
	}, 2*time.Second, 20*time.Millisecond)

	violations := 0
	for _, ev := range snapshot() {
		if ev.Type == platev.EventType("sod.violation") {
			violations++
		}
	}
	assert.Equal(t, 1, violations, "one record for the pass, not one per block")
}
