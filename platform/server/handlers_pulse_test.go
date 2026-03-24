package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/platform/auth"
	"github.com/neokapi/neokapi/platform/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Middleware tests
// ---------------------------------------------------------------------------

func TestPulseAccessMiddleware_PrivateWorkspace(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pulse/my-workspace", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("workspace")
	c.SetParamValues("my-workspace")

	// No workspace found → 404.
	mw := PulseAccessMiddleware("secret", &mockPulseAuth{})
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPulseAccessMiddleware_PublicWorkspace(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pulse/my-workspace", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("workspace")
	c.SetParamValues("my-workspace")

	ws := &platauth.Workspace{
		ID:                  "ws-1",
		Name:                "My Workspace",
		Slug:                "my-workspace",
		DashboardVisibility: platauth.DashboardPublic,
	}

	mw := PulseAccessMiddleware("secret", &mockPulseAuth{workspace: ws})
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("X-Robots-Tag"))
}

func TestPulseAccessMiddleware_UnlistedWorkspace(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pulse/my-workspace", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("workspace")
	c.SetParamValues("my-workspace")

	ws := &platauth.Workspace{
		ID:                  "ws-1",
		Name:                "My Workspace",
		Slug:                "my-workspace",
		DashboardVisibility: platauth.DashboardUnlisted,
	}

	mw := PulseAccessMiddleware("secret", &mockPulseAuth{workspace: ws})
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "noindex", rec.Header().Get("X-Robots-Tag"))
}

// ---------------------------------------------------------------------------
// Cache tests
// ---------------------------------------------------------------------------

func TestPulseCache_SetAndGet(t *testing.T) {
	c := newPulseCache()
	c.Set("key", "overview", "data")

	val, ok := c.Get("key")
	assert.True(t, ok)
	assert.Equal(t, "data", val)
}

func TestPulseCache_InvalidateWorkspace(t *testing.T) {
	c := newPulseCache()
	c.Set(pulseCacheKey("ws-1", "overview", ""), "overview", "data1")
	c.Set(pulseCacheKey("ws-1", "leaderboard", ""), "leaderboard", "data2")
	c.Set(pulseCacheKey("ws-2", "overview", ""), "overview", "data3")

	c.InvalidateWorkspace("ws-1")

	_, ok := c.Get(pulseCacheKey("ws-1", "overview", ""))
	assert.False(t, ok)
	_, ok = c.Get(pulseCacheKey("ws-1", "leaderboard", ""))
	assert.False(t, ok)

	// ws-2 should be unaffected.
	val, ok := c.Get(pulseCacheKey("ws-2", "overview", ""))
	assert.True(t, ok)
	assert.Equal(t, "data3", val)
}

func TestPulseCache_InvalidateProject(t *testing.T) {
	c := newPulseCache()
	c.Set(pulseCacheKey("ws-1", "project", "p-1"), "project", "data1")
	c.Set(pulseCacheKey("ws-1", "overview", ""), "overview", "data2")

	c.InvalidateProject("ws-1", "p-1")

	_, ok := c.Get(pulseCacheKey("ws-1", "project", "p-1"))
	assert.False(t, ok)
	// Overview should also be invalidated.
	_, ok = c.Get(pulseCacheKey("ws-1", "overview", ""))
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// Type serialization tests
// ---------------------------------------------------------------------------

func TestPulseOverview_JSON(t *testing.T) {
	overview := store.PulseOverview{
		Workspace: store.PulseWorkspaceInfo{
			Name: "Test",
			Slug: "test",
		},
		Projects:       []store.PulseProjectSummary{},
		TopLanguages:   []store.PulseLanguageRank{},
		TopContribs:    []store.PulseContributor{},
		RisingStars:    []store.PulseRisingStar{},
		RecentActivity: []store.PulseActivity{},
		Stats: store.PulseGlobalStats{
			TotalProjects:  2,
			TotalLanguages: 3,
			OverallPercent: 75.5,
		},
	}

	data, err := json.Marshal(overview)
	require.NoError(t, err)

	var decoded store.PulseOverview
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "Test", decoded.Workspace.Name)
	assert.Equal(t, "test", decoded.Workspace.Slug)
	assert.Equal(t, 2, decoded.Stats.TotalProjects)
	assert.InDelta(t, 75.5, decoded.Stats.OverallPercent, 0.01)
}

func TestDashboardVisibility_Defaults(t *testing.T) {
	assert.True(t, platauth.ValidDashboardVisibility[platauth.DashboardPrivate])
	assert.True(t, platauth.ValidDashboardVisibility[platauth.DashboardUnlisted])
	assert.True(t, platauth.ValidDashboardVisibility[platauth.DashboardPublic])
	assert.False(t, platauth.ValidDashboardVisibility["invalid"])
}

// ---------------------------------------------------------------------------
// Mock auth store
// ---------------------------------------------------------------------------

type mockPulseAuth struct {
	auth.AuthStore
	workspace  *platauth.Workspace
	membership *platauth.Membership
}

func (m *mockPulseAuth) GetWorkspaceBySlug(_ context.Context, slug string) (*platauth.Workspace, error) {
	if m.workspace != nil && m.workspace.Slug == slug {
		return m.workspace, nil
	}
	return nil, assert.AnError
}

func (m *mockPulseAuth) GetMembership(_ context.Context, workspaceID, userID string) (*platauth.Membership, error) {
	if m.membership != nil {
		return m.membership, nil
	}
	return nil, assert.AnError
}
