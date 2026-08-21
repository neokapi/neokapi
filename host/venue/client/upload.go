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
	"sync"

	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"google.golang.org/protobuf/proto"
)

// Content goes to object storage, not through the API.
//
// Every byte a push sent used to transit the API server, which then wrote it to
// the blob store the worker would read it back from. Object storage was already
// the destination — the API was a hop in the middle of it, and it cost a 2 MiB
// request cap that shaped the chunking, an interactive server's throughput and
// memory spent on bulk transfer, and uploads that could not run in parallel
// because they contended with the API's own concurrency.
//
// A push now asks for presigned PUTs, keyed by the content hash of the bytes it
// is about to send, and writes them straight to the store. The venue verifies
// what it received by hash at commit, so a wrong object is a mismatch that
// refuses the push rather than a corruption that lands.
//
// The proxy path stays for a deployment whose blob store is a directory. That
// is what the transport distinction was declared for; it just never had a
// second value that worked.

const (
	// uploadWindow is how many chunks are held and uploaded at once. It bounds
	// both the producer's memory (window × chunk size — the per-chunk caps are
	// in push.go, where the chunker reads them) and the number of URLs asked
	// for in one grant.
	uploadWindow = 8

	// uploadConcurrency is how many direct PUTs run at a time within a window.
	// The proxy path is not parallelised at all: its whole cost is the load it
	// puts on the interactive server.
	uploadConcurrency = 4
)

// TransportDirect and TransportProxy are the two ways a chunk reaches storage.
const (
	TransportDirect = "direct"
	TransportProxy  = "proxy"
)

// stagedChunk is one chunk, marshaled, compressed and named by its content,
// waiting for the grant that lets it be written.
type stagedChunk struct {
	Index       int
	ContentType string
	RecordCount int
	Hash        string
	Data        []byte
}

// ChunkRefOf is what the manifest records about a chunk once it has landed.
func (s stagedChunk) ref() ChunkRef {
	return ChunkRef{
		Index:       s.Index,
		ContentType: s.ContentType,
		Hash:        s.Hash,
		RecordCount: s.RecordCount,
		ByteSize:    int64(len(s.Data)),
	}
}

// stageChunk marshals and compresses one chunk and names it by its bytes.
//
// The name is a plain SHA-256 of the exact bytes uploaded, which is what the
// blob store keys on and what the worker resolves. It is deliberately NOT
// model.ComputeContentHash: that TrimSpace-normalizes its input, which corrupts
// the digest of compressed binary data.
func (c *BowrainClient) stageChunk(index int, contentType string, blocks []*pb.SyncBlock) (*stagedChunk, error) {
	chunk := &pb.SyncChunk{
		ContentType: contentType,
		RecordCount: int32(len(blocks)),
		Blocks:      blocks,
	}
	data, err := proto.Marshal(chunk)
	if err != nil {
		return nil, fmt.Errorf("marshal chunk: %w", err)
	}
	if c.compressor != nil {
		data, err = c.compressor.Compress(data)
		if err != nil {
			return nil, fmt.Errorf("compress chunk: %w", err)
		}
	}
	sum := sha256.Sum256(data)
	return &stagedChunk{
		Index:       index,
		ContentType: contentType,
		RecordCount: len(blocks),
		Hash:        hex.EncodeToString(sum[:]),
		Data:        data,
	}, nil
}

// uploadGrant is the venue's answer about a set of content hashes.
type uploadGrant struct {
	Transport string            `json:"transport"`
	URLs      map[string]string `json:"urls"`
	Have      []string          `json:"have"`
}

// requestUploadURLs asks for a presigned PUT per hash the venue does not
// already hold.
func (c *BowrainClient) requestUploadURLs(ctx context.Context, hashes []string) (*uploadGrant, error) {
	body, err := json.Marshal(map[string]any{"hashes": hashes})
	if err != nil {
		return nil, fmt.Errorf("marshal upload request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.streamPrefix()+"/push/uploads", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, NewStatusError("push uploads", resp.StatusCode, b)
	}
	var grant uploadGrant
	if err := json.NewDecoder(resp.Body).Decode(&grant); err != nil {
		return nil, fmt.Errorf("decode upload grant: %w", err)
	}
	return &grant, nil
}

// uploadWindowOfChunks writes one window of staged chunks and returns their
// manifest refs.
//
// A window is the unit of both memory and parallelism: its chunks are held
// while they upload and released after, and one grant covers all of them.
func (c *BowrainClient) uploadWindowOfChunks(ctx context.Context, uploadID string, window []*stagedChunk) ([]ChunkRef, error) {
	if len(window) == 0 {
		return nil, nil
	}

	hashes := make([]string, 0, len(window))
	for _, ch := range window {
		hashes = append(hashes, ch.Hash)
	}

	grant, err := c.requestUploadURLs(ctx, hashes)
	if err != nil {
		// A venue too old to grant URLs still takes the proxy. The push slows
		// down; it does not fail.
		grant = &uploadGrant{Transport: TransportProxy}
	}

	held := map[string]bool{}
	for _, h := range grant.Have {
		held[h] = true
	}

	refs := make([]ChunkRef, len(window))
	errs := make([]error, len(window))
	sem := make(chan struct{}, uploadConcurrency)
	var wg sync.WaitGroup

	for i, ch := range window {
		refs[i] = ch.ref()
		// Already in storage, byte for byte — the key IS the content. A push
		// retried after a failed commit re-sends nothing it already sent.
		if held[ch.Hash] {
			continue
		}

		url, direct := grant.URLs[ch.Hash]
		if !direct {
			// The proxy path stays sequential. Parallelising it would put the
			// concurrency onto the interactive server this exists to take work
			// off — the contention is the thing being removed, not the waiting.
			errs[i] = c.putProxied(ctx, uploadID, ch)
			continue
		}
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			errs[i] = c.putDirect(ctx, url, ch.Data)
		})
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("upload chunk %d: %w", window[i].Index, err)
		}
	}
	return refs, nil
}

// putDirect writes a chunk to object storage through its presigned URL.
//
// The request carries no credential of ours: the URL is the grant, and it is
// scoped to one key whose name is the content's own hash. c.Do is deliberately
// not used — it attaches this client's authorization, which the object store
// has no business seeing and which some backends reject alongside a presigned
// signature.
func (c *BowrainClient) putDirect(ctx context.Context, url string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("direct upload HTTP %d", resp.StatusCode)
	}
	return nil
}

// putProxied writes a chunk through the API, for a deployment whose blob store
// is a directory.
func (c *BowrainClient) putProxied(ctx context.Context, uploadID string, ch *stagedChunk) error {
	u := c.streamPrefix() + fmt.Sprintf("/push/chunks/%s/%d", uploadID, ch.Index)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(ch.Data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chunk upload HTTP %d", resp.StatusCode)
	}
	return nil
}
