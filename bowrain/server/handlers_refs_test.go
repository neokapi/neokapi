package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListProjectRefs_UnifiedStreamsAndTags verifies the /:id/refs endpoint
// returns BOTH streams and tags (finding #59: previously aliased to the
// tags-only handler).
func TestListProjectRefs_UnifiedStreamsAndTags(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push content so "main" exists as a stream.
	pushBlocks(t, srv, e, authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
	})

	// Create an extra stream so the listing has >1 stream.
	streamReq := httptest.NewRequest(http.MethodPost, "/api/v1/test/"+pid+"/streams",
		strings.NewReader(`{"name":"feature/x","parent":"main"}`))
	streamReq.Header.Set("Content-Type", "application/json")
	streamReq.Header.Set("Authorization", authHeader)
	streamRec := httptest.NewRecorder()
	e.ServeHTTP(streamRec, streamReq)
	require.Equal(t, http.StatusCreated, streamRec.Code, streamRec.Body.String())

	// Create a tag on main.
	tagReq := httptest.NewRequest(http.MethodPost, "/api/v1/test/"+pid+"/tags",
		strings.NewReader(`{"name":"v1.0","stream":"main","kind":"release"}`))
	tagReq.Header.Set("Content-Type", "application/json")
	tagReq.Header.Set("Authorization", authHeader)
	tagRec := httptest.NewRecorder()
	e.ServeHTTP(tagRec, tagReq)
	require.Equal(t, http.StatusCreated, tagRec.Code, tagRec.Body.String())

	// List refs.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/"+pid+"/refs", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var refs []ProjectRef
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &refs))

	var streamNames, tagNames []string
	for _, r := range refs {
		switch r.Type {
		case "stream":
			streamNames = append(streamNames, r.Name)
		case "tag":
			tagNames = append(tagNames, r.Name)
		}
	}

	assert.Contains(t, streamNames, "main", "refs must include the main stream")
	assert.Contains(t, streamNames, "feature/x", "refs must include created streams")
	assert.Contains(t, tagNames, "v1.0", "refs must include tags (not just streams)")
}
