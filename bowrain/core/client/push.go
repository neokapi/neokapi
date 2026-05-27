package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/protobuf/proto"

	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	bowsync "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/core/model"
)

// PushInitRequest is the request for the Merkle tree diff negotiation.
type PushInitRequest struct {
	ProjectID  string            `json:"project_id"`
	Stream     string            `json:"stream"`
	ItemHashes map[string]string `json:"item_hashes"` // item_name → hash
	RootHash   string            `json:"root_hash"`
}

// PushInitResponse is the response from the init endpoint.
type PushInitResponse struct {
	UploadID           string   `json:"upload_id"`
	Status             string   `json:"status"` // "unchanged", "diff_computed"
	ChangedItems       []string `json:"changed_items"`
	NewItems           []string `json:"new_items"`
	DeletedItems       []string `json:"deleted_items"`
	UnchangedItemCount int      `json:"unchanged_item_count"`
}

// PushDiffRequest sends block-level hashes for one item.
type PushDiffRequest struct {
	UploadID    string            `json:"upload_id"`
	ItemName    string            `json:"item_name"`
	BlockHashes map[string]string `json:"block_hashes"`
}

// PushDiffResponse lists needed blocks and transport info.
type PushDiffResponse struct {
	Needed    []string `json:"needed"`
	Deleted   []string `json:"deleted"`
	Conflicts []string `json:"conflicts"`
	ChunkURLs []string `json:"chunk_urls"`
	Transport string   `json:"transport"` // "direct" or "proxy"
}

// PushCommitRequest finalizes the push.
type PushCommitRequest struct {
	UploadID      string          `json:"upload_id"`
	ProjectID     string          `json:"project_id"`
	Stream        string          `json:"stream"`
	Chunks        []ChunkRef      `json:"chunks"`
	Items         json.RawMessage `json:"items"`
	ActorID       string          `json:"actor_id"`
	WorkspaceSlug string          `json:"workspace_slug"`
}

type ChunkRef struct {
	Index       int    `json:"index"`
	ContentType string `json:"content_type"`
	Hash        string `json:"hash"`
	RecordCount int    `json:"record_count"`
	ByteSize    int64  `json:"byte_size"`
}

// Push performs a complete push: init → diff → upload chunks → commit.
// blocks is a map of item_name → blocks for that item.
// items is the item metadata.
func (c *BowrainClient) Push(ctx context.Context, blocksByItem map[string][]*model.Block, items []ItemMeta) (*SyncPushResponse, error) {
	// 1. Compute Merkle hashes.
	itemHashes := make(map[string]string)
	blockHashesByItem := make(map[string]map[string]string)
	for itemName, blocks := range blocksByItem {
		blockHashes := make(map[string]string, len(blocks))
		for _, b := range blocks {
			identity := model.ComputeIdentity(b)
			blockHashes[b.ID] = identity.ContentHash
		}
		blockHashesByItem[itemName] = blockHashes
		itemHashes[itemName] = bowsync.ComputeItemHash(blockHashes)
	}
	rootHash := bowsync.ComputeRootHash(itemHashes)

	// 2. Init — send item hashes.
	initResp, err := c.pushInit(ctx, PushInitRequest{
		ItemHashes: itemHashes,
		RootHash:   rootHash,
	})
	if err != nil {
		return nil, fmt.Errorf("push init: %w", err)
	}
	if initResp.Status == "unchanged" {
		return &SyncPushResponse{PushID: "unchanged"}, nil
	}

	// 3. For each changed/new item, send block-level diff and collect needed blocks.
	allNeeded := map[string]map[string]bool{} // item → set of needed block IDs
	diffItems := make([]string, 0, len(initResp.ChangedItems)+len(initResp.NewItems))
	diffItems = append(diffItems, initResp.ChangedItems...)
	diffItems = append(diffItems, initResp.NewItems...)
	transport := "proxy"

	for _, itemName := range diffItems {
		hashes := blockHashesByItem[itemName]
		if hashes == nil {
			continue
		}
		diffResp, err := c.pushDiff(ctx, PushDiffRequest{
			UploadID:    initResp.UploadID,
			ItemName:    itemName,
			BlockHashes: hashes,
		})
		if err != nil {
			return nil, fmt.Errorf("push diff for %s: %w", itemName, err)
		}
		needed := map[string]bool{}
		for _, id := range diffResp.Needed {
			needed[id] = true
		}
		allNeeded[itemName] = needed
		if diffResp.Transport != "" {
			transport = diffResp.Transport
		}
	}

	// 4. Build and upload chunks (only needed blocks).
	var chunks []ChunkRef
	chunkIndex := 0

	for itemName, neededIDs := range allNeeded {
		blocks := blocksByItem[itemName]
		var chunkBlocks []*pb.SyncBlock
		chunkBytes := 0

		for _, b := range blocks {
			if !neededIDs[b.ID] {
				continue
			}
			sb := bowsync.BlockToProto(b, itemName)
			chunkBlocks = append(chunkBlocks, sb)
			chunkBytes += len(sb.SourceText) * 2 // rough estimate

			// Seal chunk at ~1MB.
			if chunkBytes >= 1024*1024 || len(chunkBlocks) >= 500 {
				ref, err := c.uploadChunk(ctx, initResp.UploadID, chunkIndex, "blocks", chunkBlocks, transport)
				if err != nil {
					return nil, fmt.Errorf("upload chunk %d: %w", chunkIndex, err)
				}
				chunks = append(chunks, *ref)
				chunkIndex++
				chunkBlocks = nil
				chunkBytes = 0
			}
		}

		// Flush remaining blocks.
		if len(chunkBlocks) > 0 {
			ref, err := c.uploadChunk(ctx, initResp.UploadID, chunkIndex, "blocks", chunkBlocks, transport)
			if err != nil {
				return nil, fmt.Errorf("upload chunk %d: %w", chunkIndex, err)
			}
			chunks = append(chunks, *ref)
			chunkIndex++
		}
	}

	// 5. Commit.
	itemsJSON, _ := json.Marshal(items)
	commitResp, err := c.pushCommit(ctx, PushCommitRequest{
		UploadID: initResp.UploadID,
		Chunks:   chunks,
		Items:    itemsJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("push commit: %w", err)
	}

	return commitResp, nil
}

func (c *BowrainClient) pushInit(ctx context.Context, req PushInitRequest) (*PushInitResponse, error) {
	body, _ := json.Marshal(req)
	u := c.streamPrefix() + "/push/init"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.doRequest(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("push init HTTP %d: %s", resp.StatusCode, string(b))
	}
	var result PushInitResponse
	return &result, json.NewDecoder(resp.Body).Decode(&result)
}

func (c *BowrainClient) pushDiff(ctx context.Context, req PushDiffRequest) (*PushDiffResponse, error) {
	body, _ := json.Marshal(req)
	u := c.streamPrefix() + "/push/diff"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.doRequest(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("push diff HTTP %d: %s", resp.StatusCode, string(b))
	}
	var result PushDiffResponse
	return &result, json.NewDecoder(resp.Body).Decode(&result)
}

func (c *BowrainClient) uploadChunk(ctx context.Context, uploadID string, index int, contentType string, blocks []*pb.SyncBlock, transport string) (*ChunkRef, error) {
	chunk := &pb.SyncChunk{
		ContentType: contentType,
		RecordCount: int32(len(blocks)),
		Blocks:      blocks,
	}
	data, err := proto.Marshal(chunk)
	if err != nil {
		return nil, fmt.Errorf("marshal chunk: %w", err)
	}

	// Compress with zstd.
	if c.compressor != nil {
		data = c.compressor.Compress(data)
	}

	// Upload via proxy (local dev) — direct SAS upload can be added later.
	u := c.streamPrefix() + fmt.Sprintf("/push/chunks/%s/%d", uploadID, index)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.doRequest(httpReq)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chunk upload HTTP %d", resp.StatusCode)
	}

	// Compute the chunk's content-addressed key for the manifest. This MUST
	// match how the blob store keys uploaded bytes (plain SHA-256 of the exact
	// bytes uploaded) so the worker can Download(chunk.Hash). Do NOT use
	// model.ComputeContentHash here — it TrimSpace-normalizes its input, which
	// corrupts the hash of binary (compressed) chunk data.
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	return &ChunkRef{
		Index:       index,
		ContentType: contentType,
		Hash:        hash,
		RecordCount: len(blocks),
		ByteSize:    int64(len(data)),
	}, nil
}

func (c *BowrainClient) pushCommit(ctx context.Context, req PushCommitRequest) (*SyncPushResponse, error) {
	body, _ := json.Marshal(req)
	u := c.streamPrefix() + "/push/commit"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.doRequest(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("push commit HTTP %d: %s", resp.StatusCode, string(b))
	}
	var result SyncPushResponse
	return &result, json.NewDecoder(resp.Body).Decode(&result)
}
