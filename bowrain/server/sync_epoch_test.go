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

// epochInit posts a push-init to one stream stating a content-model
// generation, and returns the raw response so the refusal's status and message
// can be read.
func epochInit(t *testing.T, srv *Server, authHeader, projectID, stream string, epoch int, force bool) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"item_hashes":           map[string]string{},
		"content_model_epoch":   epoch,
		"allow_model_downgrade": force,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/"+projectID+"/sync/"+stream+"/push/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	return rec
}

// The content-model epoch, from the client's side of the wire.
//
// One server and one project across the cases, isolated by STREAM. Standing a
// server up runs the whole migration set into a fresh schema and this package
// pays that per test function — against a 15-minute budget it has already
// overrun once — and a second project is not available to pay it with anyway.
// The stream is the right seam regardless: the mark is recorded per stream, so
// two streams cannot see each other's.
func TestSyncPushInit_ContentModelEpoch(t *testing.T) {
	srv, token := newTestServer(t)
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// A committed push marks `main`, and from here it is the stream that holds
	// content from this generation.
	pushBlocks(t, srv, srv.GetEcho(), authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
	})

	// A push from a kapi whose content model is older than the stream's is
	// refused before it uploads anything: applying it would flatten structure a
	// richer kapi wrote, and nothing on the server can recover that.
	t.Run("an older content model is refused", func(t *testing.T) {
		require.Equal(t, http.StatusOK,
			epochInit(t, srv, authHeader, pid, "main", venue.ContentModelEpoch, false).Code,
			"the generation the stream holds is not a downgrade")

		rec := epochInit(t, srv, authHeader, pid, "main", venue.ContentModelEpoch-1, false)
		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Contains(t, rec.Body.String(), "newer kapi")
		assert.Contains(t, rec.Body.String(), "push --force",
			"a refusal has to say how to proceed deliberately")
	})

	// `kapi push --force` is the deliberate downgrade: it says the flattening
	// is what you meant.
	t.Run("force carries past the refusal", func(t *testing.T) {
		assert.Equal(t, http.StatusOK,
			epochInit(t, srv, authHeader, pid, "main", venue.ContentModelEpoch-1, true).Code)
	})

	// A stream nobody has pushed to holds nothing to downgrade, so the guard
	// must not turn a first push into a conflict.
	t.Run("a fresh stream refuses nobody", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, epochInit(t, srv, authHeader, pid, "fresh", 0, false).Code)
	})

	// The mark is set by a committed push, not by one that merely negotiated:
	// closing a stream on the strength of content that never arrived would lock
	// out producers who could still write it correctly.
	t.Run("an abandoned push does not close the stream", func(t *testing.T) {
		require.Equal(t, http.StatusOK,
			epochInit(t, srv, authHeader, pid, "abandoned", venue.ContentModelEpoch, false).Code)

		assert.Equal(t, http.StatusOK,
			epochInit(t, srv, authHeader, pid, "abandoned", venue.ContentModelEpoch-1, false).Code,
			"a negotiation that never committed marks nothing")
	})
}
