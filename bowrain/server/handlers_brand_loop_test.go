package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	brandpg "github.com/neokapi/neokapi/bowrain/brand"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupBrandLoopServer(t *testing.T) *Server {
	t.Helper()
	cfg := DefaultConfig()
	srv := NewServer(cfg)
	initTestStores(t, srv) // ContentStore + shared test DB (skips if no container)
	db := pgtest.NewTestDB(t)
	bs, err := brandpg.NewPostgresBrandStore(db)
	require.NoError(t, err)
	srv.BrandStore = bs
	return srv
}

// TestBrandLoop_EndToEnd exercises the correction-learning loop over the real
// HTTP handlers and Postgres store: corrections aggregate into candidates, a
// candidate is promoted (and leaves the list, recorded + versioned), another is
// rejected (and is suppressed), and progressive autonomy auto-promotes once a
// term crosses the threshold.
func TestBrandLoop_EndToEnd(t *testing.T) {
	srv := setupBrandLoopServer(t)
	e := srv.GetEcho()
	ctx := context.Background()

	const wsID = "ws-loop-e2e"
	const userID = "u-loop-e2e"
	profile := &corebrand.VoiceProfile{ID: "p-loop-e2e", WorkspaceID: wsID, Name: "Loop E2E"}
	require.NoError(t, srv.BrandStore.CreateProfile(ctx, profile))

	// correct posts a correction through the handler and returns the decoded body.
	correct := func(term, replacement string) map[string]any {
		body := fmt.Sprintf(`{"profile_id":%q,"original_text":%q,"corrected_text":%q,"dimension":"vocabulary"}`,
			profile.ID, term, replacement)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("user_id", userID)
		c.Set("workspace_id", wsID)
		require.NoError(t, srv.HandleCreateBrandVoiceCorrection(c))
		require.Equal(t, http.StatusCreated, rec.Code)
		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		return out
	}

	candidates := func(all bool) []corebrand.CandidateRule {
		url := "/?min_count=3"
		if all {
			url += "&all=true"
		}
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(profile.ID)
		c.Set("workspace_id", wsID)
		require.NoError(t, srv.HandleListCandidates(c))
		require.Equal(t, http.StatusOK, rec.Code)
		var out []corebrand.CandidateRule
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		return out
	}

	decide := func(handler echo.HandlerFunc, term, replacement string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"term":%q,"replacement":%q,"dimension":"vocabulary","correction_count":3}`, term, replacement)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(profile.ID)
		c.Set("user_id", userID)
		c.Set("workspace_id", wsID)
		require.NoError(t, handler(c))
		return rec
	}

	find := func(cs []corebrand.CandidateRule, term string) *corebrand.CandidateRule {
		for i := range cs {
			if strings.EqualFold(cs[i].Term, term) {
				return &cs[i]
			}
		}
		return nil
	}

	// ── corrections → pending candidate ────────────────────────────────
	for range 3 {
		correct("utilize", "use")
	}
	c := find(candidates(false), "utilize")
	require.NotNil(t, c, "utilize should be a candidate")
	assert.Equal(t, corebrand.RuleDecisionPending, c.Status)
	assert.Equal(t, 3, c.CorrectionCount)

	// ── promote → leaves the list, recorded + enforced + versioned ─────
	rec := decide(srv.HandlePromoteSuggestedRule, "utilize", "use")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Nil(t, find(candidates(false), "utilize"), "promoted candidate should leave the review list")
	got, err := srv.BrandStore.GetProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.Len(t, got.Vocabulary.ForbiddenTerms, 1)
	assert.Equal(t, "utilize", got.Vocabulary.ForbiddenTerms[0].Term)
	d, err := srv.BrandStore.GetRuleDecision(ctx, profile.ID, "utilize")
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, corebrand.RuleDecisionPromoted, d.Status)
	assert.Equal(t, got.Version, d.PromotedVersion)

	// ── reject → suppressed from the list, visible in history ──────────
	for range 3 {
		correct("leverage", "use")
	}
	require.NotNil(t, find(candidates(false), "leverage"))
	rec = decide(srv.HandleRejectSuggestedRule, "leverage", "use")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Nil(t, find(candidates(false), "leverage"), "rejected candidate should be suppressed")
	hist := find(candidates(true), "leverage")
	require.NotNil(t, hist, "rejected candidate should remain in history")
	assert.Equal(t, corebrand.RuleDecisionRejected, hist.Status)

	// ── progressive autonomy → auto-promote at threshold ───────────────
	got.Autonomy = corebrand.AutonomyConfig{AutoPromoteAtCount: 2}
	require.NoError(t, srv.BrandStore.UpdateProfile(ctx, got))
	first := correct("synergy", "teamwork")
	assert.Nil(t, first["auto_promoted"], "one correction is below the threshold")
	second := correct("synergy", "teamwork")
	assert.Equal(t, "synergy", second["auto_promoted"], "second correction crosses the threshold and auto-promotes")
	d, err = srv.BrandStore.GetRuleDecision(ctx, profile.ID, "synergy")
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, corebrand.RuleDecisionPromoted, d.Status)
	assert.True(t, d.Auto, "autonomy-promoted decision should be marked auto")
}

// TestBrandLoop_EvaluateBlastRadius proves the blast-radius preview endpoint runs
// the candidate rule over real stored content and reports the impact before the
// rule is promoted.
func TestBrandLoop_EvaluateBlastRadius(t *testing.T) {
	srv := setupBrandLoopServer(t)
	e := srv.GetEcho()
	ctx := context.Background()

	const wsID = "ws-blast"
	profile := &corebrand.VoiceProfile{ID: "p-blast", WorkspaceID: wsID, Name: "Blast"}
	require.NoError(t, srv.BrandStore.CreateProfile(ctx, profile))

	const projectID = "proj-blast"
	require.NoError(t, srv.ContentStore.CreateProject(ctx, &platstore.Project{
		ID: projectID, Name: "Blast Content", DefaultSourceLanguage: "en",
	}))
	block := func(idStr, text string) *model.Block {
		return &model.Block{ID: idStr, Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: text}}}}
	}
	require.NoError(t, srv.ContentStore.StoreBlocks(ctx, projectID, "main", []*model.Block{
		block("b1", "Please utilize the dashboard"),
		block("b2", "Utilize it again here"),
		block("b3", "A clean sentence with nothing to flag"),
	}))

	body := fmt.Sprintf(`{"term":"utilize","replacement":"use","project_id":%q,"stream":"main"}`, projectID)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(profile.ID)
	c.Set("workspace_id", wsID)
	require.NoError(t, srv.HandleEvaluateRulePromotion(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var radius corebrand.BlastRadius
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &radius))
	assert.Equal(t, 3, radius.TotalBlocks)
	assert.Equal(t, 2, radius.NewViolations, "two blocks contain 'utilize'")
	assert.Equal(t, 2, radius.AffectedBlocks)
	assert.Equal(t, 0, radius.ResolvedViolations)
}
