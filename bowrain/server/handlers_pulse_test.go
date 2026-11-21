package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Surface gating
// ---------------------------------------------------------------------------

// TestPulseRoutes_UnmountedByDefault verifies the public Pulse surface is not
// mounted unless Config.PulseEnabled (BOWRAIN_PULSE_ENABLED) is set. With the
// dedicated pulse routes gone, the paths fall through to the JWT-guarded
// workspace tree (`/api/v1/:ws/…` with ws="pulse") and return 401 to an anonymous caller.
func TestPulseRoutes_UnmountedByDefault(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := shutdownOnCleanup(t, NewServer(cfg))
	e := srv.GetEcho()

	for _, path := range []string{
		"/api/v1/pulse",
		"/api/v1/pulse/some-workspace",
		"/api/v1/pulse/some-workspace/leaderboard",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code,
			"pulse route %s must not be publicly reachable by default", path)
	}
}

// TestPulseRoutes_MountedWhenEnabled verifies the explicit opt-in re-mounts the
// surface: the front page responds and workspace routes reach the access
// middleware instead of 404ing.
func TestPulseRoutes_MountedWhenEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	cfg.PulseEnabled = true
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pulse", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code,
		"pulse front page must be mounted when PulseEnabled is set")
}

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
		Type:                platauth.WorkspaceTypeTeam,
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
		Type:                platauth.WorkspaceTypeTeam,
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

func TestPulseAccessMiddleware_PersonalWorkspace(t *testing.T) {
	token, err := platauth.GenerateToken(&platauth.User{ID: "owner"}, "secret", time.Hour)
	require.NoError(t, err)
	for _, visibility := range []platauth.DashboardVisibility{platauth.DashboardPublic, platauth.DashboardUnlisted} {
		t.Run(string(visibility), func(t *testing.T) {
			ws := &platauth.Workspace{
				ID: "ws", Slug: "personal", Type: platauth.WorkspaceTypePersonal,
				DashboardVisibility: visibility, PulseAccessKey: "dashboard-key",
			}
			api := echo.New()
			api.GET("/:workspace", func(c echo.Context) error {
				return pulseJSON(c, 60, "dashboard")
			}, PulseAccessMiddleware("secret", &mockPulseAuth{workspace: ws, membership: &platauth.Membership{}}))
			for _, path := range []string{"/personal", "/dashboard-key"} {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				rec := httptest.NewRecorder()
				api.ServeHTTP(rec, req)
				assert.Equal(t, http.StatusNotFound, rec.Code)
				req.Header.Set("Authorization", "Bearer "+token)
				rec = httptest.NewRecorder()
				api.ServeHTTP(rec, req)
				assert.Equal(t, http.StatusOK, rec.Code)
				assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
			}
		})
	}
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
// Handler integration tests
// ---------------------------------------------------------------------------

func newPulseTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := DefaultConfig()
	cfg.JWTSecret = "secret"
	srv := shutdownOnCleanup(t, NewServer(cfg))
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
	ctx := t.Context()

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

func TestPulseProjectPublication(t *testing.T) {
	srv := newPulseTestServer(t)
	ctx := t.Context()
	ws := &platauth.Workspace{ID: "ws", Slug: "team", Type: platauth.WorkspaceTypeTeam, DashboardVisibility: platauth.DashboardPublic}
	api := echo.New()
	g := api.Group("/pulse/:workspace", PulseAccessMiddleware("secret", &mockPulseAuth{workspace: ws}))
	g.GET("/projects", srv.HandlePulseProjects)
	g.GET("/projects/:id", srv.HandlePulseProjectDetail, PulseProjectAccessMiddleware(srv.ContentStore))
	for _, tc := range []struct {
		id, workspace, visibility string
		archive                   bool
		want                      int
	}{
		{"public", "ws", "public", false, http.StatusOK},
		{"unlisted", "ws", "unlisted", false, http.StatusOK},
		{"private", "ws", "private", false, http.StatusNotFound},
		{"other", "another", "public", false, http.StatusNotFound},
		{"archived", "ws", "public", true, http.StatusNotFound},
		{"unknown", "ws", "unknown", false, http.StatusNotFound},
	} {
		require.NoError(t, srv.ContentStore.CreateProject(ctx, &store.Project{
			ID: tc.id, Name: tc.id, WorkspaceID: tc.workspace, DashboardVisibility: tc.visibility,
		}))
		if tc.archive {
			require.NoError(t, srv.ContentStore.ArchiveProject(ctx, tc.id))
		}
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/pulse/team/projects/"+tc.id, nil))
		assert.Equal(t, tc.want, rec.Code, tc.id)
	}
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/pulse/team/projects", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct{ Projects []store.PulseProjectSummary }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	var ids []string
	for _, p := range body.Projects {
		ids = append(ids, p.ID)
	}
	assert.ElementsMatch(t, []string{"public", "unlisted"}, ids)
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

func TestHandlePulseOverview_CacheHeaders(t *testing.T) {
	for _, tc := range []struct {
		visibility platauth.DashboardVisibility
		want       string
	}{
		{platauth.DashboardPublic, "public, max-age=60"},
		{platauth.DashboardPrivate, "private, no-store"},
	} {
		t.Run(string(tc.visibility), func(t *testing.T) {
			srv := newPulseTestServer(t)
			ws := &platauth.Workspace{ID: "ws", Slug: "team", Type: platauth.WorkspaceTypeTeam, DashboardVisibility: tc.visibility}
			for range 2 {
				c, rec := pulseContext(echo.New(), http.MethodGet, "/pulse/team", ws)
				require.NoError(t, srv.HandlePulseOverview(c))
				assert.Equal(t, tc.want, rec.Header().Get("Cache-Control"))
			}
		})
	}
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
		Workspaces []any                  `json:"workspaces"`
		Stats      store.PulseGlobalStats `json:"stats"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp.Workspaces)
}

func TestHandlePulseFrontPage_WithPublicWorkspace(t *testing.T) {
	srv := newPulseTestServer(t)
	ctx := t.Context()

	// Create a public workspace.
	ws := &platauth.Workspace{
		Name:                "Public WS",
		Type:                platauth.WorkspaceTypeTeam,
		Slug:                "public-ws",
		DashboardVisibility: platauth.DashboardPublic,
	}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))

	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, &platauth.Workspace{
		Name: "Personal", Slug: "personal", Type: platauth.WorkspaceTypePersonal, DashboardVisibility: platauth.DashboardPublic,
	}))

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

// TestPulseActivityVisibility checks that the activity feed and the heatmap
// of a public workspace show only the projects Pulse lists, whoever asks. A
// private or archived project's id in ?project= answers as an unknown id
// does.
func TestPulseActivityVisibility(t *testing.T) {
	srv := newPulseTestServer(t)
	ctx := t.Context()
	srv.ActivityStore = bstore.NewActivityStore(srv.ContentStore.(*bstore.PostgresStore).SQLDB())
	ws := &platauth.Workspace{ID: "ws-vis", Slug: "vis", Type: platauth.WorkspaceTypeTeam, DashboardVisibility: platauth.DashboardPublic}

	for _, p := range []struct {
		id, visibility string
		archive        bool
	}{
		{"p-public", "public", false},
		{"p-private", "private", false},
		{"p-archived", "public", true},
	} {
		require.NoError(t, srv.ContentStore.CreateProject(ctx, &store.Project{
			ID: p.id, Name: p.id, WorkspaceID: ws.ID, DashboardVisibility: p.visibility,
		}))
		if p.archive {
			require.NoError(t, srv.ContentStore.ArchiveProject(ctx, p.id))
		}
		require.NoError(t, srv.ActivityStore.Create(ctx, &bstore.Activity{
			WorkspaceID: ws.Slug, ProjectID: p.id, ActorID: "u", ActorName: "U",
			Type: bstore.ActivityItemPushed, Summary: p.id,
		}))
	}

	token, err := platauth.GenerateToken(&platauth.User{ID: "u-1", Email: "u@example.com"}, "secret", time.Hour)
	require.NoError(t, err)
	callers := []struct {
		name   string
		auth   *mockPulseAuth
		bearer string
	}{
		{"anonymous", &mockPulseAuth{workspace: ws}, ""},
		{"member", &mockPulseAuth{workspace: ws, membership: &platauth.Membership{WorkspaceID: ws.ID, UserID: "u-1"}}, token},
		{"non-member", &mockPulseAuth{workspace: ws}, token},
	}
	requests := []struct {
		name, query string
		want        []string
	}{
		{"feed", "", []string{"p-public"}},
		{"public project", "?project=p-public", []string{"p-public"}},
		{"private project", "?project=p-private", nil},
		{"archived project", "?project=p-archived", nil},
		{"unknown project", "?project=p-none", nil},
	}

	for _, caller := range callers {
		api := echo.New()
		g := api.Group("/pulse/:workspace", PulseAccessMiddleware("secret", caller.auth))
		g.GET("/activity", srv.HandlePulseActivity)
		g.GET("/activity/heatmap", srv.HandlePulseActivityHeatmap)
		get := func(path string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if caller.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+caller.bearer)
			}
			rec := httptest.NewRecorder()
			api.ServeHTTP(rec, req)
			return rec
		}

		var unknown *httptest.ResponseRecorder
		for _, tc := range requests {
			t.Run(caller.name+"/"+tc.name, func(t *testing.T) {
				rec := get("/pulse/vis/activity" + tc.query)
				require.Equal(t, http.StatusOK, rec.Code)
				var resp struct {
					Activities []store.PulseActivity `json:"activities"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				var got []string
				for _, a := range resp.Activities {
					got = append(got, a.Project)
				}
				assert.ElementsMatch(t, tc.want, got)
				if tc.name == "unknown project" {
					unknown = rec
				}
			})
		}
		t.Run(caller.name+"/hidden project answers as unknown", func(t *testing.T) {
			require.NotNil(t, unknown)
			for _, id := range []string{"p-private", "p-archived"} {
				rec := get("/pulse/vis/activity?project=" + id)
				assert.Equal(t, unknown.Code, rec.Code, id)
				assert.Equal(t, unknown.Header().Get("Cache-Control"), rec.Header().Get("Cache-Control"), id)
				assert.JSONEq(t, unknown.Body.String(), rec.Body.String(), id)
			}
		})
		t.Run(caller.name+"/heatmap", func(t *testing.T) {
			rec := get("/pulse/vis/activity/heatmap")
			require.Equal(t, http.StatusOK, rec.Code)
			var resp struct {
				Days []store.PulseHeatmapDay `json:"days"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			total := 0
			for _, d := range resp.Days {
				total += d.Count
			}
			assert.Equal(t, 1, total)
		})
	}
}
