package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/platform/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRolesTestServer creates a test server with auth, a user as owner, and
// seeded default role templates.
func newRolesTestServer(t *testing.T) (*Server, string, string) {
	t.Helper()
	cfg := DefaultServerConfig()
	cfg.JWTSecret = "test-roles-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	ctx := t.Context()

	user := &platauth.User{Email: "admin@example.com", Name: "Admin"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, user))

	ws := &platauth.Workspace{Name: "Roles WS", Slug: "roles-ws"}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, srv.AuthStore.AddMember(ctx, ws.ID, user.ID, platauth.RoleOwner))
	require.NoError(t, srv.AuthStore.SeedDefaultRoleTemplates(ctx, ws.ID))

	token, err := platauth.GenerateToken(user, cfg.JWTSecret, 1*time.Hour)
	require.NoError(t, err)

	return srv, token, ws.Slug
}

func TestListRoleTemplates(t *testing.T) {
	srv, jwt, wsSlug := newRolesTestServer(t)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/"+wsSlug+"/roles", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var roles []json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &roles))
	assert.Len(t, roles, 5, "should return 5 default role templates")
}

func TestCreateRoleTemplate(t *testing.T) {
	srv, jwt, wsSlug := newRolesTestServer(t)
	e := srv.GetEcho()

	body := `{"name":"custom-role","display_name":"Custom Role","description":"A custom role","permissions":["translate","review"],"position":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/"+wsSlug+"/roles",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var created platauth.RoleTemplate
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "custom-role", created.Name)
	assert.Equal(t, "Custom Role", created.DisplayName)
	assert.NotEmpty(t, created.ID)

	// Verify it appears in list (5 defaults + 1 custom = 6).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/"+wsSlug+"/roles", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var roles []json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &roles))
	assert.Len(t, roles, 6)
}

func TestUpdateRoleTemplate(t *testing.T) {
	srv, jwt, wsSlug := newRolesTestServer(t)
	e := srv.GetEcho()

	// Create a custom role to update.
	body := `{"name":"updatable","display_name":"Updatable","permissions":["translate"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/"+wsSlug+"/roles",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created platauth.RoleTemplate
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	// Update name and permissions.
	updateBody := `{"name":"updated-role","display_name":"Updated Role","permissions":["translate","review","manage_tm"]}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/"+wsSlug+"/roles/"+created.ID,
		strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var updated platauth.RoleTemplate
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "updated-role", updated.Name)
	assert.Equal(t, "Updated Role", updated.DisplayName)
}

func TestDeleteRoleTemplate(t *testing.T) {
	srv, jwt, wsSlug := newRolesTestServer(t)
	e := srv.GetEcho()

	// Create a custom (non-builtin) role to delete.
	body := `{"name":"deletable","display_name":"Deletable","permissions":["translate"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/"+wsSlug+"/roles",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created platauth.RoleTemplate
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	// Delete it.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/"+wsSlug+"/roles/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	// Verify it's gone (back to 5 defaults).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/"+wsSlug+"/roles", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var roles []json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &roles))
	assert.Len(t, roles, 5)
}

func TestDeleteBuiltinRoleTemplate(t *testing.T) {
	srv, jwt, wsSlug := newRolesTestServer(t)
	e := srv.GetEcho()

	// List roles to find a builtin ID.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/"+wsSlug+"/roles", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var roles []platauth.RoleTemplate
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &roles))
	require.NotEmpty(t, roles)

	builtinID := roles[0].ID

	// Attempt to delete a builtin role — should fail.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/"+wsSlug+"/roles/"+builtinID, nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
