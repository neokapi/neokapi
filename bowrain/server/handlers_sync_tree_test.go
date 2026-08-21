package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type treeItemJSON struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Keys    []string `json:"keys"`
	Content []string `json:"content"`
	Record  []string `json:"record"`
}

type treeJSON struct {
	RootHash string         `json:"root_hash"`
	Items    []treeItemJSON `json:"items"`
}

func fetchTree(t *testing.T, e http.Handler, projectID, authHeader, query string) (treeJSON, *httptest.ResponseRecorder) {
	t.Helper()
	url := "/api/v1/projects/" + projectID + "/sync/main/tree" + query
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var out treeJSON
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	}
	return out, rec
}

// The tree is the fetch a push is built on: one request answers what the venue
// holds for the whole scope, which is what replaced a negotiation that
// descended one item at a time.
func TestSyncTree_AnswersForTheWholeStream(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	a1 := &model.Block{ID: "a1", Translatable: true}
	a1.SetSourceText("Hello")
	a2 := &model.Block{ID: "a2", Translatable: true}
	a2.SetSourceText("World")
	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Elsewhere")

	ctx := t.Context()
	require.NoError(t, srv.ContentStore.StoreBlocksForItem(ctx, pid, "main", "docs/a.json", []*model.Block{a1, a2}))
	require.NoError(t, srv.ContentStore.StoreBlocksForItem(ctx, pid, "main", "web/b.json", []*model.Block{b1}))

	tree, rec := fetchTree(t, e, pid, authHeader, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, tree.Items, 2)

	byPath := map[string]treeItemJSON{}
	for _, item := range tree.Items {
		byPath[item.Path] = item
	}

	docs := byPath["docs/a.json"]
	assert.Len(t, docs.Keys, 2)
	assert.Len(t, docs.Content, 2, "a content hash per block, so a rename can be recognised by what is inside the file")
	assert.Len(t, docs.Record, 2, "a transfer hash per block, so a producer can tell what is missing")
	assert.NotEmpty(t, docs.ID, "the venue names each item, because identity is the venue's to assign")

	assert.NotEmpty(t, tree.RootHash)
	assert.NotEmpty(t, rec.Header().Get("ETag"))
}

// A scope narrows the answer, so a `kapi push <subdir>` fetches what it is
// about to speak for rather than the whole project.
func TestSyncTree_ScopeNarrowsTheAnswer(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	inside := &model.Block{ID: "i1", Translatable: true}
	inside.SetSourceText("Inside")
	outside := &model.Block{ID: "o1", Translatable: true}
	outside.SetSourceText("Outside")

	ctx := t.Context()
	require.NoError(t, srv.ContentStore.StoreBlocksForItem(ctx, pid, "main", "docs/a.json", []*model.Block{inside}))
	require.NoError(t, srv.ContentStore.StoreBlocksForItem(ctx, pid, "main", "web/b.json", []*model.Block{outside}))

	tree, rec := fetchTree(t, e, pid, authHeader, "?scope=docs")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, tree.Items, 1)
	assert.Equal(t, "docs/a.json", tree.Items[0].Path)
}

// A warm producer revalidates in one conditional request instead of reading the
// whole tree back.
func TestSyncTree_RevalidatesAgainstItsTag(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	require.NoError(t, srv.ContentStore.StoreBlocksForItem(t.Context(), pid, "main", "a.json", []*model.Block{b}))

	_, first := fetchTree(t, e, pid, authHeader, "")
	require.Equal(t, http.StatusOK, first.Code)
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/tree", nil)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotModified, rec.Code)

	// And a tree that moved is served afresh, tag and all.
	b2 := &model.Block{ID: "b2", Translatable: true}
	b2.SetSourceText("World")
	require.NoError(t, srv.ContentStore.StoreBlocksForItem(t.Context(), pid, "main", "a.json", []*model.Block{b, b2}))

	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/tree", nil)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEqual(t, etag, rec.Header().Get("ETag"))
}
