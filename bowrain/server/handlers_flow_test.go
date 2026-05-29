package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/flow"
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
	if flowID != "" {
		c.SetParamNames("ws", "id", "flowId")
		c.SetParamValues("demo", projectID, flowID)
	} else {
		c.SetParamNames("ws", "id")
		c.SetParamValues("demo", projectID)
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
    {"id":"ai-translate","type":"tool","name":"ai-translate","position":{"x":250,"y":0}},
    {"id":"writer","type":"writer","name":"auto","position":{"x":500,"y":0}}
  ],
  "edges": [
    {"id":"e1","source":"reader","target":"ai-translate"},
    {"id":"e2","source":"ai-translate","target":"writer"}
  ]
}`

func TestHandleListFlowDefinitions_BuiltInOnly(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)

	rec := flowReq(t, srv, http.MethodGet, "/api/v1/demo/"+projectID+"/flows", "", projectID, "")
	assert.Equal(t, http.StatusOK, rec.Code)

	var defs []flow.FlowDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &defs))
	// All built-in flows are present even with no stored flows.
	assert.GreaterOrEqual(t, len(defs), len(flow.BuiltInFlows()))
	var hasBuiltIn bool
	for _, d := range defs {
		if d.ID == "ai-translate" && d.Source == "built-in" {
			hasBuiltIn = true
		}
	}
	assert.True(t, hasBuiltIn, "built-in ai-translate flow should be listed")
}

func TestHandleFlowDefinition_CRUD(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)
	base := "/api/v1/demo/" + projectID + "/flows"

	// Create.
	rec := flowReq(t, srv, http.MethodPost, base, sampleFlowBody, projectID, "")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created flow.FlowDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "project", created.Source)
	assert.Equal(t, "Custom Translate", created.Name)
	assert.Len(t, created.Nodes, 3)
	assert.NotEmpty(t, created.CreatedAt)

	// List now includes the project flow alongside built-ins.
	rec = flowReq(t, srv, http.MethodGet, base, "", projectID, "")
	var defs []flow.FlowDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &defs))
	var found bool
	for _, d := range defs {
		if d.ID == created.ID {
			found = true
			assert.Equal(t, "project", d.Source)
		}
	}
	assert.True(t, found, "created project flow should be in the listing")

	// Get the single project flow.
	rec = flowReq(t, srv, http.MethodGet, base+"/"+created.ID, "", projectID, created.ID)
	require.Equal(t, http.StatusOK, rec.Code)
	var got flow.FlowDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, created.ID, got.ID)

	// Update.
	updateBody := strings.Replace(sampleFlowBody, "Custom Translate", "Renamed Flow", 1)
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

	rec := flowReq(t, srv, http.MethodGet, base+"/ai-translate", "", projectID, "ai-translate")
	require.Equal(t, http.StatusOK, rec.Code)
	var def flow.FlowDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &def))
	assert.Equal(t, "ai-translate", def.ID)
	assert.Equal(t, "built-in", def.Source)
}

func TestHandleFlowDefinition_BuiltInImmutable(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)
	base := "/api/v1/demo/" + projectID + "/flows"

	// Cannot create a flow whose id collides with a built-in.
	body := strings.Replace(sampleFlowBody, `"name": "Custom Translate"`, `"id":"ai-translate","name":"X"`, 1)
	rec := flowReq(t, srv, http.MethodPost, base, body, projectID, "")
	assert.Equal(t, http.StatusConflict, rec.Code)

	// Cannot update a built-in.
	rec = flowReq(t, srv, http.MethodPut, base+"/ai-translate", sampleFlowBody, projectID, "ai-translate")
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// Cannot delete a built-in.
	rec = flowReq(t, srv, http.MethodDelete, base+"/ai-translate", "", projectID, "ai-translate")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleCreateFlowDefinition_Invalid(t *testing.T) {
	srv, projectID := setupTestServerWithFlowStore(t)
	base := "/api/v1/demo/" + projectID + "/flows"

	// Edge references a non-existent node → validation error.
	bad := `{"name":"Bad","nodes":[{"id":"a","type":"tool","name":"ai-translate","position":{"x":0,"y":0}}],"edges":[{"id":"e","source":"a","target":"missing"}]}`
	rec := flowReq(t, srv, http.MethodPost, base, bad, projectID, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
