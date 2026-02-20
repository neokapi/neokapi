package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthEndpoint(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp HealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)
	assert.NotEmpty(t, resp.Version)
}

func TestConfigEndpoint(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ConfigResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "standalone", resp.Mode)
}

func TestConfigEndpointStandaloneMode(t *testing.T) {
	// Without JWTSecret, mode should be "standalone".
	cfg := DefaultServerConfig()
	srv := NewServer(cfg)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ConfigResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "standalone", resp.Mode)
}

func TestInfoEndpoint(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp InfoResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Version)
	assert.NotEmpty(t, resp.Commit)
	assert.NotEmpty(t, resp.BuildDate)
}

func TestListFormatsEndpoint(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/formats", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var formats []FormatInfo
	err := json.Unmarshal(rec.Body.Bytes(), &formats)
	require.NoError(t, err)
	require.NotEmpty(t, formats)

	// Verify some expected formats exist.
	formatNames := make(map[string]FormatInfo)
	for _, f := range formats {
		formatNames[f.Name] = f
	}

	expectedFormats := []string{"plaintext", "html", "xml", "json", "yaml", "xliff2", "csv", "srt", "vtt"}
	for _, name := range expectedFormats {
		info, ok := formatNames[name]
		assert.True(t, ok, "expected format %q not found", name)
		if ok {
			assert.True(t, info.HasReader, "format %q should have reader", name)
			assert.True(t, info.HasWriter, "format %q should have writer", name)
		}
	}

	// Verify sorted order.
	for i := 1; i < len(formats); i++ {
		assert.LessOrEqual(t, formats[i-1].Name, formats[i].Name, "formats should be sorted")
	}
}

func TestListToolsEndpoint(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var tools []ToolInfo
	err := json.Unmarshal(rec.Body.Bytes(), &tools)
	require.NoError(t, err)
	assert.NotNil(t, tools)
}

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "0.0.0.0", cfg.Host)
	assert.Empty(t, cfg.DataDir)
}

func TestConfigEndpointServerMode(t *testing.T) {
	// With JWTSecret, mode should be "server".
	cfg := DefaultServerConfig()
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ConfigResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "server", resp.Mode)
}

func TestNewServerCreatesRegistries(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	assert.NotNil(t, srv.FormatRegistry)
	assert.NotNil(t, srv.ToolRegistry)
	// Verify formats were registered.
	assert.True(t, srv.FormatRegistry.HasReader("plaintext"))
	assert.True(t, srv.FormatRegistry.HasWriter("plaintext"))
}
