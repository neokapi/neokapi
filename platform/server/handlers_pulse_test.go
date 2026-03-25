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
// Project access middleware tests
// ---------------------------------------------------------------------------

func TestPulseProjectAccessMiddleware_PrivateProject(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.JWTSecret = "secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	ctx := context.Background()
	proj := &store.Project{
		ID:                  "p-1",
		Name:                "Private Project",
		WorkspaceID:         "ws-1",
		DashboardVisibility: "private",
	}
	require.NoError(t, srv.ContentStore.CreateProject(ctx, proj))

	ws := &platauth.Workspace{
		ID:   "ws-1",
		Name: "Test WS",
		Slug: "test-ws",
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pulse/test-ws/projects/p-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("workspace", "pid")
	c.SetParamValues("test-ws", "p-1")
	c.Set("pulse_workspace", ws)

	mw := PulseProjectAccessMiddleware(srv.ContentStore)
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPulseProjectAccessMiddleware_PublicProject(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.JWTSecret = "secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	ctx := context.Background()
	proj := &store.Project{
		ID:                  "p-1",
		Name:                "Public Project",
		WorkspaceID:         "ws-1",
		DashboardVisibility: "public",
	}
	require.NoError(t, srv.ContentStore.CreateProject(ctx, proj))

	ws := &platauth.Workspace{
		ID:   "ws-1",
		Name: "Test WS",
		Slug: "test-ws",
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pulse/test-ws/projects/p-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("workspace", "pid")
	c.SetParamValues("test-ws", "p-1")
	c.Set("pulse_workspace", ws)

	mw := PulseProjectAccessMiddleware(srv.ContentStore)
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPulseProjectAccessMiddleware_WrongWorkspace(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.JWTSecret = "secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	ctx := context.Background()
	proj := &store.Project{
		ID:                  "p-1",
		Name:                "Project",
		WorkspaceID:         "ws-other",
		DashboardVisibility: "public",
	}
	require.NoError(t, srv.ContentStore.CreateProject(ctx, proj))

	ws := &platauth.Workspace{
		ID:   "ws-1",
		Name: "Test WS",
		Slug: "test-ws",
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pulse/test-ws/projects/p-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("workspace", "pid")
	c.SetParamValues("test-ws", "p-1")
	c.Set("pulse_workspace", ws)

	mw := PulseProjectAccessMiddleware(srv.ContentStore)
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------------------------------------------------------------------------
// Handler integration tests
// ---------------------------------------------------------------------------

func newPulseTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := DefaultServerConfig()
	cfg.JWTSecret = "secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	return srv
}

func pulseContext(e *echo.Echo, method, path string, ws *platauth.Workspace) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("workspace")
	c.SetParamValues(ws.Slug)
	c.Set("pulse_workspace", ws)
	c.Set("pulse_workspace_id", ws.ID)
	return c, rec
}

func TestHandlePulseOverview_EmptyWorkspace(t *testing.T) {
	srv := newPulseTestServer(t)
	ws := &platauth.Workspace{ID: "ws-1", Name: "Test", Slug: "test"}

	e := echo.New()
	c, rec := pulseContext(e, http.MethodGet, "/api/v1/pulse/test", ws)

	err := srv.HandlePulseOverview(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var overview store.PulseOverview
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &overview))
	assert.Equal(t, "Test", overview.Workspace.Name)
	assert.Equal(t, 0, overview.Stats.TotalProjects)
	assert.Empty(t, overview.Projects)
}

func TestHandlePulseOverview_WithProjects(t *testing.T) {
	srv := newPulseTestServer(t)
	ctx := context.Background()

	// Create one public and one private project.
	require.NoError(t, srv.ContentStore.CreateProject(ctx, &store.Project{
		ID:                  "p-pub",
		Name:                "Public",
		WorkspaceID:         "ws-1",
		DashboardVisibility: "public",
	}))
	require.NoError(t, srv.ContentStore.CreateProject(ctx, &store.Project{
		ID:                  "p-priv",
		Name:                "Private",
		WorkspaceID:         "ws-1",
		DashboardVisibility: "private",
	}))

	ws := &platauth.Workspace{ID: "ws-1", Name: "Test", Slug: "test"}
	e := echo.New()
	c, rec := pulseContext(e, http.MethodGet, "/api/v1/pulse/test", ws)

	err := srv.HandlePulseOverview(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var overview store.PulseOverview
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &overview))
	assert.Equal(t, 1, overview.Stats.TotalProjects, "private project should be excluded")
	require.Len(t, overview.Projects, 1)
	assert.Equal(t, "Public", overview.Projects[0].Name)
}

func TestHandlePulseProjects_FiltersPrivate(t *testing.T) {
	srv := newPulseTestServer(t)
	ctx := context.Background()

	require.NoError(t, srv.ContentStore.CreateProject(ctx, &store.Project{
		ID: "p-1", Name: "Pub", WorkspaceID: "ws-1", DashboardVisibility: "public",
	}))
	require.NoError(t, srv.ContentStore.CreateProject(ctx, &store.Project{
		ID: "p-2", Name: "Unlisted", WorkspaceID: "ws-1", DashboardVisibility: "unlisted",
	}))
	require.NoError(t, srv.ContentStore.CreateProject(ctx, &store.Project{
		ID: "p-3", Name: "Private", WorkspaceID: "ws-1", DashboardVisibility: "private",
	}))

	ws := &platauth.Workspace{ID: "ws-1", Name: "Test", Slug: "test"}
	e := echo.New()
	c, rec := pulseContext(e, http.MethodGet, "/api/v1/pulse/test/projects", ws)

	err := srv.HandlePulseProjects(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Projects []store.PulseProjectSummary `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Projects, 2, "private project excluded, public+unlisted included")
}

func TestHandlePulseLeaderboard(t *testing.T) {
	srv := newPulseTestServer(t)
	ws := &platauth.Workspace{ID: "ws-1", Name: "Test", Slug: "test"}

	e := echo.New()
	c, rec := pulseContext(e, http.MethodGet, "/api/v1/pulse/test/leaderboard", ws)

	err := srv.HandlePulseLeaderboard(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var lb store.PulseLeaderboard
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lb))
	assert.NotNil(t, lb.Contributors)
	assert.NotNil(t, lb.Languages)
}

func TestHandlePulseActivity(t *testing.T) {
	srv := newPulseTestServer(t)
	ws := &platauth.Workspace{ID: "ws-1", Name: "Test", Slug: "test"}

	e := echo.New()
	c, rec := pulseContext(e, http.MethodGet, "/api/v1/pulse/test/activity", ws)

	err := srv.HandlePulseActivity(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Activities []store.PulseActivity `json:"activities"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Activities)
}

func TestHandlePulseTerms(t *testing.T) {
	srv := newPulseTestServer(t)
	ws := &platauth.Workspace{ID: "ws-1", Name: "Test", Slug: "test"}

	e := echo.New()
	c, rec := pulseContext(e, http.MethodGet, "/api/v1/pulse/test/terms", ws)

	err := srv.HandlePulseTerms(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Terms []store.PulseTermEntry `json:"terms"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Terms)
}

func TestHandlePulseOverview_CDNCacheHeaders(t *testing.T) {
	srv := newPulseTestServer(t)
	ws := &platauth.Workspace{ID: "ws-1", Name: "Test", Slug: "test"}

	e := echo.New()
	c, rec := pulseContext(e, http.MethodGet, "/api/v1/pulse/test", ws)

	err := srv.HandlePulseOverview(c)
	require.NoError(t, err)
	assert.Contains(t, rec.Header().Get("Cache-Control"), "public")
}

func TestHandlePulseOverview_NoWorkspace(t *testing.T) {
	srv := newPulseTestServer(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pulse/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	// No pulse_workspace set on context.

	err := srv.HandlePulseOverview(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandlePulseActivityHeatmap(t *testing.T) {
	srv := newPulseTestServer(t)
	ws := &platauth.Workspace{ID: "ws-1", Name: "Test", Slug: "test"}

	e := echo.New()
	c, rec := pulseContext(e, http.MethodGet, "/api/v1/pulse/test/activity/heatmap", ws)

	err := srv.HandlePulseActivityHeatmap(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Days []store.PulseHeatmapDay `json:"days"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Days)
}

func TestHandlePulseFrontPage(t *testing.T) {
	srv := newPulseTestServer(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pulse", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := srv.HandlePulseFrontPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Workspaces []any                `json:"workspaces"`
		Stats      store.PulseGlobalStats `json:"stats"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Workspaces)
}

func TestHandlePulseFrontPage_WithPublicWorkspace(t *testing.T) {
	srv := newPulseTestServer(t)
	ctx := context.Background()

	// Create a public workspace.
	ws := &platauth.Workspace{
		Name:                "Public WS",
		Slug:                "public-ws",
		DashboardVisibility: platauth.DashboardPublic,
	}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))

	// Create a public project in it.
	require.NoError(t, srv.ContentStore.CreateProject(ctx, &store.Project{
		ID:                  "p-1",
		Name:                "My Project",
		WorkspaceID:         ws.ID,
		DashboardVisibility: "public",
	}))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pulse", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := srv.HandlePulseFrontPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Workspaces []struct {
			Slug     string `json:"slug"`
			Projects int    `json:"projects"`
		} `json:"workspaces"`
		Stats store.PulseGlobalStats `json:"stats"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Workspaces, 1)
	assert.Equal(t, "public-ws", resp.Workspaces[0].Slug)
	assert.Equal(t, 1, resp.Workspaces[0].Projects)
	assert.Equal(t, 1, resp.Stats.TotalProjects)
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

func (m *mockPulseAuth) GetWorkspaceByAccessKey(_ context.Context, key string) (*platauth.Workspace, error) {
	if m.workspace != nil && m.workspace.PulseAccessKey != "" && m.workspace.PulseAccessKey == key {
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
