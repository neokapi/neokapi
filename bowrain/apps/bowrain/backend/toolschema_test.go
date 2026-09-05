package backend

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetToolSchemaProxiesTheServerRoute(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		switch r.URL.Path {
		case "/api/v1/tools/term-check/schema":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"$id":"term-check","title":"Terminology Check","type":"object",` +
				`"ui:groups":[{"id":"main","label":"Main"}],` +
				`"properties":{"caseSensitive":{"type":"boolean","title":"Case Sensitive"}}}`))
		case "/api/v1/tools/broken/schema":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"tool schema not found"}`))
		}
	}))
	defer srv.Close()
	app := connectedApp(srv.URL)

	schema, err := app.GetToolSchema("term-check")
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok-123", gotAuth)
	assert.Equal(t, "/api/v1/tools/term-check/schema", gotPath)
	require.NotNil(t, schema)
	assert.Equal(t, "term-check", schema["$id"])
	// Extension fields travel as the server sent them.
	assert.NotNil(t, schema["ui:groups"])
	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "caseSensitive")

	// A tool without a schema is a nil document, as the server's 404 says.
	schema, err = app.GetToolSchema("no-such-tool")
	require.NoError(t, err)
	assert.Nil(t, schema)

	// Any other failure is reported, with the status the server gave.
	_, err = app.GetToolSchema("broken")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestGetToolSchemaOfflineReadsTheLocalRegistry(t *testing.T) {
	reg := registry.NewToolRegistry()
	libtools.RegisterAll(reg)
	app := &App{toolReg: reg}

	schema, err := app.GetToolSchema("term-check")
	require.NoError(t, err)
	require.NotNil(t, schema)
	assert.Equal(t, "term-check", schema["$id"])
	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "caseSensitive")

	schema, err = app.GetToolSchema("no-such-tool")
	require.NoError(t, err)
	assert.Nil(t, schema)

	// No registry at all still answers, with nothing.
	schema, err = (&App{}).GetToolSchema("term-check")
	require.NoError(t, err)
	assert.Nil(t, schema)
}
