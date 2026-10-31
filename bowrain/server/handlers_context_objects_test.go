package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/core/workspace/workspacetest"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/contextremote"
)

// contextRemote opens the client remote kapi syncs through on the project
// at <server>/<ws>/<pid>, signed in with token.
func contextRemote(t *testing.T, server, ws, pid, token string) workspace.Remote {
	t.Helper()
	r, err := contextremote.New(contextremote.Options{URL: server + "/" + ws + "/" + pid, Token: token})
	require.NoError(t, err)
	return r
}

// rawRequest sends one request to the routes outside the client, for the
// answers the client maps away. It returns the status and the body.
func rawRequest(t *testing.T, method, target, token string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, target, bytes.NewReader(body))
	require.NoError(t, err)
	if body != nil {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	if token != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, data
}

// contextObjectSQLiteServer serves the context object routes over a SQLite
// content store on a local listener, behind the server's body limit and a
// stand-in for the auth middleware that signs every request in as member with
// perms.
func contextObjectSQLiteServer(t *testing.T, cs *sqlitestore.SQLiteStore, member string, perms platauth.Permission) string {
	t.Helper()
	srv := &Server{ContentStore: cs, wsStores: newWorkspaceStores()}
	e := echo.New()
	e.Use(middleware.BodyLimit(requestBodyLimit))
	g := e.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", member)
			c.Set("project_permissions", perms)
			return next(c)
		}
	})
	srv.registerContextObjectRoutes(g)
	hs := httptest.NewServer(e)
	t.Cleanup(hs.Close)
	return hs.URL
}

func newContextObjectSQLiteStore(t *testing.T) *sqlitestore.SQLiteStore {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func seedContextObjectProject(t *testing.T, cs platstore.ContentStore) string {
	t.Helper()
	p := &platstore.Project{
		Name: "ctx", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"},
		WorkspaceID: "ws", Properties: map[string]string{},
	}
	require.NoError(t, cs.CreateProject(t.Context(), p))
	return p.ID
}

// The routes over the SQLite store, read and written through the client
// remote kapi syncs with, pass every behaviour a remote owes the sync engine.
// Each case gets a fresh project, and each opener another client.
func TestContextObjectRoutes_RemoteConformance_SQLite(t *testing.T) {
	workspacetest.RunRemoteConformance(t, func(t *testing.T) func(t *testing.T) workspace.Remote {
		cs := newContextObjectSQLiteStore(t)
		server := contextObjectSQLiteServer(t, cs, "member-a", platauth.PermViewContent|contextObjectWritePermission)
		pid := seedContextObjectProject(t, cs)
		return func(t *testing.T) workspace.Remote { return contextRemote(t, server, "ws", pid, "unused") }
	})
}

// A push stamps each new object with the signed-in member; a reader without
// the write permission reads and cannot push.
func TestContextObjectRoutes_PushedByAndPermissions_SQLite(t *testing.T) {
	ctx := t.Context()
	cs := newContextObjectSQLiteStore(t)
	pid := seedContextObjectProject(t, cs)
	writerURL := contextObjectSQLiteServer(t, cs, "member-a", platauth.PermViewContent|contextObjectWritePermission)
	name := workspace.SegmentName("w1", "000000000000000000000001")
	writer := contextRemote(t, writerURL, "ws", pid, "unused")
	require.NoError(t, writer.Put(ctx, workspace.Object{Name: name, Data: []byte("one\n")}))
	obj, err := cs.GetContextObject(ctx, pid, name)
	require.NoError(t, err)
	assert.Equal(t, "member-a", obj.PushedBy)

	readerURL := contextObjectSQLiteServer(t, cs, "viewer", platauth.PermViewContent)
	reader := contextRemote(t, readerURL, "ws", pid, "unused")
	got, err := reader.Get(ctx, name)
	require.NoError(t, err)
	assert.Equal(t, "one\n", string(got))
	err = reader.Put(ctx, workspace.Object{Name: workspace.SegmentName("w2", "000000000000000000000002"), Data: []byte("x\n")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 403")
	require.NotErrorIs(t, err, workspace.ErrRemoteUnreachable, "a refused permission is not an unreachable backend")

	base := writerURL + "/api/v1/ws/" + pid + "/context/objects"
	status, _ := rawRequest(t, http.MethodGet, base+"?dir=log", "", nil)
	assert.Equal(t, http.StatusBadRequest, status, "a directory is named with its slash")
}

// The conflict answer carries the code the client maps to ErrObjectExists.
func TestContextObjectRoutes_ConflictBody_SQLite(t *testing.T) {
	ctx := t.Context()
	cs := newContextObjectSQLiteStore(t)
	server := contextObjectSQLiteServer(t, cs, "member-a", platauth.PermViewContent|contextObjectWritePermission)
	pid := seedContextObjectProject(t, cs)
	r := contextRemote(t, server, "ws", pid, "unused")
	name := workspace.SegmentName("w1", "000000000000000000000001")
	require.NoError(t, r.Put(ctx, workspace.Object{Name: name, Data: []byte("one\n")}))
	err := r.Put(ctx, workspace.Object{Name: name, Data: []byte("two\n")})
	require.ErrorIs(t, err, workspace.ErrObjectExists)

	body, err := json.Marshal(contextObjectBatch{Objects: []apiclient.ContextObjectWire{{Name: name, Data: []byte("two\n")}}})
	require.NoError(t, err)
	status, raw := rawRequest(t, http.MethodPost, server+"/api/v1/ws/"+pid+"/context/objects", "", body)
	require.Equal(t, http.StatusConflict, status)
	var e2 ErrorResponse
	require.NoError(t, json.Unmarshal(raw, &e2))
	assert.Equal(t, "object_exists", e2.Code)
	assert.Equal(t, name, e2.Details, "the body names the refused object alone")
}

// A blob whose name is not the address of its bytes is refused with 400, so
// it can never take the name its real bytes would need.
func TestContextObjectRoutes_BlobNameMustMatchBytes_SQLite(t *testing.T) {
	ctx := t.Context()
	cs := newContextObjectSQLiteStore(t)
	server := contextObjectSQLiteServer(t, cs, "member-a", platauth.PermViewContent|contextObjectWritePermission)
	pid := seedContextObjectProject(t, cs)
	r := contextRemote(t, server, "ws", pid, "unused")
	name := workspace.BlobObjectName(workspace.BlobAddress([]byte("right")))
	body, err := json.Marshal(contextObjectBatch{Objects: []apiclient.ContextObjectWire{{Name: name, Data: []byte("wrong")}}})
	require.NoError(t, err)
	status, raw := rawRequest(t, http.MethodPost, server+"/api/v1/ws/"+pid+"/context/objects", "", body)
	require.Equal(t, http.StatusBadRequest, status)
	var e2 ErrorResponse
	require.NoError(t, json.Unmarshal(raw, &e2))
	assert.Equal(t, "invalid_name", e2.Code)
	assert.Equal(t, name, e2.Details)
	require.NoError(t, r.Put(ctx, workspace.Object{Name: name, Data: []byte("right")}),
		"the blob's real bytes still land under its name")
}

// A push to a project id the store does not hold answers 404, which the
// client reports with the project to check.
func TestContextObjectRoutes_UnknownProject_SQLite(t *testing.T) {
	cs := newContextObjectSQLiteStore(t)
	server := contextObjectSQLiteServer(t, cs, "member-a", platauth.PermViewContent|contextObjectWritePermission)
	r := contextRemote(t, server, "ws", "no-such-project", "unused")
	err := r.Put(t.Context(), workspace.Object{Name: workspace.SegmentName("w1", "000000000000000000000001"), Data: []byte("x\n")})
	require.Error(t, err)
	require.NotErrorIs(t, err, workspace.ErrRemoteUnreachable)
	assert.Contains(t, err.Error(), "holds no project ws/no-such-project")
}

// The routes on the full server, behind its real auth middleware and over
// PostgreSQL, pass the conformance suite through the client remote, and a
// workspace viewer reads and cannot push.
func TestContextObjectRoutes_RemoteConformance_Postgres(t *testing.T) {
	srv, token := newTestServer(t)
	hs := httptest.NewServer(srv.GetEcho())
	t.Cleanup(hs.Close)
	// Projects are stored directly: the free plan's project limit would stop
	// the API's create after three.
	createProject := func(t *testing.T) string {
		p := &platstore.Project{
			Name: "ctx", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"},
			WorkspaceID: "test-ws", Properties: map[string]string{},
		}
		require.NoError(t, srv.ContentStore.CreateProject(t.Context(), p))
		return p.ID
	}

	workspacetest.RunRemoteConformance(t, func(t *testing.T) func(t *testing.T) workspace.Remote {
		pid := createProject(t)
		return func(t *testing.T) workspace.Remote { return contextRemote(t, hs.URL, "test", pid, token) }
	})

	t.Run("pushed by and permissions", func(t *testing.T) {
		ctx := t.Context()
		pid := createProject(t)
		name := workspace.SegmentName("w1", "000000000000000000000001")
		owner := contextRemote(t, hs.URL, "test", pid, token)
		require.NoError(t, owner.Put(ctx, workspace.Object{Name: name, Data: []byte("one\n")}))
		cos, ok := platstore.ContextObjects(srv.ContentStore)
		require.True(t, ok)
		obj, err := cos.GetContextObject(ctx, pid, name)
		require.NoError(t, err)
		assert.Equal(t, "test-user", obj.PushedBy)

		viewer := &platauth.User{ID: "viewer-user", Email: "viewer@example.com", Name: "Viewer"}
		require.NoError(t, srv.AuthStore.CreateUser(ctx, viewer))
		require.NoError(t, srv.AuthStore.AddMember(ctx, "test-ws", viewer.ID, platauth.RoleViewer))
		vt, err := platauth.GenerateToken(viewer, srv.Config.JWTSecret, time.Hour)
		require.NoError(t, err)
		reader := contextRemote(t, hs.URL, "test", pid, vt)
		got, err := reader.Get(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, "one\n", string(got))
		err = reader.Put(ctx, workspace.Object{Name: workspace.SegmentName("w2", "000000000000000000000002"), Data: []byte("x\n")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 403")
	})

	t.Run("a project in another workspace is not found", func(t *testing.T) {
		ctx := t.Context()
		ws := &platauth.Workspace{ID: "other-ws", Name: "Other", Slug: "other", Type: platauth.WorkspaceTypePersonal}
		require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))
		p := &platstore.Project{
			Name: "theirs", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"},
			WorkspaceID: ws.ID, Properties: map[string]string{},
		}
		require.NoError(t, srv.ContentStore.CreateProject(ctx, p))
		status, _ := rawRequest(t, http.MethodGet, hs.URL+"/api/v1/test/"+p.ID+"/context/objects?dir=log/", token, nil)
		assert.Equal(t, http.StatusNotFound, status)
		r := contextRemote(t, hs.URL, "test", p.ID, token)
		err := r.Put(ctx, workspace.Object{Name: workspace.SegmentName("w1", "000000000000000000000001"), Data: []byte("x\n")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 404")
		cos, ok := platstore.ContextObjects(srv.ContentStore)
		require.True(t, ok)
		names, err := cos.ListContextObjects(ctx, p.ID, workspace.RemoteLogDir)
		require.NoError(t, err)
		assert.Empty(t, names, "nothing reached the other workspace's project")
	})
}

// A push whose objects exceed what one request body carries is split across
// requests, and every object lands.
func TestContextObjectRoutes_LargePushLands_SQLite(t *testing.T) {
	ctx := t.Context()
	cs := newContextObjectSQLiteStore(t)
	server := contextObjectSQLiteServer(t, cs, "member-a", platauth.PermViewContent|contextObjectWritePermission)
	pid := seedContextObjectProject(t, cs)
	r := contextRemote(t, server, "ws", pid, "unused")
	var objs []workspace.Object
	for i := range 4 {
		blob := bytes.Repeat([]byte{byte('a' + i)}, 12<<20)
		objs = append(objs, workspace.Object{Name: workspace.BlobObjectName(workspace.BlobAddress(blob)), Data: blob})
	}
	require.NoError(t, r.Put(ctx, objs...))
	names, err := r.List(ctx, workspace.RemoteBlobDir)
	require.NoError(t, err)
	assert.Len(t, names, 4)
	got, err := r.Get(ctx, objs[3].Name)
	require.NoError(t, err)
	assert.Equal(t, objs[3].Data, got)
}
