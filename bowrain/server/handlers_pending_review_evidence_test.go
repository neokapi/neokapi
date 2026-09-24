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

// The review queue must include the evidence used by approve-passing: rule-based
// checks, terms and voice scores. These tests verify that clients receive enough
// evidence to classify pending targets consistently with the server.

// pendingFrBlock builds a translatable block whose fr target is a pending
// draft — a candidate for both the queue and the bulk approve-passing pass.
func pendingFrBlock(id, source, frTarget string) *model.Block {
	b := &model.Block{ID: id, Translatable: true}
	b.SetSourceText(source)
	b.SetTargetText("fr", frTarget)
	b.Target("fr").Status = model.TargetStatusDraft
	return b
}

// bindVoiceProfile binds a voice profile to a project, the rung voicescope
// resolves for every locale of it, so the profile governs them.
func bindVoiceProfile(t *testing.T, s *Server, projID, profileID string) {
	t.Helper()
	ctx := context.Background()
	proj, err := s.ContentStore.GetProject(ctx, projID)
	require.NoError(t, err)
	if proj.Properties == nil {
		proj.Properties = map[string]string{}
	}
	proj.Properties[coreprofile.PropertyProfileID] = profileID
	require.NoError(t, s.ContentStore.UpdateProject(ctx, proj))
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

	// A profile with a raised bar, bound to the project so it governs fr, and two
	// scores against it: one below the bar, one above. A scored block carries its
	// score against the bar, and an unscored one carries the bar alone.
	profile := &coreprofile.VoiceProfile{ID: "p-queue", Scope: wsID, Name: "Queue Voice", MinScore: 90}
	require.NoError(t, s.VoiceStore.CreateProfile(ctx, profile))
	bindVoiceProfile(t, s, projID, profile.ID)
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
		assert.Nil(t, unscored.VoiceScore, "nothing has scored this block")
		require.NotNil(t, unscored.VoiceBar, "the governing profile still holds it to a bar")
		assert.Equal(t, 90, *unscored.VoiceBar)
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
			termsClear := e.TermCompliance == platstore.TermComplianceCompliant ||
				e.TermCompliance == platstore.TermComplianceNotGoverned
			voiceClear := e.VoiceBar == nil || (e.VoiceScore != nil && *e.VoiceScore >= *e.VoiceBar)
			fromEvidence := termsClear && voiceClear
			fromServer := blockCompliantAndPassing(ctx, block, "fr", scores, gate)
			assert.Equal(t, fromServer, fromEvidence,
				"block %s: the queue's evidence and the server's predicate disagree", e.BlockID)
		}
	})
}

// TestPendingReview_NotGovernedIsNotCompliant pins the not-governed state. With
// no terms store, no brand vocabulary and no bound voice profile there is
// nothing to comply with: a queue reporting "compliant" would claim evidence it
// never had, and no bar is owed a result either.
func TestPendingReview_NotGovernedIsNotCompliant(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)

	// No concepts seeded and no profile bound: nothing governs fr beyond the checks.
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("plain", "Use the app", "Il faut utiliser l'application"),
	})

	page := listPendingReview(t, s, wsID, projID)
	require.Len(t, page.Entries, 1)
	entry := entryFor(t, page, ids["Use the app"])
	assert.Equal(t, platstore.TermComplianceNotGoverned, entry.TermCompliance,
		"with nothing governing the locale the verdict is not governed, not compliant")
	assert.Nil(t, entry.VoiceScore, "no voice profile governs the locale")
	assert.Nil(t, entry.VoiceBar, "so no voice bar applies to the block")
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
	bindVoiceProfile(t, s, projID, profile.ID)
	score := func(blockID string, value int) {
		require.NoError(t, s.VoiceStore.StoreScore(ctx, &coreprofile.StoredScore{
			ProjectID: projID, Stream: "main", BlockID: blockID, ProfileID: profile.ID,
			Locale: "fr", Score: value, CheckedAt: time.Now().UTC(),
		}))
	}
	score(ids["Save the app"], 62)
	score(ids["Close the app"], 95)

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
	assert.Equal(t, 0, res.SkippedTermsNotChecked, "terms are bound, so every target's terminology was checked")
	assert.Equal(t, 1, res.SkippedBelowVoiceBar, "the below-bar target is named as such")
	assert.Equal(t, 0, res.SkippedVoiceNotChecked, "the forbidden-term target is counted against terminology first")
	assert.Equal(t, res.Skipped,
		res.SkippedFailingChecks+res.SkippedTermViolations+res.SkippedTermsNotChecked+
			res.SkippedBelowVoiceBar+res.SkippedVoiceNotChecked+res.SkippedSelfAuthored,
		"every skip is attributed to exactly one bar")
}

// TestApprovePassing_ApprovesWhereNothingGoverns: with no terms, no voice
// profile rules and no bound voice profile, nothing governs the locale beyond the
// checks. A bar that governs nothing is no bar, so the pass approves the targets
// the checks clear and names no skip for terminology or voice.
func TestApprovePassing_ApprovesWhereNothingGoverns(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
		pendingFrBlock("b2", "Goodbye", "Au revoir"),
	})

	page := listPendingReview(t, s, wsID, projID)
	require.Len(t, page.Entries, 2)
	for _, e := range page.Entries {
		assert.Equal(t, platstore.TermComplianceNotGoverned, e.TermCompliance, "block %s", e.BlockID)
		assert.Nil(t, e.VoiceBar, "block %s: no voice profile governs fr", e.BlockID)
	}

	rec, res := callApprovePassing(t, s, wsID, projID, ownerID, `{}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 2, res.Approved, "a target nothing governs beyond the checks is approved on the checks")
	assert.Equal(t, 0, res.Skipped)
	assert.Equal(t, 0, res.SkippedTermsNotChecked, "terminology governs nothing here, so it skips nothing")
	assert.Equal(t, 0, res.SkippedVoiceNotChecked, "no voice profile governs fr, so voice skips nothing")
	assert.Equal(t, 0, res.RemainingPending)
	assert.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, projID, ids["Hello"], "fr"))
	assert.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, projID, ids["Goodbye"], "fr"))
}

// TestApprovePassing_NeverApprovesUnscoredVoice: a voice profile bound to the
// project governs fr, and nothing has scored either target. The voice bar has no
// result for them, so the pass approves neither, names the skips for the voice
// bar, and leaves both pending.
func TestApprovePassing_NeverApprovesUnscoredVoice(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	ctx := context.Background()
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b1", "Hello", "Bonjour"),
		pendingFrBlock("b2", "Goodbye", "Au revoir"),
	})
	profile := &coreprofile.VoiceProfile{ID: "p-unscored", Scope: wsID, Name: "Unscored Voice", MinScore: 80}
	require.NoError(t, s.VoiceStore.CreateProfile(ctx, profile))
	bindVoiceProfile(t, s, projID, profile.ID)

	page := listPendingReview(t, s, wsID, projID)
	require.Len(t, page.Entries, 2)
	for _, e := range page.Entries {
		assert.Nil(t, e.VoiceScore, "block %s: nothing has scored it", e.BlockID)
		require.NotNil(t, e.VoiceBar, "block %s: the bound profile holds it to a bar", e.BlockID)
		assert.Equal(t, 80, *e.VoiceBar)
	}

	rec, res := callApprovePassing(t, s, wsID, projID, ownerID, `{}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 0, res.Approved, "a target with no result against a governing voice bar is never approved")
	assert.Equal(t, 2, res.Skipped)
	assert.Equal(t, 2, res.SkippedVoiceNotChecked, "the skips are named for the voice bar that was not checked")
	assert.Equal(t, 0, res.SkippedBelowVoiceBar, "an unscored target is below nothing")
	assert.Equal(t, 2, res.RemainingPending)
	assert.Equal(t, model.TargetStatusDraft, targetStatus(t, s, projID, ids["Hello"], "fr"))
	assert.Equal(t, model.TargetStatusDraft, targetStatus(t, s, projID, ids["Goodbye"], "fr"))
}
