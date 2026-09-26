package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleCheckVoice_WholeWordAndConceptID proves the /check endpoint runs
// through the shared whole-word matcher (so "use" never flags inside "user") and
// propagates the concept_id and structured replacement of the workspace
// terms store's rule.
func TestHandleCheckVoice_WholeWordAndConceptID(t *testing.T) {
	srv := setupVoiceLoopServer(t)
	e := srv.GetEcho()
	ctx := context.Background()

	profile := &coreprofile.VoiceProfile{ID: "p-check", Scope: "ws-check", Name: "Check"}
	require.NoError(t, srv.VoiceStore.CreateProfile(ctx, profile))
	tb, err := srv.wsStores.getTerms("check")
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{ID: "c-use", Terms: []terms.Term{
		{Text: "adopt", Locale: "en", Status: model.TermPreferred},
		{Text: "use", Locale: "en", Status: model.TermForbidden},
	}}))

	check := func(text string) VoiceCheckResponse {
		body := fmt.Sprintf(`{"text":%q}`, text)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("ws", "id")
		c.SetParamValues("check", profile.ID)
		// WorkspaceAccessMiddleware sets workspace_id in production; set it here so
		// the handler's cross-tenant guard (profile.WorkspaceID must match the
		// request workspace) resolves to the profile's own workspace.
		c.Set("workspace_id", profile.Scope)
		require.NoError(t, srv.HandleCheckVoice(c))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var out VoiceCheckResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		return out
	}

	// Whole-word: "use" must not match inside "user" (the old substring bug).
	noHit := check("The user clicked the button")
	assert.Empty(t, noHit.Findings, "whole-word matcher must not flag 'use' inside 'user'")

	// A real occurrence flags and carries the concept link + structured replacement.
	hit := check("Please use the dashboard")
	require.Len(t, hit.Findings, 1)
	assert.Equal(t, "use", hit.Findings[0].OriginalText)
	assert.Equal(t, "c-use", hit.Findings[0].Metadata["concept_id"])
	assert.Equal(t, "adopt", hit.Findings[0].Metadata["replacement"])
}

// TestGetSuggestedRules_BackfillsConceptID proves correction-derived candidates
// surface the knowledge-graph concept their term already denotes, from the
// recorded rule decision (durably, after a demote too), while a concept-less
// suggestion stays empty.
func TestGetSuggestedRules_BackfillsConceptID(t *testing.T) {
	srv := setupVoiceLoopServer(t)
	ctx := context.Background()
	const wsID = "ws-concept-backfill"

	profile := &coreprofile.VoiceProfile{ID: "p-backfill", Scope: wsID, Name: "Backfill"}
	require.NoError(t, srv.VoiceStore.CreateProfile(ctx, profile))
	require.NoError(t, srv.VoiceStore.RecordRuleDecision(ctx, &coreprofile.RuleDecision{
		ProfileID: profile.ID, Term: "utilize", Replacement: "use",
		Status: coreprofile.RuleDecisionPromoted, ConceptID: "c-utilize",
		DecidedAt: time.Now().UTC(),
	}))

	// A concept-backed promotion that was later demoted keeps its concept on the
	// durable decision even though the live profile no longer carries the term.
	require.NoError(t, srv.VoiceStore.RecordRuleDecision(ctx, &coreprofile.RuleDecision{
		ProfileID: profile.ID, Term: "legacy", Replacement: "current",
		Status: coreprofile.RuleDecisionPromoted, ConceptID: "c-legacy",
		DecidedAt: time.Now().UTC(),
	}))

	store := func(term, repl string) {
		require.NoError(t, srv.VoiceStore.StoreCorrection(ctx, &coreprofile.Correction{
			ProfileID: profile.ID, Dimension: coreprofile.DimensionVocabulary,
			OriginalText: term, CorrectedText: repl, CorrectedBy: "u1",
		}))
	}
	for range 3 {
		store("utilize", "use")    // concept on the promotion's decision
		store("legacy", "current") // concept only on the durable decision
		store("plain", "simple")   // concept-less
	}

	rules, err := srv.VoiceStore.GetSuggestedRules(ctx, wsID, 3)
	require.NoError(t, err)
	byTerm := map[string]*coreprofile.SuggestedRule{}
	for _, r := range rules {
		byTerm[strings.ToLower(r.Term)] = r
	}

	require.NotNil(t, byTerm["utilize"])
	assert.Equal(t, "c-utilize", byTerm["utilize"].ConceptID,
		"a promoted term back-fills its concept")
	require.NotNil(t, byTerm["legacy"])
	assert.Equal(t, "c-legacy", byTerm["legacy"].ConceptID,
		"a demoted term's concept survives on the rule decision")
	require.NotNil(t, byTerm["plain"])
	assert.Empty(t, byTerm["plain"].ConceptID,
		"a concept-less suggestion stays empty")
}
