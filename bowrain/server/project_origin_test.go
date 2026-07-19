package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the connector-sourced read-only gate end-to-end against a
// real SQLite content store + connector-config store (no mock backend):
//   - a Managed project (no source connector) accepts uploads and deletes;
//   - a Connected project (a forge connector bound by project_id — the kapi
//     push / GitHub-App case, signal #1) refuses every SOURCE mutation with 409;
//   - a connector-backed collection (signal #2) refuses uploads even when the
//     project itself has no source connector, and the project rolls up to Hybrid.

const originTestWS = "ws-origin"

func newOriginTestServer(t *testing.T) (*Server, *sqlitestore.SQLiteStore, *bstore.ConnectorConfigStore) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	// nil cipher exercises the plaintext pass-through; the gate only reads the
	// connector type + project_id, neither of which is a secret field.
	ccs := bstore.NewConnectorConfigStore(cs.DB(), nil)

	srv := &Server{
		ContentStore:         cs,
		FormatRegistry:       reg,
		ConnectorConfigStore: ccs,
		wsStores:             newWorkspaceStores(),
	}
	return srv, cs, ccs
}

func seedOriginProject(t *testing.T, cs *sqlitestore.SQLiteStore, name string) *platstore.Project {
	t.Helper()
	proj := &platstore.Project{
		Name:                  name,
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
		WorkspaceID:           originTestWS,
		Properties:            map[string]string{},
	}
	require.NoError(t, cs.CreateProject(t.Context(), proj))
	return proj
}

// bindForgeSource persists a forge source connector bound to the project by
// project_id — exactly what the forge/GitHub-App setup writes and what the gate
// treats as signal #1.
func bindForgeSource(t *testing.T, ccs *bstore.ConnectorConfigStore, projectID string) {
	t.Helper()
	_, err := ccs.Upsert(t.Context(), &bstore.ConnectorConfig{
		WorkspaceID: originTestWS,
		Type:        "forge",
		Name:        "site repo",
		Config: map[string]string{
			"project_id": projectID,
			"repo":       "https://github.com/acme/site.git",
			"branch":     "main",
		},
	})
	require.NoError(t, err)
}

// originCtx builds an echo context wired for a project-scoped handler: the ws/id
// route params (+ any extra), the workspace id, and full permissions.
func originCtx(e *echo.Echo, r *http.Request, rec *httptest.ResponseRecorder, projectID string, extra ...[2]string) echo.Context {
	c := e.NewContext(r, rec)
	names := []string{"ws", "id"}
	values := []string{"acme", projectID}
	for _, p := range extra {
		names = append(names, p[0])
		values = append(values, p[1])
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	c.Set("workspace_id", originTestWS)
	c.Set("project_permissions", platauth.PermAll)
	return c
}

func getOriginProjectInfo(t *testing.T, srv *Server, e *echo.Echo, projectID string) ProjectInfoResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/acme/"+projectID, nil)
	c := originCtx(e, r, rec, projectID)
	require.NoError(t, srv.HandleGetEditorProject(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var info ProjectInfoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
	return info
}

func TestSourceGate_ManagedProjectAllowsSourceMutations(t *testing.T) {
	srv, cs, _ := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "managed-proj")
	e := echo.New()

	// Upload succeeds.
	body, ct := multipartFiles(t, map[string]string{"app.json": `{"greeting":"Hello"}`})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/"+proj.ID+"/items/main", body)
	r.Header.Set("Content-Type", ct)
	c := originCtx(e, r, rec, proj.ID, [2]string{"ref", "main"})
	require.NoError(t, srv.HandleUploadFiles(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Deleting the uploaded item succeeds.
	rec2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodDelete, "/api/v1/acme/"+proj.ID+"/items/main?item=app.json", nil)
	c2 := originCtx(e, r2, rec2, proj.ID, [2]string{"ref", "main"})
	require.NoError(t, srv.HandleRemoveFile(c2))
	require.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())

	// The project reports Managed + editable.
	info := getOriginProjectInfo(t, srv, e, proj.ID)
	assert.Equal(t, projectTypeManaged, info.Type)
	assert.True(t, info.Editable)
}

func TestSourceGate_ConnectedProjectBlocksSourceMutations(t *testing.T) {
	srv, cs, ccs := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "connected-proj")
	bindForgeSource(t, ccs, proj.ID)
	e := echo.New()

	// Seed the default collection + a synced item, as a forge ingest would.
	require.NoError(t, EnsureDefaultCollection(t.Context(), cs, proj.ID))
	def, err := cs.GetDefaultCollection(t.Context(), proj.ID)
	require.NoError(t, err)
	require.NoError(t, cs.StoreItem(t.Context(), proj.ID, "main", &platstore.Item{
		Name:         "app.json",
		Format:       "json",
		ItemType:     "file",
		CollectionID: def.ID,
		Properties:   map[string]string{},
	}))

	// Project-level upload → 409, naming the repo the source is synced from.
	body, ct := multipartFiles(t, map[string]string{"extra.json": `{"a":"b"}`})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/"+proj.ID+"/items/main", body)
	r.Header.Set("Content-Type", ct)
	c := originCtx(e, r, rec, proj.ID, [2]string{"ref", "main"})
	require.NoError(t, srv.HandleUploadFiles(c))
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "github.com/acme/site.git")

	// Upload to a specific collection → 409.
	body2, ct2 := multipartFiles(t, map[string]string{"extra.json": `{"a":"b"}`})
	rec3 := httptest.NewRecorder()
	r3 := httptest.NewRequest(http.MethodPost, "/api/v1/acme/"+proj.ID+"/collections/main/"+def.ID+"/items", body2)
	r3.Header.Set("Content-Type", ct2)
	c3 := originCtx(e, r3, rec3, proj.ID, [2]string{"cid", def.ID})
	require.NoError(t, srv.HandleUploadToCollection(c3))
	assert.Equal(t, http.StatusConflict, rec3.Code, rec3.Body.String())

	// Item delete → 409.
	rec4 := httptest.NewRecorder()
	r4 := httptest.NewRequest(http.MethodDelete, "/api/v1/acme/"+proj.ID+"/items/main?item=app.json", nil)
	c4 := originCtx(e, r4, rec4, proj.ID, [2]string{"ref", "main"})
	require.NoError(t, srv.HandleRemoveFile(c4))
	assert.Equal(t, http.StatusConflict, rec4.Code, rec4.Body.String())

	// The item is still present — the delete was refused, not silently applied.
	items, err := cs.ListItems(t.Context(), proj.ID, "main")
	require.NoError(t, err)
	require.Len(t, items, 1)

	// The project reports Connected + read-only, and every collection is
	// connector-sourced.
	info := getOriginProjectInfo(t, srv, e, proj.ID)
	assert.Equal(t, projectTypeConnected, info.Type)
	assert.False(t, info.Editable)
	require.NotEmpty(t, info.Collections)
	for _, col := range info.Collections {
		assert.Equal(t, collectionOriginConnected, col.Origin, "collection %s", col.Name)
		assert.False(t, col.Editable, "collection %s", col.Name)
	}
}

func TestSourceGate_ConnectedCollectionBlocksUploadAndRollsUpHybrid(t *testing.T) {
	srv, cs, _ := newOriginTestServer(t)
	proj := seedOriginProject(t, cs, "hybrid-proj")
	e := echo.New()

	require.NoError(t, EnsureDefaultCollection(t.Context(), cs, proj.ID))
	def, err := cs.GetDefaultCollection(t.Context(), proj.ID)
	require.NoError(t, err)

	connected := &platstore.Collection{
		ProjectID:       proj.ID,
		Name:            "cms",
		Kind:            platstore.CollectionConnected,
		ItemLabel:       "page",
		ConnectorConfig: map[string]string{"base_url": "https://example.com"},
	}
	require.NoError(t, cs.CreateCollection(t.Context(), connected))

	// Upload to the connector-backed collection → 409 (signal #2, no project
	// source connector needed).
	body, ct := multipartFiles(t, map[string]string{"page.html": "<p>hi</p>"})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/"+proj.ID+"/collections/main/"+connected.ID+"/items", body)
	r.Header.Set("Content-Type", ct)
	c := originCtx(e, r, rec, proj.ID, [2]string{"cid", connected.ID})
	require.NoError(t, srv.HandleUploadToCollection(c))
	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())

	// Upload to the managed default collection → 200.
	body2, ct2 := multipartFiles(t, map[string]string{"app.json": `{"a":"b"}`})
	rec2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodPost, "/api/v1/acme/"+proj.ID+"/collections/main/"+def.ID+"/items", body2)
	r2.Header.Set("Content-Type", ct2)
	c2 := originCtx(e, r2, rec2, proj.ID, [2]string{"cid", def.ID})
	require.NoError(t, srv.HandleUploadToCollection(c2))
	assert.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())

	// Mixed collection origins roll up to Hybrid.
	info := getOriginProjectInfo(t, srv, e, proj.ID)
	assert.Equal(t, projectTypeHybrid, info.Type)

	origins := map[string]string{}
	for _, col := range info.Collections {
		origins[col.Name] = col.Origin
	}
	assert.Equal(t, collectionOriginManaged, origins["default"])
	assert.Equal(t, collectionOriginConnected, origins["cms"])
}
