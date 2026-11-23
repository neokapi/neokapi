package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/host/flowdef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestServerWithFlowStore wires a server with content + flow-definition
// stores, and creates a project row the flow_definitions FK can reference.
func setupTestServerWithFlowStore(t *testing.T) (*Server, string) {
	t.Helper()
	srv := setupTestServerWithStores(t)

	pgStore := srv.ContentStore.(*bstore.PostgresStore)
	srv.FlowDefStore = bstore.NewFlowDefStore(pgStore.SQLDB())

	// Create a project so the flow_definitions project_id FK resolves.
	proj := &platstore.Project{
		Name:        "Flow Test",
		WorkspaceID: "demo",
	}
	require.NoError(t, srv.ContentStore.CreateProject(t.Context(), proj))
	return srv, proj.ID
}

func flowReq(t *testing.T, srv *Server, method, target, body, projectID, flowID string) *httptest.ResponseRecorder {
	t.Helper()
	e := srv.GetEcho()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	// These tests invoke handlers directly, bypassing ProjectAccessMiddleware;
	// grant full permissions so the handlers' permission checks pass.
	c.Set("project_permissions", platauth.PermAll)
	if flowID != "" {
		c.SetPathValues(echo.PathValues{{Name: "ws", Value: "demo"}, {Name: "id", Value: projectID}, {Name: "flowId", Value: flowID}})
	} else {
		c.SetPathValues(echo.PathValues{{Name: "ws", Value: "demo"}, {Name: "id", Value: projectID}})
	}

	var err error
	switch {
	case method == http.MethodGet && flowID != "":
		err = srv.HandleGetFlowDefinition(c)
	case method == http.MethodGet:
		err = srv.HandleListFlowDefinitions(c)
	case method == http.MethodPost:
		err = srv.HandleCreateFlowDefinition(c)
	case method == http.MethodPut:
		err = srv.HandleUpdateFlowDefinition(c)
	case method == http.MethodDelete:
		err = srv.HandleDeleteFlowDefinition(c)
	}
	require.NoError(t, err)
	return rec
}

const sampleFlowBody = `{
  "name": "Custom Translate",
  "description": "project flow",
  "nodes": [
    {"id":"reader","type":"reader","name":"auto","position":{"x":0,"y":0}},
    {"id":"translate","type":"tool","name":"translate","position":{"x":250,"y":0}},
    {"id":"writer","type":"writer","name":"auto","position":{"x":500,"y":0}}
  ],
  "edges": [
    {"id":"e1","source":"reader","target":"translate"},
    {"id":"e2","source":"translate","target":"writer"}
  ]
}`

func TestHandleListFlowDefinitions_BuiltInOnly(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)

	rec := flowReq(t, srv, http.MethodGet, "/api/v1/demo/"+projectID+"/flows", "", projectID, "")
	assert.Equal(t, http.StatusOK, rec.Code)

	var defs []flow.FlowDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &defs))
	// All built-in flows are present even with no stored flows.
	assert.GreaterOrEqual(t, len(defs), len(flowdef.BuiltInFlows()))
	var hasBuiltIn bool
	for _, d := range defs {
		if d.ID == "translate" && d.Source == "built-in" {
			hasBuiltIn = true
		}
	}
	assert.True(t, hasBuiltIn, "built-in translate flow should be listed")
	// The one-step built-ins have no edges; the web app iterates `edges` and
	// failed on null.
	assert.NotContains(t, rec.Body.String(), `"edges":null`)
	assert.NotContains(t, rec.Body.String(), `"nodes":null`)
}

func TestHandleFlowDefinition_CRUD(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)
	base := "/api/v1/demo/" + projectID + "/flows"

	// Create.
	rec := flowReq(t, srv, http.MethodPost, base, sampleFlowBody, projectID, "")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created bstore.StoredFlow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.NotEmpty(t, created.ID)
	assert.NotEmpty(t, created.Revision)
	assert.Equal(t, "project", created.Source)
	assert.Equal(t, "Custom Translate", created.Name)
	assert.Len(t, created.Nodes, 3)
	assert.NotEmpty(t, created.CreatedAt)

	// List now includes the project flow alongside built-ins.
	rec = flowReq(t, srv, http.MethodGet, base, "", projectID, "")
	var defs []bstore.StoredFlow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &defs))
	var found bool
	for _, d := range defs {
		if d.ID == created.ID {
			found = true
			assert.Equal(t, "project", d.Source)
			assert.Equal(t, created.Revision, d.Revision, "the listing carries the revision to save against")
		}
	}
	assert.True(t, found, "created project flow should be in the listing")

	// Get the single project flow.
	rec = flowReq(t, srv, http.MethodGet, base+"/"+created.ID, "", projectID, created.ID)
	require.Equal(t, http.StatusOK, rec.Code)
	var got flow.FlowDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, created.ID, got.ID)

	// Update, based on the revision read.
	updateBody := withName(t, "Renamed Flow", created.Revision)
	rec = flowReq(t, srv, http.MethodPut, base+"/"+created.ID, updateBody, projectID, created.ID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var updated flow.FlowDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "Renamed Flow", updated.Name)
	assert.Equal(t, created.ID, updated.ID)

	// Delete.
	rec = flowReq(t, srv, http.MethodDelete, base+"/"+created.ID, "", projectID, created.ID)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Get after delete → 404.
	rec = flowReq(t, srv, http.MethodGet, base+"/"+created.ID, "", projectID, created.ID)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleGetFlowDefinition_BuiltIn(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)
	base := "/api/v1/demo/" + projectID + "/flows"

	rec := flowReq(t, srv, http.MethodGet, base+"/translate", "", projectID, "translate")
	require.Equal(t, http.StatusOK, rec.Code)
	var def flow.FlowDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &def))
	assert.Equal(t, "translate", def.ID)
	assert.Equal(t, "built-in", def.Source)
}

func TestHandleFlowDefinition_BuiltInImmutable(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)
	base := "/api/v1/demo/" + projectID + "/flows"

	// Cannot create a flow whose id collides with a built-in.
	body := strings.Replace(sampleFlowBody, `"name": "Custom Translate"`, `"id":"translate","name":"X"`, 1)
	rec := flowReq(t, srv, http.MethodPost, base, body, projectID, "")
	assert.Equal(t, http.StatusConflict, rec.Code)

	// Cannot update a built-in.
	rec = flowReq(t, srv, http.MethodPut, base+"/translate", sampleFlowBody, projectID, "translate")
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Cannot delete a built-in.
	rec = flowReq(t, srv, http.MethodDelete, base+"/translate", "", projectID, "translate")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleCreateFlowDefinition_Invalid(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)
	base := "/api/v1/demo/" + projectID + "/flows"

	// Edge references a non-existent node → validation error.
	bad := `{"name":"Bad","nodes":[{"id":"a","type":"tool","name":"translate","position":{"x":0,"y":0}}],"edges":[{"id":"e","source":"a","target":"missing"}]}`
	rec := flowReq(t, srv, http.MethodPost, base, bad, projectID, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// withName returns sampleFlowBody renamed and, when base is set, carrying the
// revision the edit was based on.
func withName(t *testing.T, name, base string) string {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(sampleFlowBody), &m))
	m["name"] = name
	if base != "" {
		m["base_revision"] = base
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return string(b)
}

// Two people open the same flow. The first save wins and moves the revision;
// the second, based on the revision both read, is refused with the flow as it
// now stands, and nothing of it is written.
func TestHandleUpdateFlowDefinition_StaleSaveRefused(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)
	base := "/api/v1/demo/" + projectID + "/flows"

	rec := flowReq(t, srv, http.MethodPost, base, sampleFlowBody, projectID, "")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created bstore.StoredFlow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	read := created.Revision

	rec = flowReq(t, srv, http.MethodPut, base+"/"+created.ID, withName(t, "Alex's edit", read), projectID, created.ID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var saved bstore.StoredFlow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &saved))
	assert.NotEqual(t, read, saved.Revision)

	rec = flowReq(t, srv, http.MethodPut, base+"/"+created.ID, withName(t, "Blake's edit", read), projectID, created.ID)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var changed FlowChangedResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &changed))
	assert.Equal(t, "flow_changed", changed.Code)
	require.NotNil(t, changed.Current)
	assert.Equal(t, "Alex's edit", changed.Current.Name)
	assert.Equal(t, saved.Revision, changed.Current.Revision)

	// Saving over it on purpose names the revision it now has.
	rec = flowReq(t, srv, http.MethodPut, base+"/"+created.ID, withName(t, "Blake's edit", saved.Revision), projectID, created.ID)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var over bstore.StoredFlow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &over))
	assert.Equal(t, "Blake's edit", over.Name)
}

// A save without the revision it was based on, as an app older than the
// check sends, is refused with a message that says what to do, and nothing is
// written.
func TestHandleUpdateFlowDefinition_WithoutBaseRevisionRefused(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)
	base := "/api/v1/demo/" + projectID + "/flows"
	rec := flowReq(t, srv, http.MethodPost, base, sampleFlowBody, projectID, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var created bstore.StoredFlow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	rec = flowReq(t, srv, http.MethodPut, base+"/"+created.ID, withName(t, "Overwrite", ""), projectID, created.ID)
	require.Equal(t, http.StatusPreconditionRequired, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "update it")

	rec = flowReq(t, srv, http.MethodGet, base+"/"+created.ID, "", projectID, created.ID)
	var stored bstore.StoredFlow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stored))
	assert.Equal(t, created.Name, stored.Name)
	assert.Equal(t, created.Revision, stored.Revision)
}

// A save based on a flow someone deleted since is refused as not found, and
// does not bring the flow back.
func TestHandleUpdateFlowDefinition_DeletedSinceRead(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)
	base := "/api/v1/demo/" + projectID + "/flows"
	rec := flowReq(t, srv, http.MethodPost, base, sampleFlowBody, projectID, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var created bstore.StoredFlow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	rec = flowReq(t, srv, http.MethodDelete, base+"/"+created.ID, "", projectID, created.ID)
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec = flowReq(t, srv, http.MethodPut, base+"/"+created.ID, withName(t, "edit", created.Revision), projectID, created.ID)
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	rec = flowReq(t, srv, http.MethodGet, base+"/"+created.ID, "", projectID, created.ID)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// A create never replaces a stored flow of the same id.
func TestHandleCreateFlowDefinition_ExistingIDRefused(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)
	base := "/api/v1/demo/" + projectID + "/flows"
	rec := flowReq(t, srv, http.MethodPost, base, sampleFlowBody, projectID, "")
	require.Equal(t, http.StatusCreated, rec.Code)
	var created bstore.StoredFlow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(withName(t, "Overwrite", "")), &m))
	m["id"] = created.ID
	b, _ := json.Marshal(m)
	rec = flowReq(t, srv, http.MethodPost, base, string(b), projectID, "")
	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())

	rec = flowReq(t, srv, http.MethodGet, base+"/"+created.ID, "", projectID, created.ID)
	var stored bstore.StoredFlow
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stored))
	assert.Equal(t, created.Name, stored.Name)
}

// UpdateFlowDefinitionRequest embeds flow.FlowDefinition, whose MarshalJSON Go
// promotes to it; a client that marshals the request must still send
// base_revision, or the server refuses the save with 428.
func TestUpdateFlowDefinitionRequest_JSON(t *testing.T) {
	req := UpdateFlowDefinitionRequest{
		ID: "f1", Name: "One", Source: "project",
		Nodes:        []flow.FlowNode{{ID: "a", Type: flow.NodeTool, Name: "qa"}},
		BaseRevision: "r1",
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Equal(t, `{"id":"f1","name":"One","nodes":[{"id":"a","type":"tool","name":"qa","position":{"x":0,"y":0}}],`+
		`"edges":[],"source":"project","base_revision":"r1"}`, string(data))

	var back UpdateFlowDefinitionRequest
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, "r1", back.BaseRevision)
	assert.Equal(t, "f1", back.ID)
	assert.Equal(t, []flow.FlowEdge{}, back.Edges)
}
