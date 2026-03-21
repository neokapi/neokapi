package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/platform/auth"
	"github.com/neokapi/neokapi/platform/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newProjectMembersTestServer creates a test server with auth, a user as owner,
// seeded role templates, and a project. Returns the server, JWT, workspace slug,
// workspace ID, and project ID.
func newProjectMembersTestServer(t *testing.T) (*Server, string, string, string, string) {
	t.Helper()
	cfg := DefaultServerConfig()
	cfg.JWTSecret = "test-pm-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	ctx := t.Context()

	user := &platauth.User{Email: "pm-admin@example.com", Name: "PM Admin"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, user))

	ws := &platauth.Workspace{Name: "PM WS", Slug: "pm-ws"}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, srv.AuthStore.AddMember(ctx, ws.ID, user.ID, platauth.RoleOwner))
	require.NoError(t, srv.AuthStore.SeedDefaultRoleTemplates(ctx, ws.ID))

	token, err := platauth.GenerateToken(user, cfg.JWTSecret, 1*time.Hour)
	require.NoError(t, err)

	e := srv.GetEcho()

	// Create a project via the workspace-scoped editor route.
	body := `{"name":"Members Test","default_source_language":"en","target_languages":["fr","de"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+ws.Slug+"/editor/projects",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "create project: %s", rec.Body.String())

	var project store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &project))

	return srv, token, ws.Slug, ws.ID, project.ID
}

func TestListProjectMembers(t *testing.T) {
	srv, jwt, wsSlug, _, pid := newProjectMembersTestServer(t)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/workspaces/"+wsSlug+"/editor/projects/"+pid+"/members", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var members []*platauth.ProjectMembership
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &members))
	assert.Empty(t, members, "project should have no explicit members initially")
}

func TestAddProjectMember(t *testing.T) {
	srv, jwt, wsSlug, wsID, pid := newProjectMembersTestServer(t)
	e := srv.GetEcho()
	ctx := t.Context()

	// Create a second user to add as member.
	member := &platauth.User{Email: "translator@example.com", Name: "Translator"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, member))

	// Find the translator role template.
	templates, err := srv.AuthStore.ListRoleTemplates(ctx, wsID)
	require.NoError(t, err)
	var translatorRoleID string
	for _, rt := range templates {
		if rt.Name == "translator" {
			translatorRoleID = rt.ID
			break
		}
	}
	require.NotEmpty(t, translatorRoleID, "translator role template should exist")

	body := `{"user_id":"` + member.ID + `","role_id":"` + translatorRoleID + `","languages":["fr"]}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/workspaces/"+wsSlug+"/editor/projects/"+pid+"/members",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "add member: %s", rec.Body.String())

	var pm platauth.ProjectMembership
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pm))
	assert.Equal(t, member.ID, pm.UserID)
	assert.Equal(t, translatorRoleID, pm.RoleID)
	assert.Equal(t, []string{"fr"}, pm.Languages)

	// Verify member appears in list.
	req = httptest.NewRequest(http.MethodGet,
		"/api/v1/workspaces/"+wsSlug+"/editor/projects/"+pid+"/members", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var members []*platauth.ProjectMembership
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &members))
	assert.Len(t, members, 1)
}

func TestUpdateProjectMember(t *testing.T) {
	srv, jwt, wsSlug, wsID, pid := newProjectMembersTestServer(t)
	e := srv.GetEcho()
	ctx := t.Context()

	// Create a second user and add as member.
	member := &platauth.User{Email: "reviewer@example.com", Name: "Reviewer"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, member))

	templates, err := srv.AuthStore.ListRoleTemplates(ctx, wsID)
	require.NoError(t, err)
	var translatorRoleID, reviewerRoleID string
	for _, rt := range templates {
		switch rt.Name {
		case "translator":
			translatorRoleID = rt.ID
		case "reviewer":
			reviewerRoleID = rt.ID
		}
	}
	require.NotEmpty(t, translatorRoleID)
	require.NotEmpty(t, reviewerRoleID)

	// Add as translator.
	body := `{"user_id":"` + member.ID + `","role_id":"` + translatorRoleID + `","languages":["fr"]}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/workspaces/"+wsSlug+"/editor/projects/"+pid+"/members",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Update to reviewer with different languages.
	updateBody := `{"role_id":"` + reviewerRoleID + `","languages":["fr","de"]}`
	req = httptest.NewRequest(http.MethodPut,
		"/api/v1/workspaces/"+wsSlug+"/editor/projects/"+pid+"/members/"+member.ID,
		strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "update member: %s", rec.Body.String())

	var updated platauth.ProjectMembership
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, reviewerRoleID, updated.RoleID)
	assert.Equal(t, []string{"fr", "de"}, updated.Languages)
}

func TestRemoveProjectMember(t *testing.T) {
	srv, jwt, wsSlug, wsID, pid := newProjectMembersTestServer(t)
	e := srv.GetEcho()
	ctx := t.Context()

	// Create and add a member.
	member := &platauth.User{Email: "removable@example.com", Name: "Removable"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, member))

	templates, err := srv.AuthStore.ListRoleTemplates(ctx, wsID)
	require.NoError(t, err)
	var roleID string
	for _, rt := range templates {
		if rt.Name == "translator" {
			roleID = rt.ID
			break
		}
	}

	body := `{"user_id":"` + member.ID + `","role_id":"` + roleID + `"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/workspaces/"+wsSlug+"/editor/projects/"+pid+"/members",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Remove the member.
	req = httptest.NewRequest(http.MethodDelete,
		"/api/v1/workspaces/"+wsSlug+"/editor/projects/"+pid+"/members/"+member.ID, nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	// Verify member is removed.
	req = httptest.NewRequest(http.MethodGet,
		"/api/v1/workspaces/"+wsSlug+"/editor/projects/"+pid+"/members", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var members []*platauth.ProjectMembership
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &members))
	assert.Empty(t, members)
}
