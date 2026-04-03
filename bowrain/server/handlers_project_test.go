package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	cfg := DefaultServerConfig()

	cfg.JWTSecret = "test-secret"
	s := NewServer(cfg)
	initTestStores(t, s)

	require.NotNil(t, s.Services, "services should be initialized")
	require.NotNil(t, s.AuthStore, "auth store should be initialized")

	ctx := context.Background()
	user := &platauth.User{ID: "test-user", Email: "test@example.com", Name: "Test"}
	require.NoError(t, s.AuthStore.CreateUser(ctx, user))
	ws := &platauth.Workspace{ID: "test-ws", Name: "Test", Slug: "test", Type: platauth.WorkspaceTypePersonal}
	require.NoError(t, s.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, s.AuthStore.AddMember(ctx, ws.ID, user.ID, platauth.RoleOwner))

	token, err := platauth.GenerateToken(user, cfg.JWTSecret, 24*time.Hour)
	require.NoError(t, err)
	return s, token
}

func TestProjectCRUDEndpoints(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token

	// Create project (workspace-scoped: /api/v1/:ws/projects).
	body := `{"name":"Test","default_source_language":"en","target_languages":["fr","de"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var project store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &project))
	assert.Equal(t, "Test", project.Name)
	assert.NotEmpty(t, project.ID)
	projectID := project.ID

	// Get project (workspace-scoped: /api/v1/:ws/:pid).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/test/"+projectID, nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// List projects (workspace-scoped: /api/v1/:ws/projects).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/test/projects", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var projects []*store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projects))
	assert.Len(t, projects, 1)

	// Update project (workspace-scoped: /api/v1/:ws/:pid).
	body = `{"name":"Updated","default_source_language":"en","target_languages":["fr"]}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/test/"+projectID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Delete project (workspace-scoped: /api/v1/:ws/:pid).
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/test/"+projectID, nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}
