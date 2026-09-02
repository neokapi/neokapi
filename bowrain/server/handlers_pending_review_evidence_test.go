package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// A review queue is only honest about what will pass if it carries the evidence
// the server actually judges on. Approve-passing applies three bars — QA checks,
// terminology, and the voice bar — but the queue payload carried only the
// blocks, so a surface could bucket on check findings alone and call "passing"
// a set the server then refused. #1771 removed the dead `compliant` bucket rather
// than guess; these cases pin the evidence that replaces the guess.

// pendingFrBlock builds a translatable block whose fr target is a pending
// draft — a candidate for both the queue and the bulk approve-passing pass.
func pendingFrBlock(id, source, frTarget string) *model.Block {
	b := &model.Block{ID: id, Translatable: true}
	b.SetSourceText(source)
	b.SetTargetText("fr", frTarget)
	b.Target("fr").Status = model.TargetStatusDraft
	return b
}

// listPendingReview calls the queue handler as the router would, in a workspace.
func listPendingReview(t *testing.T, s *Server, wsID, projID string) pendingReviewResponse {
	t.Helper()
	e := s.GetEcho()
	r := httptest.NewRequest(http.MethodGet, "/?locales=fr", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("rc", projID, "main")
	c.Set("workspace_id", wsID)
	c.Set("project_permissions", platauth.PermAll)
	require.NoError(t, s.HandleListPendingReview(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeJSON[pendingReviewResponse](t, rec)
}

// entryFor finds the queue entry for a block id.
func entryFor(t *testing.T, page pendingReviewResponse, blockID string) pendingReviewEntry {
	t.Helper()
	for _, e := range page.Entries {
		if e.BlockID == blockID {
			return e
		}
	}
	t.Fatalf("no queue entry for block %s", blockID)
	return pendingReviewEntry{}
}

// TestPendingReview_EntriesCarryTermAndVoiceEvidence walks one project whose
// pending fr drafts differ in exactly one bar each, and asserts the queue names
// which. The term verdict has three rungs, not two: an unchecked target is not
// a compliant one.
func TestPendingReview_EntriesCarryTermAndVoiceEvidence(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	bad := pendingFrBlock("bad", "Use the app", "Il faut utiliser l'application") // forbidden "utiliser"
	miss := pendingFrBlock("miss", "Open the app", "Ouvrir le truc")              // missing mandated "application"
	low := pendingFrBlock("low", "Save the app", "Sauver l'application")          // clean terms, low voice score
	ok := pendingFrBlock("ok", "Close the app", "Fermer l'application")           // clears every bar
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{bad, miss, low, ok})

	tb, err := s.wsStores.getTerms("rc")
	require.NoError(t, err)
	seedTermUnificationConcepts(t, tb)

	// A scoring profile with a raised bar, and two scores against it: one below
	// the bar, one above. Only scored blocks carry a voice verdict.
	profile := &coreprofile.VoiceProfile{ID: "p-queue", Scope: wsID, Name: "Queue Voice", MinScore: 90}
	require.NoError(t, s.VoiceStore.CreateProfile(ctx, profile))
	lowID, okID := ids["Save the app"], ids["Close the app"]
	badID, missID := ids["Use the app"], ids["Open the app"]
	score := func(blockID string, value int) {
		require.NoError(t, s.VoiceStore.StoreScore(ctx, &coreprofile.StoredScore{
			ProjectID: projID, Stream: "main", BlockID: blockID, ProfileID: profile.ID,
			Locale: "fr", Score: value, CheckedAt: time.Now().UTC(),
		}))
	}
	score(lowID, 62)
	score(okID, 97)

	page := listPendingReview(t, s, wsID, projID)
	require.Len(t, page.Entries, 4)

	t.Run("terminology", func(t *testing.T) {
		assert.Equal(t, platstore.TermComplianceViolation, entryFor(t, page, badID).TermCompliance,
			"a forbidden term in the target is a violation")
		assert.Equal(t, platstore.TermComplianceViolation, entryFor(t, page, missID).TermCompliance,
			"a missing mandated rendering is a violation")
		assert.Equal(t, platstore.TermComplianceCompliant, entryFor(t, page, okID).TermCompliance,
			"a checked, clean target is compliant")
	})

	t.Run("voice", func(t *testing.T) {
		lowEntry := entryFor(t, page, lowID)
		require.NotNil(t, lowEntry.VoiceScore)
		require.NotNil(t, lowEntry.VoiceBar)
		assert.Equal(t, 62, *lowEntry.VoiceScore)
		assert.Equal(t, 90, *lowEntry.VoiceBar, "the bar is the scoring profile's, not the default")

		okEntry := entryFor(t, page, okID)
		require.NotNil(t, okEntry.VoiceScore)
		assert.Equal(t, 97, *okEntry.VoiceScore)

		unscored := entryFor(t, page, badID)
		assert.Nil(t, unscored.VoiceScore, "an unscored block carries no voice verdict")
		assert.Nil(t, unscored.VoiceBar, "and no bar, because none was applied to it")
	})

	// The point of carrying the evidence: a surface deriving a verdict from it
	// answers exactly what the bulk pass will do. Anything else and the queue's
	// "passing" bucket is a guess again.
	t.Run("the evidence decides the same blocks the bulk predicate does", func(t *testing.T) {
		proj, err := s.ContentStore.GetProject(ctx, projID)
		require.NoError(t, err)
		gate := s.resolveTermGate(ctx, proj, "main", wsID)
		scores := latestVoiceScores(ctx, s.VoiceStore, projID, "main")["fr"]

		for _, e := range page.Entries {
			block := storedBlockByID(t, s, projID, e.BlockID)
			fromEvidence := e.TermCompliance != platstore.TermComplianceViolation &&
				(e.VoiceScore == nil || *e.VoiceScore >= *e.VoiceBar)
			fromServer := blockCompliantAndPassing(ctx, block, "fr", scores, gate)
			assert.Equal(t, fromServer, fromEvidence,
				"block %s: the queue's evidence and the server's predicate disagree", e.BlockID)
		}
	})
}

// TestPendingReview_UncheckedIsNotCompliant pins the third rung. With no terms
// store and no brand vocabulary there is nothing to be compliant with, and a
// queue that reported "compliant" would be claiming evidence it never had.
func TestPendingReview_UncheckedIsNotCompliant(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)

	// No concepts seeded: the gate has nothing to enforce for fr.
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("plain", "Use the app", "Il faut utiliser l'application"),
	})

	page := listPendingReview(t, s, wsID, projID)
	require.Len(t, page.Entries, 1)
	entry := entryFor(t, page, ids["Use the app"])
	assert.Equal(t, platstore.TermComplianceUnchecked, entry.TermCompliance,
		"with no governance active the verdict is unchecked, not compliant")
	assert.Nil(t, entry.VoiceScore, "and an unscored block carries no voice verdict")
}

// TestApprovePassing_SkipsAreNamedByTheBarTheyMissed pins the bulk path's half
// of the contract: the response reports WHICH bar each excluded target missed,
// in the same three-way vocabulary the queue entries carry, so the preview and
// the outcome speak the same language. A target missing two bars is counted
// once, against the first — the three reasons sum to the skipped count.
func TestApprovePassing_SkipsAreNamedByTheBarTheyMissed(t *testing.T) {
	s, wsID, userID := newRecheckHarness(t)
	ctx := context.Background()

	bad := pendingFrBlock("bad", "Use the app", "Il faut utiliser l'application") // terms
	low := pendingFrBlock("low", "Save the app", "Sauver l'application")          // voice
	ok := pendingFrBlock("ok", "Close the app", "Fermer l'application")           // approved
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{bad, low, ok})

	tb, err := s.wsStores.getTerms("rc")
	require.NoError(t, err)
	seedTermUnificationConcepts(t, tb)

	profile := &coreprofile.VoiceProfile{ID: "p-bulk", Scope: wsID, Name: "Bulk Voice", MinScore: 90}
	require.NoError(t, s.VoiceStore.CreateProfile(ctx, profile))
	require.NoError(t, s.VoiceStore.StoreScore(ctx, &coreprofile.StoredScore{
		ProjectID: projID, Stream: "main", BlockID: ids["Save the app"], ProfileID: profile.ID,
		Locale: "fr", Score: 62, CheckedAt: time.Now().UTC(),
	}))

	e := s.GetEcho()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("rc", projID)
	c.Set("workspace_id", wsID)
	c.Set("user_id", userID)
	c.Set("project_permissions", platauth.PermAll)
	require.NoError(t, s.HandleApprovePassing(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	res := decodeJSON[ApprovePassingResponse](t, rec)
	assert.Equal(t, 1, res.Approved, "only the block clearing every bar is approved")
	assert.Equal(t, 2, res.Skipped)
	assert.Equal(t, 0, res.SkippedFailingChecks)
	assert.Equal(t, 1, res.SkippedTermViolations, "the forbidden-term target is named as such")
	assert.Equal(t, 1, res.SkippedBelowVoiceBar, "the below-bar target is named as such")
	assert.Equal(t, res.Skipped,
		res.SkippedFailingChecks+res.SkippedTermViolations+res.SkippedBelowVoiceBar+res.SkippedSelfAuthored,
		"every skip is attributed to exactly one bar")
}
