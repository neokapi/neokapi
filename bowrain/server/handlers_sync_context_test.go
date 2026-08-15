package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/store"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pushInitContext posts a push-init carrying only the context content type's
// half of the negotiation, and returns the decoded response.
func pushInitContext(t *testing.T, srv *Server, authHeader, projectID, contextHash string, collections []string) apiclient.PushInitResponse {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"context_hash": contextHash,
		"collections":  collections,
		"item_hashes":  map[string]string{},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/"+projectID+"/sync/main/push/init", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var out apiclient.PushInitResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

// serverContextHash folds the entry hashes a set of stored collections would
// produce, which is what a client negotiating "unchanged" has to send.
func serverContextHash(collections map[string]string) string {
	return venue.ComputeContextHash(collections)
}

// TestSyncPushInit_ContextNegotiation covers the fast path from the server's
// side: a matching context hash is reported unchanged, a differing one is
// reported changed, and an absent one makes no claim at all.
func TestSyncPushInit_ContextNegotiation(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)
	ctx := t.Context()

	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &store.Collection{
		ProjectID: pid, Name: "docs", Stream: "main",
		Owner: venue.ContextOwnerRecipe, ContextHash: "hash-docs",
	}))

	matching := serverContextHash(map[string]string{"docs": "hash-docs"})

	t.Run("a matching hash is unchanged", func(t *testing.T) {
		out := pushInitContext(t, srv, authHeader, pid, matching, []string{"docs"})
		assert.False(t, out.ContextChanged)
	})

	t.Run("a differing hash is changed", func(t *testing.T) {
		out := pushInitContext(t, srv, authHeader, pid,
			serverContextHash(map[string]string{"docs": "hash-docs-v2"}), []string{"docs"})
		assert.True(t, out.ContextChanged)
	})

	t.Run("no hash makes no claim", func(t *testing.T) {
		out := pushInitContext(t, srv, authHeader, pid, "", nil)
		assert.False(t, out.ContextChanged)
		assert.Empty(t, out.UndeclaredCollections,
			"a push that says nothing about collections must not be read as dropping them all")
	})
}

// TestSyncPushInit_ReportsUndeclaredCollections pins the mirror of
// deleted_items: the server names the recipe-owned collections a push stopped
// declaring, and a workspace-owned one is never in that list because it was
// never the recipe's to declare.
func TestSyncPushInit_ReportsUndeclaredCollections(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)
	ctx := t.Context()

	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &store.Collection{
		ProjectID: pid, Name: "docs", Stream: "main",
		Owner: venue.ContextOwnerRecipe, ContextHash: "hash-docs",
	}))
	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &store.Collection{
		ProjectID: pid, Name: "marketing", Stream: "main",
		Owner: venue.ContextOwnerRecipe, ContextHash: "hash-marketing",
	}))
	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &store.Collection{
		ProjectID: pid, Name: "uploads", Stream: "main",
		Owner: venue.ContextOwnerWorkspace,
	}))

	out := pushInitContext(t, srv, authHeader, pid,
		serverContextHash(map[string]string{"docs": "hash-docs"}), []string{"docs"})

	assert.Equal(t, []string{"marketing"}, out.UndeclaredCollections)

	// Reported is not deleted: both rows are still there afterwards.
	for _, name := range []string{"marketing", "uploads"} {
		col, err := srv.ContentStore.GetCollectionByName(ctx, pid, name, "main")
		require.NoError(t, err)
		require.NotNil(t, col, "%s must survive a push that did not declare it", name)
	}
}

// TestSyncPull_CarriesContextEntries pins the pull half: every collection
// travels with its owner, because ownership is what the client keys its own
// decisions on — withholding the rows it may not act on would leave it unable
// to tell "not mine to touch" from "not there".
func TestSyncPull_CarriesContextEntries(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)
	ctx := t.Context()

	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &store.Collection{
		ProjectID: pid, Name: "docs", Stream: "main",
		Owner:       venue.ContextOwnerRecipe,
		ContextHash: "hash-docs",
		Context:     map[string]string{"product": "kapi"},
		ConnectorConfig: map[string]string{
			coreprofile.PropertyChannel: "docs",
		},
	}))
	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &store.Collection{
		ProjectID: pid, Name: "uploads", Stream: "main",
		Owner: venue.ContextOwnerWorkspace,
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/pull", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var out apiclient.RichPullResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	byName := map[string]*pb.SyncContextEntry{}
	for _, e := range out.Contexts {
		byName[e.Name] = e
	}

	docs := byName["docs"]
	require.NotNil(t, docs)
	assert.Equal(t, venue.ContextOwnerRecipe, docs.Owner)
	assert.Equal(t, map[string]string{"product": "kapi"}, docs.Coordinates)
	assert.Equal(t, "docs", docs.Channel)
	assert.Equal(t, "hash-docs", docs.ContentHash)

	uploads := byName["uploads"]
	require.NotNil(t, uploads)
	assert.Equal(t, venue.ContextOwnerWorkspace, uploads.Owner)

	// The collection every project is created with predates the recipe and is
	// carried as the workspace's, which is what stops a recipe that never
	// mentions it from being handed authority over it.
	def := byName["default"]
	require.NotNil(t, def, "the project's own default collection travels too")
	assert.Equal(t, venue.ContextOwnerWorkspace, def.Owner)
}
