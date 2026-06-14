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

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleCheckBrandVoice_WholeWordAndConceptID proves the /check endpoint runs
// through the shared whole-word matcher (so "use" never flags inside "user") and
// propagates a concept-backed rule's concept_id and structured replacement —
// closing the divergent substring path the endpoint used to take.
func TestHandleCheckBrandVoice_WholeWordAndConceptID(t *testing.T) {
	srv := setupBrandLoopServer(t)
	e := srv.GetEcho()
	ctx := context.Background()

	profile := &corebrand.VoiceProfile{
		ID: "p-check", WorkspaceID: "ws-check", Name: "Check",
		Vocabulary: corebrand.VocabularyRules{
			ForbiddenTerms: []corebrand.TermRule{
				{Term: "use", Replacement: "adopt", ConceptID: "c-use"},
			},
		},
	}
	require.NoError(t, srv.BrandStore.CreateProfile(ctx, profile))

	check := func(text string) BrandCheckResponse {
		body := fmt.Sprintf(`{"text":%q}`, text)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(profile.ID)
		require.NoError(t, srv.HandleCheckBrandVoice(c))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var out BrandCheckResponse
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
// surface the knowledge-graph concept their term already denotes — from the live
// profile's enforced vocabulary, and (durably, after a demote) from the recorded
// rule decision — while a concept-less suggestion stays empty.
func TestGetSuggestedRules_BackfillsConceptID(t *testing.T) {
	srv := setupBrandLoopServer(t)
	ctx := context.Background()
	const wsID = "ws-concept-backfill"

	profile := &corebrand.VoiceProfile{
		ID: "p-backfill", WorkspaceID: wsID, Name: "Backfill",
		Vocabulary: corebrand.VocabularyRules{
			ForbiddenTerms: []corebrand.TermRule{
				{Term: "utilize", Replacement: "use", ConceptID: "c-utilize"},
			},
		},
	}
	require.NoError(t, srv.BrandStore.CreateProfile(ctx, profile))

	// A concept-backed promotion that was later demoted keeps its concept on the
	// durable decision even though the live profile no longer carries the term.
	require.NoError(t, srv.BrandStore.RecordRuleDecision(ctx, &corebrand.RuleDecision{
		ProfileID: profile.ID, Term: "legacy", Replacement: "current",
		Status: corebrand.RuleDecisionPromoted, ConceptID: "c-legacy",
		DecidedAt: time.Now().UTC(),
	}))

	store := func(term, repl string) {
		require.NoError(t, srv.BrandStore.StoreCorrection(ctx, &corebrand.Correction{
			ProfileID: profile.ID, Dimension: corebrand.DimensionVocabulary,
			OriginalText: term, CorrectedText: repl, CorrectedBy: "u1",
		}))
	}
	for range 3 {
		store("utilize", "use")    // concept on the live profile
		store("legacy", "current") // concept only on the durable decision
		store("plain", "simple")   // concept-less
	}

	rules, err := srv.BrandStore.GetSuggestedRules(ctx, wsID, 3)
	require.NoError(t, err)
	byTerm := map[string]*corebrand.SuggestedRule{}
	for _, r := range rules {
		byTerm[strings.ToLower(r.Term)] = r
	}

	require.NotNil(t, byTerm["utilize"])
	assert.Equal(t, "c-utilize", byTerm["utilize"].ConceptID,
		"a live forbidden term back-fills its concept")
	require.NotNil(t, byTerm["legacy"])
	assert.Equal(t, "c-legacy", byTerm["legacy"].ConceptID,
		"a demoted term's concept survives on the rule decision")
	require.NotNil(t, byTerm["plain"])
	assert.Empty(t, byTerm["plain"].ConceptID,
		"a concept-less suggestion stays empty")
}
