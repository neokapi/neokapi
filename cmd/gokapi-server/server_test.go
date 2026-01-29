package main

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
	e := srv.Echo()

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

func TestListFormatsEndpoint(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	e := srv.Echo()

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
	e := srv.Echo()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var tools []ToolInfo
	err := json.Unmarshal(rec.Body.Bytes(), &tools)
	require.NoError(t, err)
	// Empty tools list is valid for the base server.
	assert.NotNil(t, tools)
}

func TestListFlowsEndpoint(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	e := srv.Echo()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/flows", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var flows []FlowInfo
	err := json.Unmarshal(rec.Body.Bytes(), &flows)
	require.NoError(t, err)
	assert.NotNil(t, flows)
}

func TestConvertEndpointMissingFormat(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	e := srv.Echo()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/convert", nil)
	req.Header.Set("Content-Type", "multipart/form-data")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp ErrorResponse
	err := json.Unmarshal(rec.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Contains(t, errResp.Error, "input_format and output_format are required")
}

func TestTranslateEndpointMissingLang(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	e := srv.Echo()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/translate", nil)
	req.Header.Set("Content-Type", "multipart/form-data")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp ErrorResponse
	err := json.Unmarshal(rec.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Contains(t, errResp.Error, "source_lang and target_lang are required")
}

func TestFlowExecuteEndpointMissingFlowName(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	e := srv.Echo()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/flow/execute", nil)
	req.Header.Set("Content-Type", "multipart/form-data")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp ErrorResponse
	err := json.Unmarshal(rec.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Contains(t, errResp.Error, "flow_name is required")
}

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "0.0.0.0", cfg.Host)
	assert.Empty(t, cfg.DataDir)
}

func TestNewServerCreatesRegistries(t *testing.T) {
	srv := NewServer(DefaultServerConfig())
	assert.NotNil(t, srv.formatRegistry)
	assert.NotNil(t, srv.toolRegistry)
	// Verify formats were registered.
	assert.True(t, srv.formatRegistry.HasReader("plaintext"))
	assert.True(t, srv.formatRegistry.HasWriter("plaintext"))
}
