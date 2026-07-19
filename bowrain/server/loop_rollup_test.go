package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getLoopRollup fetches GET /:ws/loop-rollup and decodes the payload.
func getLoopRollup(t *testing.T, srv *Server, token string) loopRollupResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/loop-rollup", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var resp loopRollupResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

// createLoopProject creates a project in the test workspace directly on the
// content store (the plan gate caps API creation at one project on the free
// plan; the rollup needs several) and returns its ID.
func createLoopProject(t *testing.T, srv *Server, name string, targets []string) string {
	t.Helper()
	locales := make([]model.LocaleID, len(targets))
	for i, l := range targets {
		locales[i] = model.LocaleID(l)
	}
	proj := &platstore.Project{
		Name:                  name,
		DefaultSourceLanguage: "en",
		TargetLanguages:       locales,
		WorkspaceID:           "test-ws",
		Properties:            map[string]string{},
	}
	require.NoError(t, srv.ContentStore.CreateProject(t.Context(), proj))
	return proj.ID
}

// TestLoopRollupEmptyWorkspace covers the boundary: a workspace with no
// projects answers 200 with an empty rollup — no latest run, no ship data —
// so the client hides both cards.
func TestLoopRollupEmptyWorkspace(t *testing.T) {
	srv, token := newTestServer(t)
	resp := getLoopRollup(t, srv, token)
	assert.Nil(t, resp.LatestRun)
	assert.Nil(t, resp.Ship)
}

// TestLoopRollupLatestRunAcrossProjects seeds runs on two projects and
// asserts the rollup surfaces the single most recent one — with its project
// attribution, stall reason, and deep-link stream — while ship stays absent
// (no cached dashboard rollups yet).
func TestLoopRollupLatestRunAcrossProjects(t *testing.T) {
	srv, token := newTestServer(t)
	pidA := createLoopProject(t, srv, "Alpha", []string{"fr"})
	pidB := createLoopProject(t, srv, "Beta", []string{"de"})

	ctx := t.Context()
	base := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	finished := base.Add(10 * time.Minute)
	require.NoError(t, srv.ConvergenceRunStore.CreateRun(ctx, &bstore.ConvergenceRun{
		ProjectID: pidA, Trigger: "push",
		State: bstore.ConvergenceRunConverged, CreatedAt: base, FinishedAt: &finished,
	}))
	newestFinished := base.Add(2 * time.Hour)
	newest := &bstore.ConvergenceRun{
		ProjectID: pidB, Trigger: "manual",
		State:       bstore.ConvergenceRunParked,
		StallReason: convergence.StallNeedsCredits,
		CreatedAt:   base.Add(time.Hour), FinishedAt: &newestFinished,
	}
	require.NoError(t, srv.ConvergenceRunStore.CreateRun(ctx, newest))

	resp := getLoopRollup(t, srv, token)
	require.NotNil(t, resp.LatestRun)
	assert.Equal(t, newest.ID, resp.LatestRun.ID)
	assert.Equal(t, pidB, resp.LatestRun.ProjectID)
	assert.Equal(t, "Beta", resp.LatestRun.ProjectName)
	assert.Equal(t, "main", resp.LatestRun.Stream)
	assert.Equal(t, bstore.ConvergenceRunParked, resp.LatestRun.State)
	assert.Equal(t, convergence.StallNeedsCredits, resp.LatestRun.StallReason)
	assert.Equal(t, newestFinished.Format(time.RFC3339), resp.LatestRun.UpdatedAt)

	// No dashboard has been viewed → no cached rollups → ship stays absent
	// rather than reporting a guess.
	assert.Nil(t, resp.Ship)
}

// TestLoopRollupShipStates seeds two projects with mixed per-locale ship
// states, primes the per-project dashboard cache through the real dashboard
// endpoint (the same path #1342's applyShipStates enriches), and asserts the
// rollup folds the cached states into workspace counts, labeled with its
// cached basis and coverage.
func TestLoopRollupShipStates(t *testing.T) {
	srv, token := newTestServer(t)

	// Alpha (en → fr, de): one block, fr translated+reviewed → governed;
	// de translated, unreviewed → ai_shippable.
	pidA := createLoopProject(t, srv, "Alpha", []string{"fr", "de"})
	ctx := t.Context()
	cs := srv.ContentStore
	b := &model.Block{ID: "a-b1", Translatable: true}
	b.SetSourceText("Hello world")
	b.SetTargetText("fr", "Bonjour le monde")
	b.StampTargetProvenance("fr", model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	b.SetTargetText("de", "Hallo Welt")
	require.NoError(t, cs.StoreItem(ctx, pidA, "main", &platstore.Item{ProjectID: pidA, Name: "a.json", Format: "json"}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, pidA, "main", "a.json", []*model.Block{b}))

	// Beta (en → fr): one untranslated block → pending.
	pidB := createLoopProject(t, srv, "Beta", []string{"fr"})
	b2 := &model.Block{ID: "b-b1", Translatable: true}
	b2.SetSourceText("Pending copy")
	require.NoError(t, cs.StoreItem(ctx, pidB, "main", &platstore.Item{ProjectID: pidB, Name: "b.json", Format: "json"}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, pidB, "main", "b.json", []*model.Block{b2}))

	// Prime the dashboard cache for both projects (real endpoint, real
	// applyShipStates pass); the rollup itself must not recompute anything.
	getDashboard(t, srv, token, pidA, "")
	getDashboard(t, srv, token, pidB, "")

	resp := getLoopRollup(t, srv, token)
	require.NotNil(t, resp.Ship)
	ship := resp.Ship
	assert.Equal(t, loopShipBasisCached, ship.Basis)
	assert.Equal(t, 1, ship.Governed, "Alpha fr: full coverage, reviewed")
	assert.Equal(t, 1, ship.AIShippable, "Alpha de: full coverage, machine-reviewed only")
	assert.Equal(t, 1, ship.Pending, "Beta fr: untranslated")
	assert.Equal(t, 2, ship.CountedProjects)
	assert.Equal(t, 2, ship.TotalProjects)
	require.Len(t, ship.Projects, 2)
	assert.Equal(t, pidB, ship.Projects[0].ProjectID, "most-pending project leads for the deep link")
	assert.Equal(t, "main", ship.Projects[0].Stream)
	assert.Equal(t, 1, ship.Projects[0].Pending)

	// Partial coverage: dropping Alpha's cache entry (as any content change
	// would) shrinks the fold to Beta and says so via counted vs total.
	srv.invalidateDashboardCache("test-ws", pidA)
	resp = getLoopRollup(t, srv, token)
	require.NotNil(t, resp.Ship)
	assert.Equal(t, 1, resp.Ship.CountedProjects)
	assert.Equal(t, 2, resp.Ship.TotalProjects)
	assert.Equal(t, 0, resp.Ship.Governed)
	assert.Equal(t, 1, resp.Ship.Pending)

	// Ship-state fold ignores rollups past the staleness grace: age Beta's
	// entry beyond the window and the ship block disappears entirely.
	key := dashboardCacheKey("test-ws", pidB, "main")
	cached, ok := srv.dashboardCache.Load(key)
	require.True(t, ok)
	entry := cached.(*dashboardCacheEntry)
	srv.dashboardCache.Store(key, &dashboardCacheEntry{
		stats:     entry.stats,
		expiresAt: time.Now().Add(-loopRollupCacheGrace - time.Minute),
	})
	resp = getLoopRollup(t, srv, token)
	assert.Nil(t, resp.Ship)
}
