package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	bconn "github.com/neokapi/neokapi/bowrain/connector"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/crypto"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newConnectorPersistenceServer builds a minimal Server wired for the connector
// endpoints: a SQLite content store, a SQLite-backed ConnectorConfigStore, the
// remote connector registry, and the service layer. It drives handlers directly
// with hand-built echo contexts (as handlers_connector_plan_test.go does), so it
// needs neither Postgres nor the auth stack. Returns the content store too so a
// test can simulate a restart by rebuilding the service layer.
func newConnectorPersistenceServer(t *testing.T, cipher *crypto.Cipher) (*Server, *sqlitestore.SQLiteStore) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	reg := platconn.NewRegistry()
	bconn.RegisterRemote(reg) // wordpress / figma / hubspot
	formatReg := registry.NewFormatRegistry()
	toolReg := registry.NewToolRegistry()

	s := &Server{
		ConnectorReg:         reg,
		FormatRegistry:       formatReg,
		ToolRegistry:         toolReg,
		ConnectorConfigStore: bstore.NewConnectorConfigStore(cs.DB(), cipher),
		Services:             service.NewServices(cs, reg, formatReg, toolReg),
	}
	return s, cs
}

func connectorTestCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	c, err := crypto.NewCipher(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	require.NoError(t, err)
	return c
}

// connCtx builds an echo context for a manage-connectors caller in a workspace,
// with the given method/body and optional :id path param.
func connCtx(t *testing.T, method, body, wsID, pathID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, "/connectors", nil)
	} else {
		r = httptest.NewRequest(method, "/connectors", bytes.NewReader([]byte(body)))
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	c.Set("workspace_id", wsID)
	c.Set("workspace_role", platauth.RoleOwner)
	c.Set("project_permissions", platauth.PermManageConnectors)
	if pathID != "" {
		c.SetParamNames("id")
		c.SetParamValues(pathID)
	}
	return c, rec
}

func TestConnectorAddPersistsAndSurvivesRestart(t *testing.T) {
	s, cs := newConnectorPersistenceServer(t, connectorTestCipher(t))
	const wsID = "ws-1"

	// Add a WordPress connector with a secret password.
	c, rec := connCtx(t, http.MethodPost,
		`{"type":"wordpress","config":{"url":"https://blog.example.com","username":"editor","password":"hunter2","name":"Blog"}}`,
		wsID, "")
	require.NoError(t, s.HandleAddConnector(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var added map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &added))
	id := added["id"]
	require.NotEmpty(t, id)

	// The config (with its sealed secret) is persisted.
	stored, err := s.ConnectorConfigStore.Get(context.Background(), wsID, id)
	require.NoError(t, err)
	assert.Equal(t, "wordpress", stored.Type)
	assert.Equal(t, "hunter2", stored.Config["password"], "the secret must be persisted (and decryptable)")

	// Simulate a restart: rebuild the service layer (a fresh, empty
	// ConnectorService) and rehydrate from the store.
	s.Services = service.NewServices(cs, s.ConnectorReg, s.FormatRegistry, s.ToolRegistry)
	_, err = s.Services.Connector.GetConnector(wsID, id)
	require.Error(t, err, "precondition: the live instance is gone after a restart")

	s.rehydrateConnectors(context.Background())

	conn, err := s.Services.Connector.GetConnector(wsID, id)
	require.NoError(t, err, "the connector must be live again after rehydration")
	assert.Equal(t, id, conn.ID(), "the rehydrated instance keeps its persisted id")
}

func TestConnectorUpdatePreservesBlankSecret(t *testing.T) {
	s, _ := newConnectorPersistenceServer(t, connectorTestCipher(t))
	const wsID = "ws-1"

	c, rec := connCtx(t, http.MethodPost,
		`{"type":"wordpress","config":{"url":"https://blog.example.com","username":"editor","password":"hunter2"}}`,
		wsID, "")
	require.NoError(t, s.HandleAddConnector(c))
	require.Equal(t, http.StatusCreated, rec.Code)
	var added map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &added))
	id := added["id"]

	// Update: new URL, BLANK password → the stored secret is preserved.
	c, rec = connCtx(t, http.MethodPut,
		`{"type":"wordpress","config":{"url":"https://new.example.com","username":"editor","password":""}}`,
		wsID, id)
	require.NoError(t, s.HandleUpdateConnector(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Response redacts the secret.
	var updated connectorInfo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Empty(t, updated.Config["password"], "update response must not echo the secret")
	assert.Equal(t, "https://new.example.com", updated.Config["url"])

	// Stored: password preserved, url updated.
	stored, err := s.ConnectorConfigStore.Get(context.Background(), wsID, id)
	require.NoError(t, err)
	assert.Equal(t, "hunter2", stored.Config["password"], "a blank secret on update must keep the stored credential")
	assert.Equal(t, "https://new.example.com", stored.Config["url"])

	// The live instance was reconfigured in place (still addressable by id).
	_, err = s.Services.Connector.GetConnector(wsID, id)
	require.NoError(t, err)

	// Updating a connector that does not exist is a 404.
	c, rec = connCtx(t, http.MethodPut,
		`{"type":"wordpress","config":{"url":"https://x.example.com"}}`, wsID, "does-not-exist")
	require.NoError(t, s.HandleUpdateConnector(c))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestConnectorRemoveDeletesPersistedRow(t *testing.T) {
	s, _ := newConnectorPersistenceServer(t, connectorTestCipher(t))
	const wsID = "ws-1"

	c, rec := connCtx(t, http.MethodPost,
		`{"type":"figma","config":{"file_key":"abc","token":"tok","name":"Design"}}`, wsID, "")
	require.NoError(t, s.HandleAddConnector(c))
	require.Equal(t, http.StatusCreated, rec.Code)
	var added map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &added))
	id := added["id"]

	// Remove it.
	c, rec = connCtx(t, http.MethodDelete, "", wsID, id)
	require.NoError(t, s.HandleRemoveConnector(c))
	require.Equal(t, http.StatusNoContent, rec.Code)

	// The persisted row is gone.
	_, err := s.ConnectorConfigStore.Get(context.Background(), wsID, id)
	require.ErrorIs(t, err, bstore.ErrConnectorConfigNotFound)

	// The live instance is gone too.
	_, err = s.Services.Connector.GetConnector(wsID, id)
	require.Error(t, err)

	// A second remove is a 404.
	c, rec = connCtx(t, http.MethodDelete, "", wsID, id)
	require.NoError(t, s.HandleRemoveConnector(c))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestListConnectorsRedactsSecretsFromPersistedSet(t *testing.T) {
	s, _ := newConnectorPersistenceServer(t, connectorTestCipher(t))
	const wsID = "ws-1"

	c, rec := connCtx(t, http.MethodPost,
		`{"type":"hubspot","config":{"api_key":"key-secret","name":"Marketing"}}`, wsID, "")
	require.NoError(t, s.HandleAddConnector(c))
	require.Equal(t, http.StatusCreated, rec.Code)

	c, rec = connCtx(t, http.MethodGet, "", wsID, "")
	require.NoError(t, s.HandleListActiveConnectors(c))
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.NotContains(t, body, "key-secret", "the listing must never leak a secret")

	var list []connectorInfo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, "hubspot", list[0].Type)
	assert.Equal(t, "Marketing", list[0].Name)
	assert.Equal(t, platconn.CategoryMarketing, list[0].Category, "category is enriched from the registry")
	assert.Contains(t, list[0].Config, "api_key", "the secret key is present so the UI knows it is set")
	assert.Empty(t, list[0].Config["api_key"], "the secret value is redacted")

	// A different workspace sees nothing.
	c, rec = connCtx(t, http.MethodGet, "", "ws-other", "")
	require.NoError(t, s.HandleListActiveConnectors(c))
	require.Equal(t, http.StatusOK, rec.Code)
	var otherList []connectorInfo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &otherList))
	assert.Empty(t, otherList, "connector listing is workspace-scoped")
}

// recordingConnector is a stub IntegrationConnector that records which connector
// id a Fetch resolved to, so a test can prove the path :id is preferred over the
// body connector_id.
type recordingConnector struct {
	id     string
	record func(string)
}

func (r *recordingConnector) ID() string                        { return r.id }
func (r *recordingConnector) Name() string                      { return r.id }
func (r *recordingConnector) Category() platconn.Category       { return platconn.CategoryCMS }
func (r *recordingConnector) Configure(map[string]string) error { return nil }
func (r *recordingConnector) Close() error                      { return nil }
func (r *recordingConnector) Fetch(ctx context.Context, _ platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	r.record(r.id)
	return nil, nil
}
func (r *recordingConnector) Publish(context.Context, []*platconn.ContentItem, platconn.PublishOptions) error {
	return nil
}
func (r *recordingConnector) List(context.Context) ([]*platconn.ContentItem, error) { return nil, nil }
func (r *recordingConnector) Status(context.Context) (*platconn.SyncStatus, error) {
	return &platconn.SyncStatus{ConnectorID: r.id}, nil
}

func TestFetchPrefersPathIDOverBody(t *testing.T) {
	s, _ := newConnectorPersistenceServer(t, connectorTestCipher(t))
	const wsID = "ws-1"

	var fetched []string
	s.ConnectorReg.Register("stub", platconn.CategoryCMS, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return &recordingConnector{id: config["id"], record: func(id string) { fetched = append(fetched, id) }}, nil
	})

	// Two live connectors: the one named in the path and a decoy in the body.
	_, err := s.Services.Connector.AddConnector(wsID, "stub", map[string]string{"id": "path-id"})
	require.NoError(t, err)
	_, err = s.Services.Connector.AddConnector(wsID, "stub", map[string]string{"id": "body-id"})
	require.NoError(t, err)

	// Path :id = path-id, body connector_id = body-id → the path must win.
	c, rec := connCtx(t, http.MethodPost,
		`{"connector_id":"body-id","project_id":"p1"}`, wsID, "path-id")
	require.NoError(t, s.HandleFetch(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []string{"path-id"}, fetched, "the path :id must be preferred over the body connector_id")

	// With no path id, the body connector_id is the fallback.
	fetched = nil
	c, rec = connCtx(t, http.MethodPost, `{"connector_id":"body-id","project_id":"p1"}`, wsID, "")
	require.NoError(t, s.HandleFetch(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []string{"body-id"}, fetched, "with no path id, the body connector_id is used")
}

// listingConnector is a stub IntegrationConnector whose List returns a fixed set
// of content items, so a test can exercise the read-only content-listing route.
type listingConnector struct {
	id    string
	items []*platconn.ContentItem
}

func (l *listingConnector) ID() string                        { return l.id }
func (l *listingConnector) Name() string                      { return l.id }
func (l *listingConnector) Category() platconn.Category       { return platconn.CategoryCMS }
func (l *listingConnector) Configure(map[string]string) error { return nil }
func (l *listingConnector) Close() error                      { return nil }
func (l *listingConnector) Fetch(context.Context, platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	return nil, nil
}
func (l *listingConnector) Publish(context.Context, []*platconn.ContentItem, platconn.PublishOptions) error {
	return nil
}
func (l *listingConnector) List(context.Context) ([]*platconn.ContentItem, error) { return l.items, nil }
func (l *listingConnector) Status(context.Context) (*platconn.SyncStatus, error) {
	return &platconn.SyncStatus{ConnectorID: l.id}, nil
}

// connContentCtx builds an echo context for a workspace-member caller hitting the
// read-only content route, with the :id path param and an optional ?paths= query.
func connContentCtx(t *testing.T, wsID, pathID, paths string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	target := "/connectors/" + pathID + "/content"
	if paths != "" {
		target += "?paths=" + url.QueryEscape(paths)
	}
	r := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	c.Set("workspace_id", wsID)
	c.SetParamNames("id")
	c.SetParamValues(pathID)
	return c, rec
}

func TestConnectorContentListsItems(t *testing.T) {
	s, _ := newConnectorPersistenceServer(t, connectorTestCipher(t))
	const wsID = "ws-1"

	items := []*platconn.ContentItem{
		{ID: "home", Path: "/home", Name: "Home", Format: "html"},
		{ID: "about", Path: "/about", Name: "About", Format: "html"},
	}
	s.ConnectorReg.Register("listing", platconn.CategoryCMS, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return &listingConnector{id: config["id"], items: items}, nil
	})
	_, err := s.Services.Connector.AddConnector(wsID, "listing", map[string]string{"id": "wp-1"})
	require.NoError(t, err)

	// Full listing returns every item, marshaled with the ContentItem struct's
	// own (PascalCase) JSON keys under a stable {items:[...]} envelope.
	c, rec := connContentCtx(t, wsID, "wp-1", "")
	require.NoError(t, s.HandleConnectorContent(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp struct {
		Items []*platconn.ContentItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)
	assert.Equal(t, "home", resp.Items[0].ID)
	assert.Equal(t, "/home", resp.Items[0].Path)
	assert.Equal(t, "Home", resp.Items[0].Name)
	assert.Contains(t, rec.Body.String(), `"Path":"/home"`, "items use the ContentItem struct JSON shape verbatim")

	// ?paths= narrows to the matching item (matched by Path or ID).
	c, rec = connContentCtx(t, wsID, "wp-1", "/about")
	require.NoError(t, s.HandleConnectorContent(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "about", resp.Items[0].ID)

	// An unknown connector id is a 404.
	c, rec = connContentCtx(t, wsID, "does-not-exist", "")
	require.NoError(t, s.HandleConnectorContent(c))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// The connector is workspace-scoped: another workspace cannot address it.
	c, rec = connContentCtx(t, "ws-other", "wp-1", "")
	require.NoError(t, s.HandleConnectorContent(c))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
