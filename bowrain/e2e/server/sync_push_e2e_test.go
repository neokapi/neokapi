//go:build e2e

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	bowsync "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/core/model"
)

// TestSyncPushE2E exercises the full sync push protocol against a live server+worker.
//
// Prerequisites:
//   - docker compose up -d (postgres, nats, redis)
//   - make dev-server (terminal 1)
//   - make dev-worker (terminal 2)
func TestSyncPushE2E(t *testing.T) {
	token := getTestToken(t)

	// 1. Create workspace + project.
	wsSlug := fmt.Sprintf("sync-e2e-%d", time.Now().UnixMilli())
	wsBody := fmt.Sprintf(`{"name":"Sync E2E","slug":"%s"}`, wsSlug)
	resp := apiRequest(t, http.MethodPost, "/api/v1/workspaces", token, wsBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	projBody := `{"name":"Sync Test","default_source_language":"en","target_languages":["fr"]}`
	resp = apiRequest(t, http.MethodPost, "/api/v1/"+wsSlug+"/projects", token, projBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	proj := readJSON(t, resp)
	projectID := proj["id"].(string)
	t.Logf("Created project %s in workspace %s", projectID, wsSlug)

	basePath := "/api/v1/" + wsSlug + "/projects/" + projectID

	// 2. Build blocks to push.
	b1 := &model.Block{ID: "greeting", Translatable: true}
	b1.SetSourceText("Hello, world!")
	b2 := &model.Block{ID: "farewell", Translatable: true}
	b2.SetSourceText("Goodbye!")

	blocks := []*model.Block{b1, b2}
	blockHashes := map[string]string{}
	for _, b := range blocks {
		identity := model.ComputeIdentity(b)
		blockHashes[b.ID] = identity.ContentHash
	}
	itemHash := bowsync.ComputeItemHash(blockHashes)
	rootHash := bowsync.ComputeRootHash(map[string]string{"en.json": itemHash})

	// 3. Init — send item hashes.
	initBody, _ := json.Marshal(map[string]any{
		"project_id":  projectID,
		"item_hashes": map[string]string{"en.json": itemHash},
		"root_hash":   rootHash,
	})
	resp = apiRequest(t, http.MethodPost, basePath+"/sync/push/init", token, string(initBody))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	initResp := readJSON(t, resp)
	t.Logf("Init response: status=%s", initResp["status"])

	require.Equal(t, "diff_computed", initResp["status"], "first push should compute diff")
	uploadID := initResp["upload_id"].(string)
	require.NotEmpty(t, uploadID)

	// 4. Diff — send block hashes for en.json.
	diffBody, _ := json.Marshal(map[string]any{
		"upload_id":    uploadID,
		"item_name":    "en.json",
		"block_hashes": blockHashes,
	})
	resp = apiRequest(t, http.MethodPost, basePath+"/sync/push/diff", token, string(diffBody))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	diffResp := readJSON(t, resp)
	needed := diffResp["needed"].([]any)
	t.Logf("Diff response: %d blocks needed, transport=%s", len(needed), diffResp["transport"])
	assert.Len(t, needed, 2, "both blocks should be needed")

	// 5. Upload chunk via proxy.
	var syncBlocks []*pb.SyncBlock
	for _, b := range blocks {
		syncBlocks = append(syncBlocks, bowsync.BlockToProto(b, "en.json"))
	}
	chunk := &pb.SyncChunk{
		ContentType: "blocks",
		RecordCount: int32(len(syncBlocks)),
		Blocks:      syncBlocks,
	}
	chunkData, err := proto.Marshal(chunk)
	require.NoError(t, err)

	chunkURL := fmt.Sprintf("%s/sync/push/chunks/%s/0", basePath, uploadID)
	req, _ := http.NewRequest(http.MethodPut, serverURL+chunkURL, nil)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Body = io.NopCloser(bytesReader(chunkData))
	req.ContentLength = int64(len(chunkData))
	chunkResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, chunkResp.StatusCode)
	chunkResp.Body.Close()
	t.Logf("Chunk uploaded: %d bytes", len(chunkData))

	// 6. Commit.
	rawHash := sha256.Sum256(chunkData)
	chunkHash := hex.EncodeToString(rawHash[:])

	itemsJSON, _ := json.Marshal([]map[string]string{{"name": "en.json", "format": "json"}})
	commitBody, _ := json.Marshal(map[string]any{
		"upload_id":      uploadID,
		"project_id":     projectID,
		"stream":         "main",
		"workspace_slug": wsSlug,
		"chunks": []map[string]any{{
			"index":        0,
			"content_type": "blocks",
			"hash":         chunkHash,
			"record_count": 2,
			"byte_size":    len(chunkData),
		}},
		"items": json.RawMessage(itemsJSON),
	})
	resp = apiRequest(t, http.MethodPost, basePath+"/sync/push/commit", token, string(commitBody))
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("push commit returned %d: %s", resp.StatusCode, string(body))
	}
	commitResp := readJSON(t, resp)
	pushID := commitResp["push_id"].(string)
	t.Logf("Push committed: push_id=%s", pushID)

	// 7. Poll push status until completed (worker must be running).
	var pushStatus string
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		statusResp := apiRequest(t, http.MethodGet,
			basePath+"/sync/status?push_id="+pushID, token, "")
		if statusResp.StatusCode != http.StatusOK {
			statusResp.Body.Close()
			continue
		}
		sr := readJSON(t, statusResp)
		pushStatus = sr["status"].(string)
		t.Logf("Push status: %s (completed=%v, failed=%v, in_progress=%v)",
			pushStatus, sr["completed"], sr["failed"], sr["in_progress"])
		if pushStatus == "completed" || pushStatus == "failed" {
			break
		}
	}
	require.Equal(t, "completed", pushStatus, "push should complete within 15s")

	// 8. Verify blocks via sync/blocks endpoint.
	blocksResp := apiRequest(t, http.MethodGet,
		basePath+"/sync/blocks?item_name=en.json", token, "")
	require.Equal(t, http.StatusOK, blocksResp.StatusCode)
	defer blocksResp.Body.Close()
	data, _ := io.ReadAll(blocksResp.Body)
	var storedBlocks []map[string]any
	require.NoError(t, json.Unmarshal(data, &storedBlocks))
	assert.Len(t, storedBlocks, 2, "should have 2 blocks stored")
	t.Logf("Verified: %d blocks stored in en.json", len(storedBlocks))

	// 9. Second push with same hashes → should be unchanged.
	resp = apiRequest(t, http.MethodPost, basePath+"/sync/push/init", token, string(initBody))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	initResp2 := readJSON(t, resp)
	assert.Equal(t, "unchanged", initResp2["status"], "second push with same hashes should be unchanged")
	t.Logf("Second push: status=%s (Merkle tree diff working)", initResp2["status"])
}

// bytesReader wraps a byte slice as an io.Reader.
func bytesReader(data []byte) io.Reader {
	return &bytesReaderImpl{data: data}
}

type bytesReaderImpl struct {
	data []byte
	pos  int
}

func (r *bytesReaderImpl) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
