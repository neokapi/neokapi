package server

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClientServerSyncContract runs the real bowrain HTTP client
// (bowrain/core/client) against a real in-process server — the full echo router,
// real handlers, real auth — with NO mocking of routes.
//
// This is the contract test the codebase was missing. The CLI's
// CreateAuthenticatedProject once POSTed to a flat /api/v1/projects route the
// server never exposed; its unit test *mocked that flat route*, so it stayed
// green while `kapi init --server` 404'd against a live server. Exercising the
// client against srv.GetEcho() makes that class of drift fail here instead of in
// production: every client call below hits an actual registered route, so a
// route rename/removal on either side breaks this test.
func TestClientServerSyncContract(t *testing.T) {
	srv, token := newTestServer(t) // creates user + "test" workspace + JWT
	ts := httptest.NewServer(srv.GetEcho())
	defer ts.Close()
	ctx := context.Background()

	// 1. ListWorkspaces — GET /api/v1/workspaces.
	wss, err := client.ListWorkspaces(ts.URL, token)
	require.NoError(t, err, "ListWorkspaces must hit a real route")
	require.NotEmpty(t, wss, "the test user owns one workspace")
	var haveTestWS bool
	for _, w := range wss {
		if w.Slug == "test" {
			haveTestWS = true
		}
	}
	assert.True(t, haveTestWS, "expected the 'test' workspace from newTestServer")

	// 2. CreateAuthenticatedProject — resolves the workspace, then POSTs the
	//    workspace-scoped /api/v1/:ws/projects (AD-011). PRE-FIX this 404'd
	//    because the client used the non-existent flat /api/v1/projects.
	projectID, wsSlug, err := client.CreateAuthenticatedProject(
		ts.URL, token, "Integration Project", "en", []string{"fr", "de"}, "")
	require.NoError(t, err, "create must hit the workspace-scoped route, not a flat 404")
	require.NotEmpty(t, projectID)
	assert.Equal(t, "test", wsSlug)
	// (The explicit-workspace create path is covered by the client unit test;
	// the test workspace plan caps at one project, so we don't create a second.)

	// 3. Push — workspace-scoped sync route /api/v1/:ws/:pid/sync/:ref/...
	c := client.NewWorkspaceBowrainClient(ts.URL, "test", projectID, token)
	block := &model.Block{
		ID:           "b1",
		Name:         "greeting",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: "Hello, world"}}},
	}
	pushResp, err := c.Push(ctx,
		map[string][]*model.Block{"locales/en.json": {block}},
		[]client.ItemMeta{{Name: "locales/en.json", Format: "json"}},
	)
	require.NoError(t, err, "push must hit the real sync route (no nil-client panic, no 404)")
	require.NotNil(t, pushResp)
	// (Block-storage/Merkle-diff counting is exercised by push_chunking_test.go /
	// push_merkle_test.go; here we assert the route + auth contract holds.)

	// 4. Pull — same workspace-scoped sync route.
	pullResp, err := c.Pull(ctx, 0, []string{"fr"}, 100)
	require.NoError(t, err, "pull must hit the real sync route")
	require.NotNil(t, pullResp)
}

// TestPushNilClientGuard documents the robustness fix: Push on a nil client
// (a project with no server: block) returns a clear error instead of a
// nil-pointer panic in projectPrefix/streamPrefix.
func TestPushNilClientGuard(t *testing.T) {
	var c *client.BowrainClient // nil — never connected
	_, err := c.Push(context.Background(),
		map[string][]*model.Block{"x": {{ID: "b1", Source: []model.Run{{Text: &model.TextRun{Text: "hi"}}}}}},
		[]client.ItemMeta{{Name: "x", Format: "json"}},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected to a server")
}
