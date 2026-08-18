package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// epochInit posts a push-init stating a content-model generation, and returns
// the raw response so the refusal's status and message can be read.
func epochInit(t *testing.T, srv *Server, authHeader, projectID string, epoch int, force bool) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"item_hashes":           map[string]string{},
		"content_model_epoch":   epoch,
		"allow_model_downgrade": force,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/"+projectID+"/sync/main/push/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	return rec
}

// A push from a kapi whose content model is older than the stream's is refused
// before it uploads anything, because applying it would flatten structure a
// richer kapi wrote and nothing on the server can recover it.
func TestSyncPushInit_AnOlderContentModelIsRefused(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// A current kapi pushes, which marks the stream.
	pushBlocks(t, srv, srv.GetEcho(), authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
	})

	require.Equal(t, http.StatusOK, epochInit(t, srv, authHeader, pid, venue.ContentModelEpoch, false).Code,
		"the generation the stream holds is not a downgrade")

	rec := epochInit(t, srv, authHeader, pid, venue.ContentModelEpoch-1, false)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "newer kapi")
	assert.Contains(t, rec.Body.String(), "push --force",
		"a refusal has to say how to proceed deliberately")
}

// `kapi push --force` is the deliberate downgrade: it says the flattening is
// what you meant.
func TestSyncPushInit_ForceCarriesPastTheRefusal(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	pushBlocks(t, srv, srv.GetEcho(), authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
	})

	rec := epochInit(t, srv, authHeader, pid, venue.ContentModelEpoch-1, true)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// A project nobody has pushed to holds nothing to downgrade, so the guard must
// not turn a first push into a conflict.
func TestSyncPushInit_AFreshStreamRefusesNobody(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	assert.Equal(t, http.StatusOK, epochInit(t, srv, authHeader, pid, 0, false).Code)
}

// The mark is set by a committed push, not by one that merely negotiated:
// closing a stream to older producers on the strength of content that never
// arrived would lock them out of a stream they could still write correctly.
func TestSyncPushInit_AnAbandonedPushDoesNotCloseTheStream(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	require.Equal(t, http.StatusOK,
		epochInit(t, srv, authHeader, pid, venue.ContentModelEpoch, false).Code)

	assert.Equal(t, http.StatusOK,
		epochInit(t, srv, authHeader, pid, venue.ContentModelEpoch-1, false).Code,
		"a negotiation that never committed marks nothing")
}
