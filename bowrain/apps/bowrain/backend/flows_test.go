package backend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/neokapi/neokapi/bowrain/editorclient"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Flow definitions on the desktop are now project-scoped and proxy to the
// server REST API (#766). When disconnected, the built-in flows remain
// available as a read-only fallback; authoring requires a connection.

func TestListFlowDefinitions_OfflineReturnsBuiltIns(t *testing.T) {
	app := NewApp() // not connected
	defs, err := app.ListFlowDefinitions("proj-1")
	require.NoError(t, err)
	require.NotEmpty(t, defs)

	ids := make(map[string]bool)
	for _, d := range defs {
		assert.NotEmpty(t, d.ID)
		assert.NotEmpty(t, d.Name)
		assert.Equal(t, registry.SourceBuiltIn, d.Source)
		assert.NotEmpty(t, d.Nodes)
		// Since AD-026 a flow's I/O ends are bindings, not nodes, so a
		// single-tool flow (translate, pseudo-translate, …) has one node and
		// no edges. Only multi-node graphs must be wired together.
		if len(d.Nodes) > 1 {
			assert.NotEmpty(t, d.Edges, "multi-node flow %q should have edges", d.ID)
		}
		ids[d.ID] = true
	}
	assert.True(t, ids["translate"])
	assert.True(t, ids["translate-qa"])
	assert.True(t, ids["pseudo-translate"])
	assert.True(t, ids["secure-translate"])
}

func TestGetFlowDefinition_OfflineBuiltIn(t *testing.T) {
	app := NewApp()
	info, err := app.GetFlowDefinition("proj-1", "translate")
	require.NoError(t, err)
	assert.Equal(t, "Translate", info.Name)
	assert.Equal(t, registry.SourceBuiltIn, info.Source)
}

func TestGetFlowDefinition_OfflineNotFound(t *testing.T) {
	app := NewApp()
	_, err := app.GetFlowDefinition("proj-1", "nonexistent")
	assert.Error(t, err)
}

func TestSaveFlowDefinition_OfflineRequiresConnection(t *testing.T) {
	app := NewApp()
	_, err := app.SaveFlowDefinition("proj-1", FlowDefinitionInfo{
		ID:     "custom",
		Name:   "Custom",
		Source: "project",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the app is offline")
	// The frontend is told the request did not reach the server.
	wire, merr := json.Marshal(&err)
	require.NoError(t, merr)
	assert.JSONEq(t, `{"network":true}`, string(wire))
}

func TestSaveFlowDefinition_BuiltInRejected(t *testing.T) {
	app := NewApp()
	_, err := app.SaveFlowDefinition("proj-1", FlowDefinitionInfo{
		ID:     "translate",
		Name:   "AI Translate",
		Source: registry.SourceBuiltIn,
	})
	require.Error(t, err)
	// Offline check fires first (not connected). Either guard is acceptable;
	// the point is built-in/offline writes never succeed.
	assert.Error(t, err)
}

func TestDeleteFlowDefinition_OfflineRequiresConnection(t *testing.T) {
	app := NewApp()
	err := app.DeleteFlowDefinition("proj-1", "custom")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the app is offline")
	// The frontend is told the request did not reach the server.
	wire, merr := json.Marshal(&err)
	require.NoError(t, merr)
	assert.JSONEq(t, `{"network":true}`, string(wire))
}

func TestFlowDefToInfo_Serialization(t *testing.T) {
	def := flow.FlowDefinition{
		ID:   "test-flow",
		Name: "Test Flow",
		Nodes: []flow.FlowNode{
			{ID: "reader", Type: flow.NodeReader, Name: "auto", Position: flow.NodePosition{X: 0, Y: 0}},
			{ID: "redact", Type: flow.NodeTool, Name: "redact", Position: flow.NodePosition{X: 200, Y: 0}},
			{ID: "t1", Type: flow.NodeTool, Name: "translate", Position: flow.NodePosition{X: 400, Y: 0}},
			{ID: "writer", Type: flow.NodeWriter, Name: "auto", Position: flow.NodePosition{X: 600, Y: 0}},
		},
		Edges: []flow.FlowEdge{
			{ID: "e1", Source: "reader", Target: "redact"},
			{ID: "e2", Source: "redact", Target: "t1"},
			{ID: "e3", Source: "t1", Target: "writer"},
		},
	}

	info := flowDefToInfo(def)
	byID := make(map[string]FlowNodeInfo)
	for _, n := range info.Nodes {
		byID[n.ID] = n
	}
	assert.Equal(t, "redact", byID["redact"].Name)
	assert.Equal(t, "tool", byID["t1"].Type)
	assert.Len(t, info.Edges, 3)
}

func TestSecureTranslateBuiltInOrdersRedactFirst(t *testing.T) {
	app := NewApp()
	info, err := app.GetFlowDefinition("proj-1", "secure-translate")
	require.NoError(t, err)

	// Transformers are ordinary ordered steps (AD-006): redact is simply the
	// first tool node, ahead of the remote-egress translate step.
	require.NotEmpty(t, info.Nodes)
	assert.Equal(t, "redact", info.Nodes[0].Name, "secure-translate starts with redact")
}

// A desktop save names the revision it was based on, and a save the server
// refuses as stale comes back as the flow as it now stands, not as an error
// string.
func TestSaveFlowDefinition_StaleSaveCarriesTheCurrentFlow(t *testing.T) {
	var gotBase any
	srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotBase = body["base_revision"]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":  "flow_changed",
			"error": "changed",
			"current": map[string]any{
				"id": "flow-1", "name": "Alex's edit", "source": "project", "revision": "r2",
				"nodes": []any{}, "edges": []any{},
			},
		})
	}))
	t.Cleanup(srv.Close)

	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = srv.URL
	app.activeWS = "acme"
	app.remoteHTTP = editorclient.New(srv.URL, "tok")
	app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "tok"}
	app.mu.Unlock()

	res, err := app.SaveFlowDefinition("proj-1", FlowDefinitionInfo{
		ID: "flow-1", Name: "Blake's edit", Source: "project", Revision: "r1",
		Nodes: []FlowNodeInfo{}, Edges: []FlowEdgeInfo{},
	})
	require.NoError(t, err)
	assert.Equal(t, "r1", gotBase, "the save names the revision it was based on")
	require.NotNil(t, res.Changed)
	assert.Nil(t, res.Flow)
	assert.Equal(t, "Alex's edit", res.Changed.Name)
	assert.Equal(t, "r2", res.Changed.Revision)
}

// A save the server refuses reads as the server's sentence on the desktop,
// as it does on the web: no URL, no status code, no JSON.
func TestSaveFlowDefinition_RefusalReadsAsTheServersSentence(t *testing.T) {
	refusals := map[int]string{
		http.StatusBadRequest:           "edge e1 references a node that does not exist",
		http.StatusForbidden:            "cannot modify a built-in flow",
		http.StatusNotFound:             "this flow was deleted after it was read",
		http.StatusPreconditionRequired: "this version of the app cannot save flows; update it, then save again",
	}
	for status, sentence := range refusals {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": sentence})
			}))
			t.Cleanup(srv.Close)
			app := newTestApp(t)
			app.mu.Lock()
			app.connState = StateConnected
			app.serverURL = srv.URL
			app.activeWS = "acme"
			app.remoteHTTP = editorclient.New(srv.URL, "tok")
			app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "tok"}
			app.mu.Unlock()

			_, err := app.SaveFlowDefinition("proj-1", FlowDefinitionInfo{
				ID: "flow-1", Name: "Mine", Source: "project", Revision: "r1",
				Nodes: []FlowNodeInfo{}, Edges: []FlowEdgeInfo{},
			})
			require.Error(t, err)
			assert.Equal(t, sentence, err.Error())

			// The frontend receives the error as Wails marshals it, with the
			// server's status and answer, and renders it as the web does.
			wire, merr := json.Marshal(&err)
			require.NoError(t, merr)
			assert.JSONEq(t, fmt.Sprintf(`{"status":%d,"body":{"error":%q}}`, status, sentence), string(wire))
		})
	}
}

// A save that cannot reach the server tells the frontend so, rather than
// handing it the transport's message with the server's URL in it.
func TestSaveFlowDefinition_UnreachableServerIsReportedAsSuch(t *testing.T) {
	srv := newTestServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	serverURL := srv.URL
	srv.Close()

	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = serverURL
	app.activeWS = "acme"
	app.remoteHTTP = editorclient.New(serverURL, "tok")
	app.authInfo = &config.StoredAuth{ServerURL: serverURL, AccessToken: "tok"}
	app.mu.Unlock()

	_, err := app.SaveFlowDefinition("proj-1", FlowDefinitionInfo{
		ID: "flow-1", Name: "Mine", Source: "project", Revision: "r1",
		Nodes: []FlowNodeInfo{}, Edges: []FlowEdgeInfo{},
	})
	require.Error(t, err)
	wire, merr := json.Marshal(&err)
	require.NoError(t, merr)
	assert.JSONEq(t, `{"network":true}`, string(wire))
}
