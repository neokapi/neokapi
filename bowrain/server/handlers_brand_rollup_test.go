package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Pure aggregation helpers (no store; always run)
// ---------------------------------------------------------------------------

func score(s int, dims ...corebrand.DimensionScore) *corebrand.StoredScore {
	return &corebrand.StoredScore{Score: s, Dimensions: dims}
}

func TestRollupAverageScore(t *testing.T) {
	assert.Equal(t, 0, rollupAverageScore(nil))
	assert.Equal(t, 85, rollupAverageScore([]*corebrand.StoredScore{score(80), score(90)}))
	// 81 + 82 = 163 / 2 = 81.5 → rounds to 82.
	assert.Equal(t, 82, rollupAverageScore([]*corebrand.StoredScore{score(81), score(82)}))
}

func TestRollupAggregateDimensions(t *testing.T) {
	dims := rollupAggregateDimensions([]*corebrand.StoredScore{
		score(0,
			corebrand.DimensionScore{Dimension: corebrand.DimensionVocabulary, Score: 70, Penalty: 10, Issues: 2},
			corebrand.DimensionScore{Dimension: corebrand.DimensionTone, Score: 90, Penalty: 0, Issues: 0},
		),
		score(0,
			corebrand.DimensionScore{Dimension: corebrand.DimensionVocabulary, Score: 90, Penalty: 4, Issues: 1},
			corebrand.DimensionScore{Dimension: corebrand.DimensionTone, Score: 80, Penalty: 2, Issues: 1},
		),
	})
	// Canonical order: tone before vocabulary.
	require.Len(t, dims, 2)
	assert.Equal(t, corebrand.DimensionTone, dims[0].Dimension)
	assert.Equal(t, corebrand.DimensionVocabulary, dims[1].Dimension)
	// vocabulary: mean(70,90)=80, issues 2+1=3.
	assert.Equal(t, 80, dims[1].Score)
	assert.Equal(t, 3, dims[1].Issues)
	assert.Nil(t, rollupAggregateDimensions(nil))
}

func TestRollupTrend(t *testing.T) {
	assert.Equal(t, "flat", rollupTrend(corebrand.DriftResult{RecentCount: 0, RecentAvg: 0, BaselineAvg: 90}))
	assert.Equal(t, "up", rollupTrend(corebrand.DriftResult{RecentCount: 5, RecentAvg: 85, BaselineAvg: 80}))
	assert.Equal(t, "down", rollupTrend(corebrand.DriftResult{RecentCount: 5, RecentAvg: 74, BaselineAvg: 82}))
	assert.Equal(t, "flat", rollupTrend(corebrand.DriftResult{RecentCount: 5, RecentAvg: 80.4, BaselineAvg: 80}))
}

func TestRollupPaginateProjects(t *testing.T) {
	projects := []*platstore.Project{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	assert.Len(t, paginateProjects(projects, 0, 2), 2)
	assert.Equal(t, "c", paginateProjects(projects, 2, 10)[0].ID)
	assert.Empty(t, paginateProjects(projects, 5, 2))
}

func TestRollupPageParams(t *testing.T) {
	e := echo.New()
	params := func(q string) (int, int) {
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/?"+q, nil), httptest.NewRecorder())
		return rollupPageParams(c)
	}
	limit, offset := params("")
	assert.Equal(t, defaultRollupLimit, limit)
	assert.Equal(t, 0, offset)

	limit, offset = params("limit=5&offset=10")
	assert.Equal(t, 5, limit)
	assert.Equal(t, 10, offset)

	limit, _ = params("limit=9999")
	assert.Equal(t, maxRollupLimit, limit) // clamped

	limit, offset = params("limit=-3&offset=-3")
	assert.Equal(t, defaultRollupLimit, limit) // invalid ignored
	assert.Equal(t, 0, offset)
}

func TestLatestCheckedAt(t *testing.T) {
	assert.Nil(t, latestCheckedAt(nil))
	t1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	got := latestCheckedAt([]*corebrand.StoredScore{{CheckedAt: t1}, {CheckedAt: t2}, {CheckedAt: t1}})
	require.NotNil(t, got)
	assert.True(t, got.Equal(t2))
}

// ---------------------------------------------------------------------------
// Handler-level: aggregation over seeded scores + workspace isolation.
// Container-gated (initTestStores → pgtest, skipped in -short).
// ---------------------------------------------------------------------------

func TestBrandRollup_AggregatesAndIsolates(t *testing.T) {
	srv := setupBrandLoopServer(t)
	e := srv.GetEcho()
	ctx := context.Background()

	const wsA = "ws-rollup-a"
	const wsB = "ws-rollup-b"

	mkProject := func(name, ws string) *platstore.Project {
		p := &platstore.Project{Name: name, WorkspaceID: ws, DefaultSourceLanguage: "en-US"}
		require.NoError(t, srv.Services.Project.CreateProject(ctx, p))
		return p
	}
	alpha := mkProject("Alpha", wsA)     // scored
	mkProject("Bravo", wsA)              // never scored — asserted via the response
	charlie := mkProject("Charlie", wsB) // other workspace — must not leak

	storeScore := func(projectID string, s int, at time.Time, dims ...corebrand.DimensionScore) {
		require.NoError(t, srv.BrandStore.StoreScore(ctx, &corebrand.StoredScore{
			ProjectID: projectID,
			BlockID:   id.New(),
			ProfileID: "vp-1",
			Locale:    "",
			Score:     s,
			Dimensions: dims,
			CheckedAt: at,
		}))
	}
	now := time.Now().UTC()
	storeScore(alpha.ID, 80, now,
		corebrand.DimensionScore{Dimension: corebrand.DimensionVocabulary, Score: 70, Penalty: 30, Issues: 2},
		corebrand.DimensionScore{Dimension: corebrand.DimensionTone, Score: 90, Penalty: 10, Issues: 0},
	)
	storeScore(alpha.ID, 90, now,
		corebrand.DimensionScore{Dimension: corebrand.DimensionVocabulary, Score: 90, Penalty: 10, Issues: 1},
		corebrand.DimensionScore{Dimension: corebrand.DimensionTone, Score: 80, Penalty: 20, Issues: 1},
	)
	storeScore(charlie.ID, 40, now) // wsB — should never appear in a wsA rollup

	call := func(ws, query string) BrandRollupResponse {
		req := httptest.NewRequest(http.MethodGet, "/?"+query, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("workspace_id", ws)
		require.NoError(t, srv.HandleGetBrandVoiceRollup(c))
		require.Equal(t, http.StatusOK, rec.Code)
		var resp BrandRollupResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return resp
	}

	resp := call(wsA, "days=30")
	// Only wsA's projects, ordered by name; Charlie (wsB) is isolated out.
	require.Len(t, resp.Projects, 2)
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, "Alpha", resp.Projects[0].ProjectName)
	assert.Equal(t, "Bravo", resp.Projects[1].ProjectName)
	for _, p := range resp.Projects {
		assert.NotEqual(t, "Charlie", p.ProjectName)
	}

	// Alpha: overall mean(80,90)=85, 2 blocks, dims aggregated in canonical order.
	al := resp.Projects[0]
	require.NotNil(t, al.Overall)
	assert.Equal(t, 85, *al.Overall)
	assert.Equal(t, 2, al.ScoredBlocks)
	require.NotNil(t, al.LastScoredAt)
	require.Len(t, al.Dimensions, 2)
	assert.Equal(t, corebrand.DimensionTone, al.Dimensions[0].Dimension)
	assert.Equal(t, corebrand.DimensionVocabulary, al.Dimensions[1].Dimension)
	assert.Equal(t, 80, al.Dimensions[1].Score) // mean(70,90)
	assert.Equal(t, 3, al.Dimensions[1].Issues) // 2+1
	assert.Equal(t, "flat", al.Trend)           // single date → no baseline movement

	// Bravo: never scored → null overall, no activity, unknown trend.
	br := resp.Projects[1]
	assert.Nil(t, br.Overall)
	assert.Equal(t, 0, br.ScoredBlocks)
	assert.Nil(t, br.LastScoredAt)
	assert.Empty(t, br.Trend)

	// Pagination: first page of one is Alpha, total still 2.
	page := call(wsA, "limit=1")
	require.Len(t, page.Projects, 1)
	assert.Equal(t, "Alpha", page.Projects[0].ProjectName)
	assert.Equal(t, 2, page.Total)
	assert.Equal(t, 1, page.Limit)

	// wsB sees only Charlie.
	other := call(wsB, "")
	require.Len(t, other.Projects, 1)
	assert.Equal(t, "Charlie", other.Projects[0].ProjectName)
}
