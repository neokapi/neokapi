package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/gokapi/gokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	s := NewServer(cfg)
	require.NotNil(t, s.Services, "services should be initialized")
	require.NotNil(t, s.AuthStore, "auth store should be initialized")

	ctx := context.Background()
	user := &auth.User{ID: "test-user", Email: "test@example.com", Name: "Test"}
	require.NoError(t, s.AuthStore.CreateUser(ctx, user))
	ws := &auth.Workspace{ID: "test-ws", Name: "Test", Slug: "test", Type: auth.WorkspaceTypePersonal}
	require.NoError(t, s.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, s.AuthStore.AddMember(ctx, ws.ID, user.ID, auth.RoleOwner))

	token, err := auth.GenerateToken(user, cfg.JWTSecret, 24*time.Hour)
	require.NoError(t, err)
	return s, token
}

func TestProjectCRUDEndpoints(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token

	// Create project.
	body := `{"name":"Test","source_locale":"en","target_locales":["fr","de"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
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

	// Get project.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID, nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// List projects.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var projects []*store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projects))
	assert.Len(t, projects, 1)

	// Update project.
	body = `{"name":"Updated","source_locale":"en","target_locales":["fr"]}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Delete project.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+projectID, nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestBlockEndpoints(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token

	// Create project first.
	body := `{"name":"Test","source_locale":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var project store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &project))

	// Store blocks.
	blocksBody := `{"blocks":[{"id":"b1","text":"Hello"},{"id":"b2","text":"World"}]}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/blocks", strings.NewReader(blocksBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Get blocks.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/blocks", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestVersionEndpoints(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token

	// Create project + blocks.
	body := `{"name":"Test","source_locale":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var project store.Project
	_ = json.Unmarshal(rec.Body.Bytes(), &project)

	blocksBody := `{"blocks":[{"id":"b1","text":"Hello"}]}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/blocks", strings.NewReader(blocksBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Create version.
	versionBody := `{"label":"v1.0","description":"Initial"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/versions", strings.NewReader(versionBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	// List versions.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/versions", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
