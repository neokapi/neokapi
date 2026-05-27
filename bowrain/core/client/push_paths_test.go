package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPushUsesCorrectSyncPathsAndChunkHash guards two regressions that silently
// broke the entire sync protocol against the server's AD-011 routes:
//
//  1. streamPrefix() already includes "/sync/<ref>"; the per-endpoint URLs must
//     append "/push/init" (etc.), NOT "/sync/push/init". A doubled "/sync/"
//     segment 404s on the server.
//  2. The manifest chunk hash must equal the plain SHA-256 of the exact bytes
//     uploaded (the blob store's content-addressing key). Hashing with a
//     normalizing function (e.g. model.ComputeContentHash, which TrimSpaces)
//     corrupts binary chunk hashes so the worker can't Download the blob.
func TestPushUsesCorrectSyncPathsAndChunkHash(t *testing.T) {
	var paths []string
	var uploadedChunk []byte
	var committedChunks []ChunkRef

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/push/init"):
			_ = json.NewEncoder(w).Encode(PushInitResponse{
				UploadID: "up1", Status: "diff_computed", NewItems: []string{"locales/en.json"},
			})
		case strings.HasSuffix(r.URL.Path, "/push/diff"):
			_ = json.NewEncoder(w).Encode(PushDiffResponse{Needed: []string{"b1"}, Transport: "proxy"})
		case strings.Contains(r.URL.Path, "/push/chunks/"):
			uploadedChunk, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/push/commit"):
			var req PushCommitRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			committedChunks = req.Chunks
			_ = json.NewEncoder(w).Encode(SyncPushResponse{PushID: "p1", Stored: 1})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClaimTokenClient(srv.URL, "proj1", "tok")
	blk := &model.Block{
		ID:           "b1",
		Name:         "greeting",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: "Hello"}}},
	}
	_, err := c.Push(context.Background(),
		map[string][]*model.Block{"locales/en.json": {blk}},
		[]ItemMeta{{Name: "locales/en.json", Format: "json"}})
	require.NoError(t, err)

	// Guard #1: no doubled "/sync/", and all sync calls hit the AD-011 prefix.
	for _, p := range paths {
		assert.NotContains(t, p, "/sync/main/sync/", "client built a double-sync path: %s", p)
		assert.Contains(t, p, "/api/v1/projects/proj1/sync/main/", "unexpected path: %s", p)
	}
	assert.Contains(t, paths, "/api/v1/projects/proj1/sync/main/push/init")
	assert.Contains(t, paths, "/api/v1/projects/proj1/sync/main/push/commit")

	// Guard #2: committed chunk hash == SHA-256 of the uploaded bytes.
	require.Len(t, committedChunks, 1)
	require.NotEmpty(t, uploadedChunk)
	sum := sha256.Sum256(uploadedChunk)
	assert.Equal(t, hex.EncodeToString(sum[:]), committedChunks[0].Hash,
		"manifest chunk hash must equal the blob store key (plain SHA-256 of uploaded bytes)")
}
