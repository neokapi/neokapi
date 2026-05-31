package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newOutsiderToken creates a user who belongs to a DIFFERENT workspace and has
// no membership in the test workspace ("test-ws"), modelling a separate tenant.
func newOutsiderToken(t *testing.T, s *Server, id, email string) string {
	t.Helper()
	ctx := t.Context()
	u := &platauth.User{ID: id, Email: email, Name: email}
	require.NoError(t, s.AuthStore.CreateUser(ctx, u))
	ws := &platauth.Workspace{ID: id + "-ws", Name: email, Slug: id + "ws", Type: platauth.WorkspaceTypePersonal}
	require.NoError(t, s.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, s.AuthStore.AddMember(ctx, ws.ID, u.ID, platauth.RoleOwner))
	token, err := platauth.GenerateToken(u, "test-secret", time.Hour)
	require.NoError(t, err)
	return token
}

func syncReq(t *testing.T, s *Server, method, target, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, r)
	return rec
}

// TestFlatSyncIDOR_OutsiderDenied verifies that an authenticated user from a
// different tenant cannot read or write another tenant's project through the
// flat sync routes (object-level authz / IDOR regression, finding #1).
func TestFlatSyncIDOR_OutsiderDenied(t *testing.T) {
	srv, ownerToken := newTestServer(t)
	e := srv.GetEcho()
	pid := createProject(t, srv, ownerToken)

	// Seed real content so a successful read would actually exfiltrate data.
	pushBlocks(t, srv, e, "Bearer "+ownerToken, pid, []pushBlockItem{
		{ID: "b1", Text: "TopSecret", ItemName: "en.json"},
	})

	outsider := newOutsiderToken(t, srv, "outsider", "outsider@example.com")

	t.Run("pull-denied", func(t *testing.T) {
		rec := syncReq(t, srv, http.MethodGet,
			"/api/v1/projects/"+pid+"/sync/main/pull?cursor=0", outsider, "")
		assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assert.NotContains(t, rec.Body.String(), "TopSecret",
			"outsider must not see another tenant's content")
	})

	t.Run("blocks-denied", func(t *testing.T) {
		rec := syncReq(t, srv, http.MethodGet,
			"/api/v1/projects/"+pid+"/sync/main/blocks?item_name=en.json", outsider, "")
		assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assert.NotContains(t, rec.Body.String(), "TopSecret")
	})

	t.Run("push-init-denied", func(t *testing.T) {
		rec := syncReq(t, srv, http.MethodPost,
			"/api/v1/projects/"+pid+"/sync/main/push/init", outsider,
			`{"item_hashes":{},"root_hash":""}`)
		assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	})

	t.Run("status-denied", func(t *testing.T) {
		rec := syncReq(t, srv, http.MethodGet,
			"/api/v1/projects/"+pid+"/sync/main/status?push_id=x", outsider, "")
		assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	})

	// Sanity: the legitimate owner is NOT blocked (no over-restriction).
	t.Run("owner-allowed", func(t *testing.T) {
		rec := syncReq(t, srv, http.MethodGet,
			"/api/v1/projects/"+pid+"/sync/main/pull?cursor=0", ownerToken, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "TopSecret")
	})
}

// TestFlatSyncCommit_IgnoresClientProjectID verifies the commit handler binds
// the job to the path-scoped project, ignoring a forged project_id in the
// manifest body (finding #1: trusts client-supplied identifiers).
func TestFlatSyncCommit_IgnoresClientProjectID(t *testing.T) {
	srv, ownerToken := newTestServer(t)
	pid := createProject(t, srv, ownerToken)

	const forgedPID = "some-other-tenant-project"

	// Owner commits to their OWN project path but forges a different project_id
	// in the body. The enqueued job must target the path project, not the
	// client-supplied one.
	body := `{"upload_id":"u1","project_id":"` + forgedPID + `","stream":"main","chunks":[],"items":{}}`
	rec := syncReq(t, srv, http.MethodPost,
		"/api/v1/projects/"+pid+"/sync/main/push/commit", ownerToken, body)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pushID, _ := resp["push_id"].(string)
	require.NotEmpty(t, pushID)

	// The enqueued job must target the path project, not the forged one.
	jobList, err := srv.JobStore.ListJobsByPushID(t.Context(), pushID)
	require.NoError(t, err)
	require.Len(t, jobList, 1)
	assert.Equal(t, pid, jobList[0].ProjectID,
		"commit must bind to the path-scoped project, not the client-supplied project_id")
	assert.NotEqual(t, forgedPID, jobList[0].ProjectID)
}
