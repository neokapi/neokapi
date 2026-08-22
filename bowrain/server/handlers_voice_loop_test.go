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

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	voicepg "github.com/neokapi/neokapi/bowrain/voice"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupVoiceLoopServer(t *testing.T) *Server {
	t.Helper()
	cfg := DefaultConfig()
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv) // ContentStore + shared test DB (skips if no container)
	db := pgtest.NewTestDB(t)
	bs, err := voicepg.NewPostgresVoiceStore(db)
	require.NoError(t, err)
	srv.VoiceStore = bs
	return srv
}

// TestVoiceLoop_EndToEnd exercises the correction-learning loop over the real
// HTTP handlers and Postgres store: corrections aggregate into candidates, a
// candidate is promoted (and leaves the list, recorded + versioned), another is
// rejected (and is suppressed), and progressive autonomy auto-promotes once a
// term crosses the threshold.
func TestVoiceLoop_EndToEnd(t *testing.T) {
	srv := setupVoiceLoopServer(t)
	e := srv.GetEcho()
	ctx := context.Background()

	const wsID = "ws-loop-e2e"
	const userID = "u-loop-e2e"
	profile := &coreprofile.VoiceProfile{ID: "p-loop-e2e", Scope: wsID, Name: "Loop E2E"}
	require.NoError(t, srv.VoiceStore.CreateProfile(ctx, profile))

	// correct posts a correction through the handler and returns the decoded body.
	correct := func(term, replacement string) map[string]any {
		body := fmt.Sprintf(`{"profile_id":%q,"original_text":%q,"corrected_text":%q,"dimension":"vocabulary"}`,
			profile.ID, term, replacement)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)
		c.Set("user_id", userID)
		c.Set("workspace_id", wsID)
		require.NoError(t, srv.HandleCreateVoiceCorrection(c))
		require.Equal(t, http.StatusCreated, rec.Code)
		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		return out
	}

	candidates := func(all bool) []coreprofile.CandidateRule {
		url := "/?min_count=3"
		if all {
			url += "&all=true"
		}
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)
		c.SetParamNames("id")
		c.SetParamValues(profile.ID)
		c.Set("workspace_id", wsID)
		require.NoError(t, srv.HandleListCandidates(c))
		require.Equal(t, http.StatusOK, rec.Code)
		var out []coreprofile.CandidateRule
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		return out
	}

	decide := func(handler echo.HandlerFunc, term, replacement string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"term":%q,"replacement":%q,"dimension":"vocabulary","correction_count":3}`, term, replacement)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)
		c.SetParamNames("id")
		c.SetParamValues(profile.ID)
		c.Set("user_id", userID)
		c.Set("workspace_id", wsID)
		require.NoError(t, handler(c))
		return rec
	}

	find := func(cs []coreprofile.CandidateRule, term string) *coreprofile.CandidateRule {
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
	assert.Equal(t, coreprofile.RuleDecisionPending, c.Status)
	assert.Equal(t, 3, c.CorrectionCount)

	// ── promote → leaves the list, recorded + enforced + versioned ─────
	rec := decide(srv.HandlePromoteSuggestedRule, "utilize", "use")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Nil(t, find(candidates(false), "utilize"), "promoted candidate should leave the review list")
	got, err := srv.VoiceStore.GetProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.Len(t, got.Vocabulary.ForbiddenTerms, 1)
	assert.Equal(t, "utilize", got.Vocabulary.ForbiddenTerms[0].Term)
	d, err := srv.VoiceStore.GetRuleDecision(ctx, profile.ID, "utilize")
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, coreprofile.RuleDecisionPromoted, d.Status)
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
	assert.Equal(t, coreprofile.RuleDecisionRejected, hist.Status)

	// ── progressive autonomy → auto-promote at threshold ───────────────
	got.Autonomy = coreprofile.AutonomyConfig{AutoPromoteAtCount: 2}
	require.NoError(t, srv.VoiceStore.UpdateProfile(ctx, got))
	first := correct("synergy", "teamwork")
	assert.Nil(t, first["auto_promoted"], "one correction is below the threshold")
	second := correct("synergy", "teamwork")
	assert.Equal(t, "synergy", second["auto_promoted"], "second correction crosses the threshold and auto-promotes")
	d, err = srv.VoiceStore.GetRuleDecision(ctx, profile.ID, "synergy")
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, coreprofile.RuleDecisionPromoted, d.Status)
	assert.True(t, d.Auto, "autonomy-promoted decision should be marked auto")
}

// TestPhase4_VoiceRuleDemote proves a promoted brand rule can be demoted
// (removed) — promoted rules are no longer append-only.
func TestPhase4_VoiceRuleDemote(t *testing.T) {
	srv := setupVoiceLoopServer(t)
	e := srv.GetEcho()
	ctx := context.Background()
	profile := &coreprofile.VoiceProfile{ID: "p-demote", Scope: "ws-d", Name: "D"}
	require.NoError(t, srv.VoiceStore.CreateProfile(ctx, profile))

	_, changed, err := coreprofile.PromoteAndSave(ctx, srv.VoiceStore, profile.ID,
		coreprofile.SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 3})
	require.NoError(t, err)
	require.True(t, changed)
	got, _ := srv.VoiceStore.GetProfile(ctx, profile.ID)
	require.Len(t, got.Vocabulary.ForbiddenTerms, 1)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"term":"utilize"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.SetParamNames("id")
	c.SetParamValues(profile.ID)
	require.NoError(t, srv.HandleDemoteSuggestedRule(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	got, _ = srv.VoiceStore.GetProfile(ctx, profile.ID)
	assert.Empty(t, got.Vocabulary.ForbiddenTerms, "demote should remove the promoted rule")
}

// TestVoiceLoop_EvaluateBlastRadius proves the blast-radius preview endpoint runs
// the candidate rule over real stored content and reports the impact before the
// rule is promoted.
func TestVoiceLoop_EvaluateBlastRadius(t *testing.T) {
	srv := setupVoiceLoopServer(t)
	e := srv.GetEcho()
	ctx := context.Background()

	const wsID = "ws-blast"
	profile := &coreprofile.VoiceProfile{ID: "p-blast", Scope: wsID, Name: "Blast"}
	require.NoError(t, srv.VoiceStore.CreateProfile(ctx, profile))

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
	c.Set("project_permissions", platauth.PermAll)
	c.SetParamNames("id")
	c.SetParamValues(profile.ID)
	c.Set("workspace_id", wsID)
	require.NoError(t, srv.HandleEvaluateRulePromotion(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var radius coreprofile.BlastRadius
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &radius))
	assert.Equal(t, 3, radius.TotalBlocks)
	assert.Equal(t, 2, radius.NewViolations, "two blocks contain 'utilize'")
	assert.Equal(t, 2, radius.AffectedBlocks)
	assert.Equal(t, 0, radius.ResolvedViolations)
}

// TestVoiceLoop_Drift proves the drift endpoint detects a compliance decline from
// a project's stored score trend.
func TestVoiceLoop_Drift(t *testing.T) {
	srv := setupVoiceLoopServer(t)
	e := srv.GetEcho()
	ctx := context.Background()

	const projectID = "proj-drift"
	now := time.Now().UTC()
	store := func(seq, day, score int) {
		require.NoError(t, srv.VoiceStore.StoreScore(ctx, &coreprofile.StoredScore{
			ID:        fmt.Sprintf("s-%d", seq),
			ProjectID: projectID,
			Stream:    "main",
			BlockID:   fmt.Sprintf("b-%d", seq),
			Locale:    "en",
			Score:     score,
			CheckedAt: now.AddDate(0, 0, -day),
		}))
	}
	// Baseline ~5 days ago scored high; the most recent day scored low.
	seq := 0
	for i := range 3 {
		store(seq, 5, 94+i%2) // 94/95/94 five days ago
		seq++
		store(seq, 0, 70+i) // 70/71/72 today
		seq++
	}

	req := httptest.NewRequest(http.MethodGet, "/?recent_days=1&drop_points=10&days=30", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "ref")
	c.SetParamValues(projectID, "main")
	require.NoError(t, srv.HandleGetVoiceDrift(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var result coreprofile.DriftResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.True(t, result.Drifted, "a ~24-point decline should register as drift: %+v", result)
	assert.Greater(t, result.BaselineAvg, result.RecentAvg)
	assert.Equal(t, "recent average dropped from baseline", result.Reason)
}
