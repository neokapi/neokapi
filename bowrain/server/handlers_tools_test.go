package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getToolSchema(t *testing.T, srv *Server, name string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools/"+url.PathEscape(name)+"/schema", nil)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	return rec
}

func TestToolSchemaEndpoint(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))

	rec := getToolSchema(t, srv, "term-check")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	// The body is the registry's own document, extension fields included.
	registered := srv.ToolRegistry.Schema("term-check")
	require.NotNil(t, registered)
	assert.JSONEq(t, string(registered.RawJSON), rec.Body.String())

	var body struct {
		ID         string                    `json:"$id"`
		Title      string                    `json:"title"`
		ToolMeta   map[string]any            `json:"toolMeta"`
		Properties map[string]map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "term-check", body.ID)
	assert.NotEmpty(t, body.Title)
	assert.Equal(t, "term-check", body.ToolMeta["id"])
	assert.Contains(t, body.Properties, "caseSensitive")
	assert.Equal(t, "boolean", body.Properties["caseSensitive"]["type"])
}

func TestToolSchemaEndpointUnknownTool(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))

	rec := getToolSchema(t, srv, "no-such-tool")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "tool schema not found", body.Error)
}

func TestToolSchemaEndpointToolWithoutSchema(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	// A tool the registry knows only by its metadata carries no schema; the
	// route answers as it does for an unknown tool.
	srv.ToolRegistry.RegisterMetadata("bare-tool", nil, "test")
	require.True(t, srv.ToolRegistry.Has("bare-tool"))

	rec := getToolSchema(t, srv, "bare-tool")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestToolSchemaEndpointAgreesWithInfo(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var info InfoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
	require.NotEmpty(t, info.Tools)

	// /info's hasSchema is the promise the schema route keeps: every tool
	// marked as carrying one answers 200, every other one 404.
	for _, tool := range info.Tools {
		want := http.StatusNotFound
		if tool.HasSchema {
			want = http.StatusOK
		}
		assert.Equal(t, want, getToolSchema(t, srv, string(tool.Name)).Code, "tool %q", tool.Name)
	}
}

func TestToolSchemaEndpointIsPublicInServerMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := shutdownOnCleanup(t, NewServer(cfg))

	// No credentials on the request: the schema is served under the same
	// access as /info, which the web app reads before anyone signs in.
	rec := getToolSchema(t, srv, "pseudo-translate")
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, http.StatusNotFound, getToolSchema(t, srv, "no-such-tool").Code)
}
