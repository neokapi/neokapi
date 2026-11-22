package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A push's scope can list hundreds of excluded files, which as a query string
// passes the CDN's URL limit, so the scope travels in a POST body.
func TestTreeSendsTheScopeInTheBody(t *testing.T) {
	scope := venue.Scope{"docs/**/*.md"}
	for i := range 500 {
		scope = append(scope, "!docs/assets/image-"+strings.Repeat("x", 20)+string(rune('a'+i%26))+".png")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/ws/p1/sync/main/tree", r.URL.Path)
		assert.Empty(t, r.URL.RawQuery)
		var body struct {
			Scope []string `json:"scope"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, []string(scope), body.Scope)
		_, _ = w.Write([]byte(`{"root_hash":"r","items":[]}`))
	}))
	defer srv.Close()

	tree, err := NewWorkspaceBowrainClient(srv.URL, "ws", "p1", "tok").Tree(t.Context(), scope)
	require.NoError(t, err)
	assert.Equal(t, "r", tree.RootHash)
}

// A server that predates the POST route gets the scope as a query string.
func TestTreeFallsBackToAQueryOnAnOlderServer(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		assert.Equal(t, []string{"docs/**"}, r.URL.Query()["scope"])
		_, _ = w.Write([]byte(`{"root_hash":"r","items":[]}`))
	}))
	defer srv.Close()

	_, err := NewWorkspaceBowrainClient(srv.URL, "ws", "p1", "tok").Tree(t.Context(), venue.Scope{"docs/**"})
	require.NoError(t, err)
	assert.Equal(t, []string{http.MethodPost, http.MethodGet}, methods)
}

// With no scope there is nothing to carry, and the fetch stays a plain GET.
func TestTreeWithoutAScopeIsAGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Empty(t, r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"root_hash":"r","items":[]}`))
	}))
	defer srv.Close()

	_, err := NewWorkspaceBowrainClient(srv.URL, "ws", "p1", "tok").Tree(t.Context(), nil)
	require.NoError(t, err)
}
