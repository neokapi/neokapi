package backend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/neokapi/neokapi/host/venue/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

// previewFixture is one of the screen fixtures host/screens writes, the JSON
// a Bowrain server answers each screen route with.
func previewFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "packages", "project-screens", "fixtures", name))
	require.NoError(t, err)
	return data
}

// previewServer answers the four screen routes of project acme/p1 with the
// recorded JSON, to the bearer token "tok" alone, and refuses the project
// acme/locked with the server's 403 body.
func previewServer(t *testing.T) *httptest.Server {
	t.Helper()
	routes := map[string][]byte{
		"/api/v1/acme/p1/map/main":          previewFixture(t, "map-summary.json"),
		"/api/v1/acme/p1/reader/main":       previewFixture(t, "reader-file.json"),
		"/api/v1/acme/p1/rules/main/decide": previewFixture(t, "rules-decide-view.json"),
		"/api/v1/acme/p1/rules/main/kept":   previewFixture(t, "kept-rules.json"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid token"}`))
			return
		}
		if r.URL.Path == "/api/v1/acme/locked/map/main" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"insufficient project permissions"}`))
			return
		}
		if r.URL.Path == "/api/v1/acme/p1/reader/main" && r.URL.Query().Get("item") != "docs/billing.md" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no such item"}`))
			return
		}
		body, ok := routes[r.URL.Path]
		if !ok || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"project not found"}`))
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// previewApp is an app with one tab whose recipe connects to url.
func previewApp(t *testing.T, url string) *App {
	t.Helper()
	return venueApp(&project.KapiProject{
		Extras: map[string]yaml.Node{schema.VenueKey: serverExtra(t, "url: "+url+"\n")},
	})
}

// signIn stores a sign-in for server with token, in a throwaway config
// directory and a keychain held in memory.
func signIn(t *testing.T, server, token string) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
	t.Setenv("BOWRAIN_AUTH_TOKEN", "")
	require.NoError(t, config.SaveAuth(config.StoredAuth{
		ServerURL: server, AccessToken: token, User: config.StoredUser{Name: "Mira Holt"},
	}))
}

func TestBowrainPreviewReadsEachPart(t *testing.T) {
	srv := previewServer(t)
	signIn(t, srv.URL, "tok")
	a := previewApp(t, srv.URL+"/acme/p1")

	got, err := a.BowrainPreview("t", PreviewMap, "")
	require.NoError(t, err)
	require.Nil(t, got.Refusal)
	require.NotNil(t, got.Map)
	assert.Equal(t, "acme/docs", got.Map.ProjectKey)
	assert.Equal(t, 3, got.Map.Files)

	got, err = a.BowrainPreview("t", PreviewReader, "docs/billing.md")
	require.NoError(t, err)
	require.Nil(t, got.Refusal)
	require.NotNil(t, got.Reader)
	assert.Equal(t, "docs/billing.md", got.Reader.Path)

	got, err = a.BowrainPreview("t", PreviewDecide, "")
	require.NoError(t, err)
	require.Nil(t, got.Refusal)
	require.NotNil(t, got.Decide)
	assert.NotEmpty(t, got.Decide.Decisions)

	got, err = a.BowrainPreview("t", PreviewKept, "")
	require.NoError(t, err)
	require.Nil(t, got.Refusal)
	require.Len(t, got.Kept, 3)
	assert.Equal(t, "ada", got.Kept[1].KeptBy.Name, "the server's member names come through")
}

func TestBowrainPreviewRefusals(t *testing.T) {
	srv := previewServer(t)

	t.Run("the server does not take the sign-in", func(t *testing.T) {
		signIn(t, srv.URL, "expired")
		got, err := previewApp(t, srv.URL+"/acme/p1").BowrainPreview("t", PreviewMap, "")
		require.NoError(t, err)
		assert.Equal(t, &PreviewRefusal{Status: 401, Message: "invalid token", Action: PreviewActionSignIn}, got.Refusal)
		assert.Nil(t, got.Map)
	})
	t.Run("no permission on the project", func(t *testing.T) {
		signIn(t, srv.URL, "tok")
		got, err := previewApp(t, srv.URL+"/acme/locked").BowrainPreview("t", PreviewMap, "")
		require.NoError(t, err)
		assert.Equal(t, &PreviewRefusal{Status: 403, Message: "insufficient project permissions", Action: PreviewActionOpen}, got.Refusal)
	})
	t.Run("no such project", func(t *testing.T) {
		signIn(t, srv.URL, "tok")
		got, err := previewApp(t, srv.URL+"/acme/gone").BowrainPreview("t", PreviewKept, "")
		require.NoError(t, err)
		assert.Equal(t, &PreviewRefusal{Status: 404, Message: "project not found", Action: PreviewActionOpen}, got.Refusal)
	})
	t.Run("no sign-in on this Mac", func(t *testing.T) {
		keyring.MockInit()
		t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
		t.Setenv("BOWRAIN_AUTH_TOKEN", "")
		got, err := previewApp(t, srv.URL+"/acme/p1").BowrainPreview("t", PreviewMap, "")
		require.NoError(t, err)
		assert.Equal(t, &PreviewRefusal{Action: PreviewActionSignIn}, got.Refusal)
	})
	t.Run("the sign-in names another server, which is sent nothing", func(t *testing.T) {
		signIn(t, "https://other.example.com", "tok")
		got, err := previewApp(t, srv.URL+"/acme/p1").BowrainPreview("t", PreviewMap, "")
		require.NoError(t, err)
		assert.Equal(t, &PreviewRefusal{Action: PreviewActionSignIn}, got.Refusal)
	})
}

func TestBowrainPreviewRefusesWhatItCannotRead(t *testing.T) {
	signIn(t, "https://bowrain.example.com", "tok")
	a := previewApp(t, "https://bowrain.example.com/acme/p1")

	_, err := a.BowrainPreview("t", "settings", "")
	require.Error(t, err)
	_, err = a.BowrainPreview("t", PreviewReader, "")
	require.Error(t, err, "the reader part names a file")
	_, err = a.BowrainPreview("missing", PreviewMap, "")
	require.Error(t, err)

	_, err = previewApp(t, "http://bowrain.example.com/acme/p1").BowrainPreview("t", PreviewMap, "")
	require.ErrorContains(t, err, "unencrypted")
	_, err = previewApp(t, "https://bowrain.example.com/projects/p1").BowrainPreview("t", PreviewMap, "")
	require.ErrorContains(t, err, "no project in a workspace")
	_, err = venueApp(&project.KapiProject{}).BowrainPreview("t", PreviewMap, "")
	require.ErrorContains(t, err, "no Bowrain server")
}

func TestSignedInTo(t *testing.T) {
	signIn(t, "https://bowrain.example.com", "tok")
	ok, who := signedInTo("https://bowrain.example.com/acme/p1")
	assert.True(t, ok)
	assert.Equal(t, "Mira Holt", who)
	ok, _ = signedInTo("https://other.example.com/acme/p1")
	assert.False(t, ok)
}
