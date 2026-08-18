package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reviewer asks the collection, not the project: a repository publishes one
// preview host per surface it ships, so the desktop app's components and the
// web app's are two hosts and each collection names its own.
// One server for both cases; see TestSyncPush_PropertyDelivery for why this
// package shares them.
func TestCollectionResponse_Preview(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)

	t.Run("carries the preview host, per collection", func(t *testing.T) {
		collectionResponseCarriesThePreviewHost(t, srv, token, pid)
	})
	t.Run("a half-stored host is none", func(t *testing.T) {
		collectionResponseHalfStoredHostIsNone(t, srv, token, pid)
	})
}

func collectionResponseCarriesThePreviewHost(t *testing.T, srv *Server, token, pid string) {
	t.Helper()
	ctx := t.Context()

	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &store.Collection{
		ProjectID: pid, Name: "bowrain-app", Stream: "main",
		PreviewKind: "storybook",
		PreviewURL:  "https://neokapi.github.io/storybook/bowrain/",
	}))
	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &store.Collection{
		ProjectID: pid, Name: "neokapi-desktop", Stream: "main",
		PreviewKind: "storybook",
		PreviewURL:  "https://neokapi.github.io/storybook/kapi/",
	}))
	require.NoError(t, srv.ContentStore.CreateCollection(ctx, &store.Collection{
		ProjectID: pid, Name: "neokapi-docs", Stream: "main",
	}))

	byName := listCollections(t, srv, token, pid)

	require.NotNil(t, byName["bowrain-app"].Preview)
	assert.Equal(t, "storybook", byName["bowrain-app"].Preview.Kind)
	assert.Equal(t, "https://neokapi.github.io/storybook/bowrain/", byName["bowrain-app"].Preview.URL)

	require.NotNil(t, byName["neokapi-desktop"].Preview)
	assert.Equal(t, "https://neokapi.github.io/storybook/kapi/", byName["neokapi-desktop"].Preview.URL,
		"each collection answers with its own host, which is the whole reason this is not a project setting")

	assert.Nil(t, byName["neokapi-docs"].Preview,
		"a collection that declares none offers no in-context reading")
}

// A half-stored host is served as none. The recipe loader refuses one, so a row
// holding one was written by something else — and a URL with no kind is a host
// nobody knows how to find a view inside.
func collectionResponseHalfStoredHostIsNone(t *testing.T, srv *Server, token, pid string) {
	t.Helper()

	require.NoError(t, srv.ContentStore.CreateCollection(t.Context(), &store.Collection{
		ProjectID: pid, Name: "half", Stream: "main",
		PreviewURL: "https://neokapi.github.io/storybook/bowrain/",
	}))

	assert.Nil(t, listCollections(t, srv, token, pid)["half"].Preview)
}

func listCollections(t *testing.T, srv *Server, token, projectID string) map[string]CollectionResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/"+projectID+"/collections/main", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got []CollectionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	byName := map[string]CollectionResponse{}
	for _, c := range got {
		byName[c.Name] = c
	}
	return byName
}
