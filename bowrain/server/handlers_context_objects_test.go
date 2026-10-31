package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/core/workspace/workspacetest"
)

// routeRemote is a workspace.Remote over the context object routes, served in
// process. It maps the routes' 404 and 409 codes to the workspace errors, as a
// client remote does.
type routeRemote struct {
	h      http.Handler
	base   string
	bearer string
}

func (r *routeRemote) Describe() workspace.RemoteDescriptor {
	return workspace.RemoteDescriptor{Kind: "bowrain", Location: r.base}
}

func (r *routeRemote) do(ctx context.Context, method, target string, body []byte) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req := httptest.NewRequestWithContext(ctx, method, target, rd)
	if body != nil {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	if r.bearer != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+r.bearer)
	}
	rec := httptest.NewRecorder()
	r.h.ServeHTTP(rec, req)
	return rec
}

// routeError turns a refusal into the error the workspace names for it.
func routeError(rec *httptest.ResponseRecorder) error {
	var e ErrorResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	switch {
	case rec.Code == http.StatusNotFound && e.Code == contextObjectMissingCode:
		return fmt.Errorf("%w: %s", workspace.ErrNoObject, e.Details)
	case rec.Code == http.StatusConflict && e.Code == contextObjectExistsCode:
		return fmt.Errorf("%w: %s", workspace.ErrObjectExists, e.Details)
	}
	return fmt.Errorf("context objects: status %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
}

func (r *routeRemote) List(ctx context.Context, dir string) ([]string, error) {
	rec := r.do(ctx, http.MethodGet, r.base+"?dir="+url.QueryEscape(dir), nil)
	if rec.Code != http.StatusOK {
		return nil, routeError(rec)
	}
	var out contextObjectList
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		return nil, err
	}
	return out.Names, nil
}

func (r *routeRemote) Get(ctx context.Context, name string) ([]byte, error) {
	rec := r.do(ctx, http.MethodGet, r.base+"/"+name, nil)
	if rec.Code != http.StatusOK {
		return nil, routeError(rec)
	}
	return rec.Body.Bytes(), nil
}

func (r *routeRemote) Put(ctx context.Context, objs ...workspace.Object) error {
	batch := contextObjectBatch{Objects: make([]contextObjectWire, 0, len(objs))}
	for _, o := range objs {
		batch.Objects = append(batch.Objects, contextObjectWire{Name: o.Name, Data: o.Data})
	}
	body, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	rec := r.do(ctx, http.MethodPost, r.base, body)
	if rec.Code != http.StatusNoContent {
		return routeError(rec)
	}
	return nil
}

func (r *routeRemote) Close() error { return nil }

// contextObjectSQLiteServer serves the context object routes over a SQLite
// content store, with a stand-in for the auth middleware that signs every
// request in as member with perms.
func contextObjectSQLiteServer(t *testing.T, member string, perms platauth.Permission) (*echo.Echo, *sqlitestore.SQLiteStore) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	srv := &Server{ContentStore: cs, wsStores: newWorkspaceStores()}
	e := echo.New()
	g := e.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", member)
			c.Set("project_permissions", perms)
			return next(c)
		}
	})
	srv.registerContextObjectRoutes(g)
	return e, cs
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

// The routes over the SQLite store pass every behaviour a remote owes the sync
// engine. Each case gets a fresh project, and each opener another client.
func TestContextObjectRoutes_RemoteConformance_SQLite(t *testing.T) {
	workspacetest.RunRemoteConformance(t, func(t *testing.T) func(t *testing.T) workspace.Remote {
		e, cs := contextObjectSQLiteServer(t, "member-a", platauth.PermViewContent|contextObjectWritePermission)
		base := "/api/v1/ws/" + seedContextObjectProject(t, cs) + "/context/objects"
		return func(*testing.T) workspace.Remote { return &routeRemote{h: e, base: base} }
	})
}

// A push stamps each new object with the signed-in member; a reader without
// the write permission reads and cannot push.
func TestContextObjectRoutes_PushedByAndPermissions_SQLite(t *testing.T) {
	ctx := t.Context()
	e, cs := contextObjectSQLiteServer(t, "member-a", platauth.PermViewContent|contextObjectWritePermission)
	pid := seedContextObjectProject(t, cs)
	base := "/api/v1/ws/" + pid + "/context/objects"
	name := workspace.SegmentName("w1", "000000000000000000000001")
	writer := &routeRemote{h: e, base: base}
	require.NoError(t, writer.Put(ctx, workspace.Object{Name: name, Data: []byte("one\n")}))
	obj, err := cs.GetContextObject(ctx, pid, name)
	require.NoError(t, err)
	assert.Equal(t, "member-a", obj.PushedBy)

	readerEcho := echo.New()
	readerSrv := &Server{ContentStore: cs, wsStores: newWorkspaceStores()}
	readerSrv.registerContextObjectRoutes(readerEcho.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", "viewer")
			c.Set("project_permissions", platauth.PermViewContent)
			return next(c)
		}
	}))
	reader := &routeRemote{h: readerEcho, base: base}
	got, err := reader.Get(ctx, name)
	require.NoError(t, err)
	assert.Equal(t, "one\n", string(got))
	rec := reader.do(ctx, http.MethodPost, base, []byte(`{"objects":[]}`))
	assert.Equal(t, http.StatusForbidden, rec.Code)

	rec = writer.do(ctx, http.MethodGet, base+"?dir=log", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "a directory is named with its slash")
}

// The conflict answer carries the code a client maps to ErrObjectExists.
func TestContextObjectRoutes_ConflictBody_SQLite(t *testing.T) {
	ctx := t.Context()
	e, cs := contextObjectSQLiteServer(t, "member-a", platauth.PermViewContent|contextObjectWritePermission)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + seedContextObjectProject(t, cs) + "/context/objects"}
	name := workspace.SegmentName("w1", "000000000000000000000001")
	require.NoError(t, r.Put(ctx, workspace.Object{Name: name, Data: []byte("one\n")}))
	body, err := json.Marshal(contextObjectBatch{Objects: []contextObjectWire{{Name: name, Data: []byte("two\n")}}})
	require.NoError(t, err)
	rec := r.do(ctx, http.MethodPost, r.base, body)
	require.Equal(t, http.StatusConflict, rec.Code)
	var e2 ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &e2))
	assert.Equal(t, "object_exists", e2.Code)
	assert.Contains(t, e2.Details, name)
}

// The routes on the full server, behind its real auth middleware and over
// PostgreSQL, pass the conformance suite, and a workspace viewer reads and
// cannot push.
func TestContextObjectRoutes_RemoteConformance_Postgres(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
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
		base := "/api/v1/test/" + createProject(t) + "/context/objects"
		return func(*testing.T) workspace.Remote { return &routeRemote{h: e, base: base, bearer: token} }
	})

	t.Run("pushed by and permissions", func(t *testing.T) {
		ctx := t.Context()
		pid := createProject(t)
		base := "/api/v1/test/" + pid + "/context/objects"
		name := workspace.SegmentName("w1", "000000000000000000000001")
		owner := &routeRemote{h: e, base: base, bearer: token}
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
		reader := &routeRemote{h: e, base: base, bearer: vt}
		got, err := reader.Get(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, "one\n", string(got))
		err = reader.Put(ctx, workspace.Object{Name: workspace.SegmentName("w2", "000000000000000000000002"), Data: []byte("x\n")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 403")
	})
}
