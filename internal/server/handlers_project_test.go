package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := DefaultServerConfig()
	cfg.StorePath = ":memory:"
	s := NewServer(cfg)
	require.NotNil(t, s.Services, "services should be initialized with :memory: store")
	return s
}

func TestProjectCRUDEndpoints(t *testing.T) {
	srv := newTestServer(t)
	e := srv.GetEcho()

	// Create project.
	body := `{"name":"Test","source_locale":"en","target_locales":["fr","de"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// List projects.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
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
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Delete project.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+projectID, nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestBlockEndpoints(t *testing.T) {
	srv := newTestServer(t)
	e := srv.GetEcho()

	// Create project first.
	body := `{"name":"Test","source_locale":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var project store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &project))

	// Store blocks.
	blocksBody := `{"blocks":[{"id":"b1","text":"Hello"},{"id":"b2","text":"World"}]}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/blocks", strings.NewReader(blocksBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Get blocks.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/blocks", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestVersionEndpoints(t *testing.T) {
	srv := newTestServer(t)
	e := srv.GetEcho()

	// Create project + blocks.
	body := `{"name":"Test","source_locale":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var project store.Project
	json.Unmarshal(rec.Body.Bytes(), &project)

	blocksBody := `{"blocks":[{"id":"b1","text":"Hello"}]}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/blocks", strings.NewReader(blocksBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Create version.
	versionBody := `{"label":"v1.0","description":"Initial"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/versions", strings.NewReader(versionBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	// List versions.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/versions", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
