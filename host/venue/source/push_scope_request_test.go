package source

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/ref/refcache"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// scopeServer is a venue that holds stored items, records the scope a push
// sends with its tree fetch and its commit, and plans removals from that commit
// the way the venue's sync worker does: a stored item the declared tree does not
// name is removed when the declared scope covers its path.
type scopeServer struct {
	*httptest.Server
	stored []venue.TreeItem

	mu         sync.Mutex
	treeScopes [][]string
	commits    []apiclient.PushCommitRequest
	removals   []string
}

func newScopeServer(t *testing.T, projectID string, stored []venue.TreeItem) *scopeServer {
	t.Helper()
	ss := &scopeServer{stored: stored}
	published := ref.Ref{Content: 1}
	mux := http.NewServeMux()
	base := "/api/v1/projects/" + projectID
	mux.HandleFunc(base, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ProjectMetadata{ID: projectID, DefaultSourceLanguage: "en", TargetLanguages: []string{"fr"}})
	})
	mux.HandleFunc(base+"/sync/main/ref", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(published)
	})
	mux.HandleFunc(base+"/sync/main/status", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"push_id": "p1", "status": "completed", "total": 1, "completed": 1})
	})
	mux.HandleFunc(base+"/sync/main/tree", func(w http.ResponseWriter, r *http.Request) {
		ss.mu.Lock()
		ss.treeScopes = append(ss.treeScopes, r.URL.Query()["scope"])
		ss.mu.Unlock()
		current := published
		_ = json.NewEncoder(w).Encode(apiclient.TreeResponse{Ref: &current, Items: ss.stored})
	})
	mux.HandleFunc(base+"/sync/main/push/chunks/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(base+"/sync/main/push/init", func(w http.ResponseWriter, _ *http.Request) {
		current := published
		_ = json.NewEncoder(w).Encode(apiclient.PushInitResponse{UploadID: "u1", Status: "diff_computed", Ref: &current})
	})
	mux.HandleFunc(base+"/sync/main/push/commit", func(w http.ResponseWriter, r *http.Request) {
		var manifest apiclient.PushCommitRequest
		_ = json.NewDecoder(r.Body).Decode(&manifest)
		ss.mu.Lock()
		ss.commits = append(ss.commits, manifest)
		for _, item := range ss.stored {
			if manifest.Tree[item.Path].Path != "" {
				continue
			}
			if manifest.Scope.Covers(item.Path) {
				ss.removals = append(ss.removals, item.Path)
			}
		}
		sort.Strings(ss.removals)
		ss.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"push_id": "p1", "status": "queued"})
	})
	ss.Server = httptest.NewServer(mux)
	t.Cleanup(ss.Close)
	return ss
}

// scopeConnector is a project with one translated collection and one collection
// declared for its comments alone, pointed at srv.
func scopeConnector(t *testing.T, srv *httptest.Server, projectID string) *BowrainSourceConnector {
	t.Helper()
	var only coreproj.ContentComments
	require.NoError(t, yaml.Unmarshal([]byte("{only: true}"), &only))
	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
		Collections: []coreproj.Collection{
			{Name: "app", Content: []coreproj.ContentItem{{Path: "locales/en/*.json", Target: "locales/{lang}/{filename}", Format: &coreproj.FormatSpec{Name: "json"}}}},
			{Name: "config", Content: []coreproj.ContentItem{{Path: "config/*.yaml", Comments: only}}},
		},
		Server: &bproject.ServerSpec{URL: srv.URL + "/projects/" + projectID, Stream: "main"},
	})
	require.NoError(t, err)
	for rel, body := range map[string]string{
		"locales/en/app.json": `{"greeting":"Hello"}`,
		"config/app.yaml":     "# Greets the reader.\ngreeting: Hello\n",
	} {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(body), 0o644))
	}
	client := apiclient.NewProjectBearerClient(srv.URL, projectID, "test-token")
	client.SetStream("main")
	conn := &BowrainSourceConnector{
		app:       testApp(t),
		project:   proj,
		client:    client,
		formatReg: reg,
		cache:     bproject.LoadSyncCache(proj.Layout),
		refs:      refcache.Load(proj.Layout, config.NormalizeServerURL(srv.URL), projectID),
		stream:    "main",
		maxBatch:  1000,
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// The push request itself declares no pattern of a collection declared for its
// comments alone: neither the tree fetch's scope nor the commit's.
func TestPushRequestDeclaresNoCommentsOnlyPattern(t *testing.T) {
	srv := newScopeServer(t, "proj1", nil)
	conn := scopeConnector(t, srv.Server, "proj1")

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)

	require.NotEmpty(t, srv.treeScopes, "the push fetched the venue's tree")
	for _, scope := range srv.treeScopes {
		assert.Contains(t, scope, "locales/en/*.json", "the translated collection declares its pattern")
		assert.NotContains(t, scope, "config/*.yaml", "the tree fetch carries no comments-only pattern")
	}
	require.Len(t, srv.commits, 1, "the push committed")
	assert.Contains(t, []string(srv.commits[0].Scope), "locales/en/*.json")
	assert.NotContains(t, []string(srv.commits[0].Scope), "config/*.yaml", "the commit declares no comments-only pattern")
}

// A venue can hold an item under a path the recipe now declares for its comments
// alone: a file an earlier recipe pushed. No pushed collection claims it, so a
// push must leave it alone, while the same push still removes a file its own
// collection no longer has.
func TestPushDeletesNothingUnderACommentsOnlyPattern(t *testing.T) {
	srv := newScopeServer(t, "proj1", []venue.TreeItem{
		{Path: "locales/en/app.json", ID: "item-app", Keys: []string{"greeting"}, Content: []string{"sha-app-old"}},
		{Path: "locales/en/gone.json", ID: "item-gone", Keys: []string{"farewell"}, Content: []string{"sha-gone"}},
		{Path: "config/app.yaml", ID: "item-config", Keys: []string{"greeting"}, Content: []string{"sha-config"}},
	})
	conn := scopeConnector(t, srv.Server, "proj1")

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)

	require.Len(t, srv.commits, 1, "the push committed")
	assert.Equal(t, []string{"locales/en/gone.json"}, srv.removals,
		"the push removes the file its collection no longer has, and nothing under the comments-only pattern")
}
