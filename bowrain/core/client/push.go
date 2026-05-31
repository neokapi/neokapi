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

	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	bowsync "github.com/neokapi/neokapi/bowrain/core/sync"
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

const (
	// maxChunkRecords caps the number of blocks per chunk regardless of size.
	maxChunkRecords = 500

	// maxChunkMarshaledBytes bounds the *marshaled* (uncompressed) size of a
	// single chunk's blocks. The proxy upload path reads the request body with
	// io.LimitReader(body, 2 MiB) and silently truncates anything larger, which
	// would make the client-computed chunk hash diverge from the stored bytes
	// and fail the commit's BlobStore.Exists check.
	//
	// The 2 MiB cap applies to the *compressed* chunk. zstd never expands
	// real-world content, so a marshaled chunk kept under this threshold stays
	// safely under the compressed cap even in the pathological no-compression
	// case (incompressible payloads), with comfortable headroom for the proto
	// chunk envelope. Sizing the boundary from proto.Size of each block (which
	// accounts for source text, targets, annotations, skeleton, runs — the
	// whole serialized block) — rather than from SourceText alone — is what
	// prevents oversized chunks from slipping through.
	maxChunkMarshaledBytes = 1536 * 1024 // 1.5 MiB
)

// Push performs a complete push: init → diff → upload chunks → commit.
// blocksByItem maps item_name → blocks for that item; items is the item
// metadata.
//
// WIRE CONTRACT — additive-only push (#43).
//
// blocksByItem carries ONLY the blocks the caller determined have changed (the
// caller diffs against its local content-hash cache before calling Push, so an
// unchanged item is absent from the map entirely and a changed item carries
// only its changed blocks). The full per-item / per-project block set is NOT
// available at this layer.
//
// Consequently the ItemHashes / RootHash sent in the init request are computed
// over the changed subset, not the complete tree. They are therefore NOT
// authoritative Merkle roots: an item absent from blocksByItem is not a
// deletion, and an item hash computed from a partial block set will not match
// the server's hash over the item's full block set. The server MUST treat this
// push as additive — upsert the blocks it receives and never infer deletions
// from the client's hashes. The client deliberately IGNORES every deletion the
// server reports back (initResp.DeletedItems and diffResp.Deleted; see below),
// preserving non-destructiveness regardless of server behavior.
//
// If destructive sync (server-side deletion of blocks/items the client no
// longer has) is ever required, the caller must pass the FULL block set so
// ItemHashes / RootHash become authoritative, and this comment + the
// deletion-ignoring sites below must be revisited together.
func (c *BowrainClient) Push(ctx context.Context, blocksByItem map[string][]*model.Block, items []ItemMeta) (*SyncPushResponse, error) {
	// 1. Compute Merkle hashes over the changed subset only (additive-only
	//    contract above): these are change indicators, not authoritative roots.
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
	//
	// We act ONLY on ChangedItems + NewItems. initResp.DeletedItems is
	// deliberately ignored: per the additive-only contract, ItemHashes covers
	// only the changed subset, so an item the server flags "deleted" merely
	// reflects items absent from this push, not items the user removed. Acting
	// on it would be data loss.
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
		// diffResp.Deleted is deliberately ignored — same additive-only reason
		// as initResp.DeletedItems above: BlockHashes covers only the changed
		// subset, so a "deleted" block is just one not present in this push.
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
			// Estimate the boundary from the block's full marshaled proto size
			// (source + targets + annotations + skeleton + runs), not from
			// SourceText alone. Seal the current chunk *before* appending a block
			// that would push the marshaled size over the safe threshold, so the
			// chunk never exceeds the proxy upload's 2 MiB cap (#27). A single
			// block larger than the threshold still rides in its own chunk.
			sbSize := proto.Size(sb)
			if len(chunkBlocks) > 0 && chunkBytes+sbSize > maxChunkMarshaledBytes {
				ref, err := c.uploadChunk(ctx, initResp.UploadID, chunkIndex, "blocks", chunkBlocks, transport)
				if err != nil {
					return nil, fmt.Errorf("upload chunk %d: %w", chunkIndex, err)
				}
				chunks = append(chunks, *ref)
				chunkIndex++
				chunkBlocks = nil
				chunkBytes = 0
			}

			chunkBlocks = append(chunkBlocks, sb)
			chunkBytes += sbSize

			// Also cap by record count to keep chunks bounded for tiny blocks.
			if len(chunkBlocks) >= maxChunkRecords {
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
		data, err = c.compressor.Compress(data)
		if err != nil {
			return nil, fmt.Errorf("compress chunk: %w", err)
		}
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
