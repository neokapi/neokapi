package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/platformconfig"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupModelSweepServer wires a server with real Postgres sweep + platform
// config stores so the flag gate and the recommendation query run for real.
func setupModelSweepServer(t *testing.T) *Server {
	t.Helper()
	cfg := DefaultConfig()
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)

	db := pgtest.NewTestDB(t)
	sweeps, err := jobs.NewModelSweepStore(db)
	require.NoError(t, err)
	srv.SweepStore = sweeps

	pcs, err := platformconfig.NewStore(db)
	require.NoError(t, err)
	srv.PlatformConfig = platformconfig.NewService(pcs, platformconfig.Defaults{})
	require.NoError(t, srv.PlatformConfig.Refresh(t.Context()))
	return srv
}

// setModelSweepsFlag flips model_sweeps.enabled on the server's platform
// config service (persisted, then re-read — the same path a ctrl write takes).
func setModelSweepsFlag(t *testing.T, srv *Server, enabled bool) {
	t.Helper()
	raw, _ := json.Marshal(enabled)
	require.NoError(t, srv.PlatformConfig.Set(t.Context(),
		map[string]json.RawMessage{platformconfig.KeyModelSweeps: raw}, "test"))
}

func sweepCtx(t *testing.T, srv *Server, method, ws, projectID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(method, "/", nil)
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues(ws, projectID)
	c.Set("project_permissions", platauth.PermAll)
	c.Set("workspace_id", "ws-sweep-id")
	return c, rec
}

// TestModelRecommendations_RefreshFlagGate proves the endpoint gate: refresh
// is 403 while model_sweeps.enabled is off, and enqueues one sweep job per
// target locale once the founder flips it on.
func TestModelRecommendations_RefreshFlagGate(t *testing.T) {
	srv := setupModelSweepServer(t)
	ctx := t.Context()

	require.NoError(t, srv.ContentStore.CreateProject(ctx, &platstore.Project{
		ID:                    "proj-ms",
		Name:                  "Sweep",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr", "de"},
		WorkspaceID:           "ws-sweep-id",
	}))

	// Flag off (the default): refuse with a reason.
	c, rec := sweepCtx(t, srv, http.MethodPost, "acme", "proj-ms")
	require.NoError(t, srv.HandleRefreshModelRecommendations(c))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "model_sweeps.enabled")

	// Flag on: one sweep job per target locale rides the translation queue.
	setModelSweepsFlag(t, srv, true)
	c, rec = sweepCtx(t, srv, http.MethodPost, "acme", "proj-ms")
	require.NoError(t, srv.HandleRefreshModelRecommendations(c))
	require.Equal(t, http.StatusAccepted, rec.Code)
	var resp struct {
		Enqueued int      `json:"enqueued"`
		Locales  []string `json:"locales"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Enqueued)
	assert.ElementsMatch(t, []string{"fr", "de"}, resp.Locales)

	queued, err := srv.JobStore.ListJobs(ctx, "acme", 10)
	require.NoError(t, err)
	require.Len(t, queued, 2)
	locales := []string{}
	for _, j := range queued {
		assert.True(t, j.IsModelSweep(), "enqueued jobs must carry the sweep sentinel")
		assert.Equal(t, "proj-ms", j.ProjectID)
		assert.Equal(t, "ws-sweep-id", j.WorkspaceID)
		locales = append(locales, j.TargetLocale)
	}
	assert.ElementsMatch(t, []string{"fr", "de"}, locales)
}

// TestModelRecommendations_GetMarksWinner proves the read side: the latest
// measurement per (locale, model), the per-locale recommendation marked by the
// shared policy, and the instance gate surfaced for the settings panel.
func TestModelRecommendations_GetMarksWinner(t *testing.T) {
	srv := setupModelSweepServer(t)
	ctx := t.Context()
	now := time.Now().UTC()

	seed := []*jobs.ModelSweepResult{
		{ProjectID: "proj-get", Locale: "fr", Model: "m-strong", FixtureDigest: "d1",
			FixtureCount: 12, Adherence: 0.9, AdherenceBare: 0.5, Lift: 0.4, TokensUsed: 100, MeasuredAt: now},
		{ProjectID: "proj-get", Locale: "fr", Model: "m-weak", FixtureDigest: "d1",
			FixtureCount: 12, Adherence: 0.8, AdherenceBare: 0.7, Lift: 0.1, TokensUsed: 90, MeasuredAt: now},
		// Below the absolute-adherence floor: listed, never recommended.
		{ProjectID: "proj-get", Locale: "de", Model: "m-floor", FixtureDigest: "d2",
			FixtureCount: 12, Adherence: 0.4, AdherenceBare: 0.1, Lift: 0.3, TokensUsed: 80, MeasuredAt: now},
	}
	for _, r := range seed {
		require.NoError(t, srv.SweepStore.UpsertModelSweepResult(ctx, r))
	}

	c, rec := sweepCtx(t, srv, http.MethodGet, "acme", "proj-get")
	require.NoError(t, srv.HandleGetModelRecommendations(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Enabled bool `json:"enabled"`
		Locales []struct {
			Locale           string `json:"locale"`
			RecommendedModel string `json:"recommended_model"`
			Models           []struct {
				Model       string  `json:"model"`
				Lift        float64 `json:"lift"`
				Recommended bool    `json:"recommended"`
			} `json:"models"`
		} `json:"locales"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Enabled, "flag defaults off; the panel disables Refresh with a reason")
	require.Len(t, resp.Locales, 2)

	byLocale := map[string]int{}
	for i, l := range resp.Locales {
		byLocale[l.Locale] = i
	}
	fr := resp.Locales[byLocale["fr"]]
	assert.Equal(t, "m-strong", fr.RecommendedModel)
	require.Len(t, fr.Models, 2)
	assert.Equal(t, "m-strong", fr.Models[0].Model, "rows come lift-descending")
	assert.True(t, fr.Models[0].Recommended)
	assert.False(t, fr.Models[1].Recommended)

	de := resp.Locales[byLocale["de"]]
	assert.Empty(t, de.RecommendedModel, "a below-floor measurement backs no recommendation")
	require.Len(t, de.Models, 1)
	assert.False(t, de.Models[0].Recommended)
}
