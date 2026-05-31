package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proxyBodyCap mirrors the server's io.LimitReader(body, 2 MiB) on the proxy
// upload path. A chunk whose uploaded (compressed) body exceeds this is
// silently truncated server-side, breaking the content-addressed hash.
const proxyBodyCap = 2 * 1024 * 1024

// blockWithHeavyPayload builds a block whose translatable SourceText is tiny but
// whose serialized proto is large because of bulky target runs, a skeleton and
// properties. The old chunkBytes += len(SourceText)*2 estimator counts ~0 bytes
// for such a block, so a chunk full of them blows past the 2 MiB proxy cap.
func blockWithHeavyPayload(id string, payloadBytes int) *model.Block {
	bulk := strings.Repeat("x", payloadBytes)
	half := strings.Repeat("y", payloadBytes/2)
	b := &model.Block{
		ID:           id,
		Name:         id,
		Translatable: true,
		// Tiny source — defeats any SourceText-based size estimate.
		Source: []model.Run{{Text: &model.TextRun{Text: "hi"}}},
		// Heavy target runs.
		Targets: map[model.VariantKey]*model.Target{
			{Locale: "fr-FR"}: {Runs: []model.Run{{Text: &model.TextRun{Text: bulk}}}},
		},
		// Heavy skeleton carries the non-translatable structure.
		Skeleton: &model.Skeleton{
			Strategy: model.SkeletonFragmentBased,
			Parts:    []model.SkeletonPart{&model.SkeletonText{Text: half}},
		},
		// Heavy non-translatable properties.
		Properties: map[string]string{"bulk": half},
	}
	return b
}

// collectUploadedChunks runs Push against a test server that records every
// uploaded chunk body and the proto SyncChunk it decodes to. needed lists the
// block IDs the server asks for (all, here).
func collectUploadedChunks(t *testing.T, c *BowrainClient, blocksByItem map[string][]*model.Block, items []ItemMeta) (bodies [][]byte, chunks []*pb.SyncChunk) {
	t.Helper()

	// Server asks for every block of every item.
	needed := map[string][]string{}
	newItems := make([]string, 0, len(blocksByItem))
	for item, blocks := range blocksByItem {
		newItems = append(newItems, item)
		for _, b := range blocks {
			needed[item] = append(needed[item], b.ID)
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/push/init"):
			_ = json.NewEncoder(w).Encode(PushInitResponse{
				UploadID: "up1", Status: "diff_computed", NewItems: newItems,
			})
		case strings.HasSuffix(r.URL.Path, "/push/diff"):
			var req PushDiffRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(PushDiffResponse{
				Needed: needed[req.ItemName], Transport: "proxy",
			})
		case strings.Contains(r.URL.Path, "/push/chunks/"):
			body, _ := io.ReadAll(r.Body)
			bodies = append(bodies, body)
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/push/commit"):
			_ = json.NewEncoder(w).Encode(SyncPushResponse{PushID: "p1", Stored: 1})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c.baseURL = srv.URL
	_, err := c.Push(context.Background(), blocksByItem, items)
	require.NoError(t, err)

	for _, body := range bodies {
		raw := body
		if c.compressor != nil {
			d, err := c.compressor.Decompress(body)
			require.NoError(t, err)
			raw = d
		}
		var chunk pb.SyncChunk
		require.NoError(t, proto.Unmarshal(raw, &chunk))
		chunks = append(chunks, &chunk)
	}
	return bodies, chunks
}

// TestPushChunkSizeBoundedByMarshaledSize verifies #27: chunks are sized from
// the full marshaled proto (targets/skeleton/annotations/runs), not SourceText,
// so a stream of heavy blocks is split into chunks that each stay under both the
// marshaled-size threshold and the 2 MiB proxy body cap.
func TestPushChunkSizeBoundedByMarshaledSize(t *testing.T) {
	const blockPayload = 256 * 1024 // 256 KiB heavy target+skeleton per block
	const blockCount = 24           // ~6 MiB marshaled total → forces multiple chunks

	blocks := make([]*model.Block, blockCount)
	for i := range blocks {
		blocks[i] = blockWithHeavyPayload(fmt.Sprintf("b%02d", i), blockPayload)
	}
	blocksByItem := map[string][]*model.Block{"locales/en.json": blocks}
	items := []ItemMeta{{Name: "locales/en.json", Format: "json"}}

	c := NewClaimTokenClient("placeholder", "proj1", "tok")
	bodies, chunks := collectUploadedChunks(t, c, blocksByItem, items)

	require.NotEmpty(t, chunks)
	// Must have split into more than one chunk (single 6 MiB chunk would be
	// truncated by the proxy).
	assert.Greater(t, len(chunks), 1, "heavy blocks must split across chunks")

	totalBlocks := 0
	for i, chunk := range chunks {
		totalBlocks += len(chunk.Blocks)

		// Each uploaded body must fit under the proxy's LimitReader cap.
		assert.LessOrEqualf(t, len(bodies[i]), proxyBodyCap,
			"chunk %d uploaded body %d exceeds proxy 2 MiB cap", i, len(bodies[i]))

		// The marshaled proto must stay under our boundary plus the largest
		// single block (a chunk is sealed before exceeding the threshold; one
		// block may still tip it over the line). This proves the boundary is
		// computed from the full marshaled size, not SourceText.
		marshaled := proto.Size(chunk)
		assert.LessOrEqualf(t, marshaled, maxChunkMarshaledBytes+blockPayload*2,
			"chunk %d marshaled size %d not bounded by marshaled-size threshold", i, marshaled)
	}
	assert.Equal(t, blockCount, totalBlocks, "every needed block must be uploaded exactly once")
}

// TestPushChunkSizeBoundedWithCompression repeats the size guard with zstd
// compression on (the production proxy path), asserting the compressed body
// stays under the 2 MiB cap.
func TestPushChunkSizeBoundedWithCompression(t *testing.T) {
	const blockPayload = 300 * 1024
	const blockCount = 20

	blocks := make([]*model.Block, blockCount)
	for i := range blocks {
		blocks[i] = blockWithHeavyPayload(fmt.Sprintf("b%02d", i), blockPayload)
	}
	blocksByItem := map[string][]*model.Block{"locales/en.json": blocks}
	items := []ItemMeta{{Name: "locales/en.json", Format: "json"}}

	c := NewClaimTokenClient("placeholder", "proj1", "tok")
	c.EnableCompression(nil)
	bodies, chunks := collectUploadedChunks(t, c, blocksByItem, items)

	require.NotEmpty(t, chunks)
	for i := range bodies {
		assert.LessOrEqualf(t, len(bodies[i]), proxyBodyCap,
			"compressed chunk %d body %d exceeds proxy 2 MiB cap", i, len(bodies[i]))
	}
}

// TestPushChunkRecordCountCap verifies the record-count cap still bounds chunks
// of tiny blocks (where marshaled size never reaches the byte threshold).
func TestPushChunkRecordCountCap(t *testing.T) {
	const blockCount = maxChunkRecords + 50

	blocks := make([]*model.Block, blockCount)
	for i := range blocks {
		blocks[i] = &model.Block{
			ID:           fmt.Sprintf("b%04d", i),
			Translatable: true,
			Source:       []model.Run{{Text: &model.TextRun{Text: "x"}}},
		}
	}
	blocksByItem := map[string][]*model.Block{"locales/en.json": blocks}
	items := []ItemMeta{{Name: "locales/en.json", Format: "json"}}

	c := NewClaimTokenClient("placeholder", "proj1", "tok")
	_, chunks := collectUploadedChunks(t, c, blocksByItem, items)

	require.Greater(t, len(chunks), 1, "must split once record-count cap is exceeded")
	total := 0
	for _, chunk := range chunks {
		assert.LessOrEqual(t, len(chunk.Blocks), maxChunkRecords,
			"chunk must not exceed the record-count cap")
		total += len(chunk.Blocks)
	}
	assert.Equal(t, blockCount, total)
}
