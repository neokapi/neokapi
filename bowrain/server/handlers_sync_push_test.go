package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	syncengine "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestSyncPush_Init_Unchanged(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push some blocks first.
	pushBlocks(t, srv, e, authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
	})

	// Compute the root hash matching server state.
	diffEngine := syncengine.NewDiffEngine(srv.ContentStore, nil)
	ctx := t.Context()
	itemHashes, err := diffEngine.ExportItemHashes(ctx, pid, "main")
	require.NoError(t, err)
	rootHash := venue.ComputeRootHash(itemHashes)

	// Init with matching root hash → unchanged.
	body, _ := json.Marshal(map[string]any{
		"project_id":  pid,
		"item_hashes": itemHashes,
		"root_hash":   rootHash,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/main/push/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "unchanged", resp["status"])
}

func TestSyncPush_Init_DiffComputed(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push initial content.
	pushBlocks(t, srv, e, authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
	})

	// Init with a different item hash → diff computed.
	body, _ := json.Marshal(map[string]any{
		"project_id":  pid,
		"item_hashes": map[string]string{"en.json": "different-hash"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/main/push/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "diff_computed", resp["status"])
	assert.NotEmpty(t, resp["upload_id"])
	changed := resp["changed_items"].([]any)
	assert.Contains(t, changed, "en.json")
}

func TestSyncPush_FullPushFlow(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// 1. Init — new project, all items are new.
	initBody, _ := json.Marshal(map[string]any{
		"project_id":  pid,
		"item_hashes": map[string]string{"en.json": "new-hash"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/main/push/init", bytes.NewReader(initBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var initResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initResp))
	uploadID := initResp["upload_id"].(string)
	assert.NotEmpty(t, uploadID)

	// 2. Diff — all blocks are new.
	diffBody, _ := json.Marshal(map[string]any{
		"upload_id":    uploadID,
		"item_name":    "en.json",
		"block_hashes": map[string]string{"b1": "hash1", "b2": "hash2"},
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/main/push/diff", bytes.NewReader(diffBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var diffResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &diffResp))
	needed := diffResp["needed"].([]any)
	assert.Len(t, needed, 2) // both blocks are new

	// 3. Upload chunk via proxy.
	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello")
	b2 := &model.Block{ID: "b2", Translatable: true}
	b2.SetSourceText("World")

	chunk := &pb.SyncChunk{
		ContentType: "blocks",
		RecordCount: 2,
		Blocks: []*pb.SyncBlock{
			venue.BlockToProto(b1, "en.json"),
			venue.BlockToProto(b2, "en.json"),
		},
	}
	chunkData, err := proto.Marshal(chunk)
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPut,
		"/api/v1/projects/"+pid+"/sync/main/push/chunks/"+uploadID+"/0",
		bytes.NewReader(chunkData))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// 4. Commit.
	// The chunk hash is the raw SHA-256 of the uploaded bytes — what the client
	// sends and what the content-addressed store keys on. Not
	// model.ComputeContentHash, which trims surrounding whitespace before
	// hashing: a marshalled SyncChunk starts with 0x0a, so trimming yields a
	// digest that names no stored blob. That went unnoticed while the commit
	// handler skipped its chunk-existence check for chunked stores.
	rawHash := sha256.Sum256(chunkData)
	chunkHash := hex.EncodeToString(rawHash[:])
	itemsJSON, _ := json.Marshal([]map[string]string{
		{"name": "en.json", "format": "json"},
	})
	commitBody, _ := json.Marshal(map[string]any{
		"upload_id":  uploadID,
		"project_id": pid,
		"stream":     "main",
		"chunks": []map[string]any{
			{"index": 0, "content_type": "blocks", "hash": chunkHash, "record_count": 2, "byte_size": len(chunkData)},
		},
		"items": json.RawMessage(itemsJSON),
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/main/push/commit", bytes.NewReader(commitBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var commitResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &commitResp))
	assert.NotEmpty(t, commitResp["push_id"])
}

func TestSyncPush_UploadBudgetEnforced(t *testing.T) {
	srv, token := newTestServer(t)
	// Set a very small budget.
	srv.Config.MaxPushBytes = 100
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Commit with chunks exceeding the budget.
	itemsJSON, _ := json.Marshal([]map[string]string{
		{"name": "en.json", "format": "json"},
	})
	commitBody, _ := json.Marshal(map[string]any{
		"upload_id":  "test-upload",
		"project_id": pid,
		"stream":     "main",
		"chunks": []map[string]any{
			{"index": 0, "content_type": "blocks", "hash": "abc", "record_count": 1, "byte_size": 200},
		},
		"items": json.RawMessage(itemsJSON),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/main/push/commit", bytes.NewReader(commitBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "upload budget exceeded")
}

// TestSyncPush_CommitRejectsChunkNotInStorage pins that a commit manifest may
// only name chunks this server actually stored. The check was previously
// skipped whenever the blob store implemented ChunkedBlobStore — which the
// local store used on every self-hosted deployment does — so the hash travelled
// to the worker unexamined. The hashes here are well formed; what they lack is
// a preceding upload.
func TestSyncPush_CommitRejectsChunkNotInStorage(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{"digest that was never uploaded", strings.Repeat("ab", 32)},
		{"path traversal in place of a digest", "../../../../etc/passwd"},
		{"relative path into the blob root", "../_uploads/x"},
		{"absolute path", "/etc/passwd"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, token := newTestServer(t)
			e := srv.GetEcho()
			authHeader := "Bearer " + token
			pid := createProject(t, srv, token)

			itemsJSON, _ := json.Marshal([]map[string]string{
				{"name": "en.json", "format": "json"},
			})
			commitBody, _ := json.Marshal(map[string]any{
				"upload_id":  "test-upload",
				"project_id": pid,
				"stream":     "main",
				"chunks": []map[string]any{
					{"index": 0, "content_type": "blocks", "hash": tt.hash, "record_count": 1, "byte_size": 10},
				},
				"items": json.RawMessage(itemsJSON),
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/main/push/commit", bytes.NewReader(commitBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", authHeader)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code, "commit must not be queued")
			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Contains(t, resp["error"], "not found in storage")
		})
	}
}
