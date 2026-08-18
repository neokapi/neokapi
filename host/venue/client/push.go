package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/protobuf/proto"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
)

// This file also carries the context content type's client half: the recipe's
// declared collections travel with the push (PushContext) instead of being
// uploaded by a separate call afterwards.

// PushInitRequest is the request for the Merkle tree diff negotiation.
type PushInitRequest struct {
	ProjectID  string            `json:"project_id"`
	Stream     string            `json:"stream"`
	ItemHashes map[string]string `json:"item_hashes"` // item_name → hash
	RootHash   string            `json:"root_hash"`

	// ContextHash is the fast path for the context content type: a hash over
	// every declared collection's context entry. When it matches the server's,
	// the recipe's structure and governance are already in force and the
	// manifest carries no contexts.
	ContextHash string `json:"context_hash,omitempty"`

	// Collections names the collections the recipe declares, so the server can
	// answer which of the ones it holds are no longer declared.
	Collections []string `json:"collections,omitempty"`

	// ContentModelEpoch states the generation of content model this producer
	// reads into. A stream that has received a higher one refuses this push
	// rather than letting it flatten what a richer kapi wrote — see
	// core/venue.ContentModelEpoch. Stated at init so the refusal costs no
	// upload.
	ContentModelEpoch int `json:"content_model_epoch,omitempty"`

	// AllowModelDowngrade carries `kapi push --force` past that refusal: the
	// deliberate downgrade, for when flattening is what you meant.
	AllowModelDowngrade bool `json:"allow_model_downgrade,omitempty"`
}

// PushInitResponse is the response from the init endpoint.
type PushInitResponse struct {
	UploadID           string   `json:"upload_id"`
	Status             string   `json:"status"` // "unchanged", "diff_computed"
	ChangedItems       []string `json:"changed_items"`
	NewItems           []string `json:"new_items"`
	DeletedItems       []string `json:"deleted_items"`
	UnchangedItemCount int      `json:"unchanged_item_count"`

	// ContextChanged reports whether the declared context differs from what the
	// server holds.
	ContextChanged bool `json:"context_changed"`

	// UndeclaredCollections lists recipe-owned collections the server holds
	// that this push no longer declares. Reported, never acted on — the same
	// contract as DeletedItems, and for a stronger reason: a collection is
	// where content lives, so dropping one on a recipe edit would take the
	// server-side content grouped under it with it.
	UndeclaredCollections []string `json:"undeclared_collections,omitempty"`

	// Ref is the server's freshness ref for the stream, as of this
	// negotiation. It is what the commit that follows asserts, so a push does
	// not pay a second round trip to learn what it last saw. Absent from a
	// server too old to publish one.
	Ref *ref.Ref `json:"ref,omitempty"`
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

	// Contexts carries the context content type: the collections the recipe
	// declares, their coordinates and their resolved governance. It rides in
	// the manifest rather than in a chunk because it is the shape the uploaded
	// items are stored into — one push is one consistent state.
	Contexts []*pb.SyncContextEntry `json:"contexts,omitempty"`

	// Decisions carries the project's committed decision record (core/state):
	// who approved which unit, when, and the hash of the translation each
	// decision blesses. A sidecar like Contexts rather than a chunk — the
	// ledger is small next to content, and it belongs in the same transaction
	// as the content it judges. The server upserts idempotently, so sending
	// the full set every push is correct first; a hash fast-path is an
	// optimization this field's shape does not preclude.
	Decisions []venue.UnitDecision `json:"decisions,omitempty"`

	// ExpectedRef is the compare-and-swap assertion: the governance components
	// this push last observed on the server. The server asserts only the ones
	// this manifest writes, and rejects with a conflict when one has moved.
	ExpectedRef ref.Ref `json:"expected_ref,omitzero"`

	// BlockPropertyKeys declares the property keys this producer's readers
	// emit, so the server knows what this push is authoritative about and
	// leaves the rest of a stored block's properties alone. See
	// core/venue.BlockPropertyKeys — it scopes deletion, never transfer.
	BlockPropertyKeys []string `json:"block_property_keys,omitempty"`

	// ContentModelEpoch is the generation this push wrote, recorded on the
	// stream once the manifest commits.
	ContentModelEpoch int `json:"content_model_epoch,omitempty"`
}

// PushUnchanged is the push id a push reports when the negotiation found
// nothing to send: no chunks were uploaded, no manifest was committed, and no
// worker job exists to confirm. A caller reads it as "the server already holds
// this state", never as "the server accepted a write".
const PushUnchanged = "unchanged"

// PushOption adjusts one push.
type PushOption func(*pushSettings)

type pushSettings struct {
	expected       ref.Ref
	propertyKeys   []string
	allowDowngrade bool
}

// AssertRef makes the push a compare-and-swap over the governance it carries:
// the server refuses the commit when a component this push writes has moved
// since observed.
//
// The value to pass is the ref the caller LAST OBSERVED — the cached one, not
// one fetched at the start of this push. The payload was built by diffing
// against the cached ref, so that is the state the write is correct against; a
// freshly fetched value would happily accept a write computed from ground that
// had already shifted.
func AssertRef(observed ref.Ref) PushOption {
	return func(s *pushSettings) { s.expected = observed }
}

// DeclareBlockProperties tells the server which block property keys this
// producer's readers emit, so a push is authoritative about those and about
// nothing else. Computed over every block the producer read — see
// core/venue.BlockPropertyKeys.
//
// A push that declares nothing is read as knowing nothing, so it adds and
// updates but never removes: an older kapi cannot delete a field it has never
// heard of.
func DeclareBlockProperties(keys []string) PushOption {
	return func(s *pushSettings) { s.propertyKeys = keys }
}

// AllowModelDowngrade lets this push write content from an older model
// generation than the stream has already received — what `kapi push --force`
// means when the two disagree. Without it the server refuses, because the
// alternative is silently flattening work a richer kapi did.
func AllowModelDowngrade() PushOption {
	return func(s *pushSettings) { s.allowDowngrade = true }
}

// PushContext is everything the context content type contributes to one push:
// the entries themselves and the hash negotiated at init. Callers build it with
// NewPushContext so the two cannot drift apart.
type PushContext struct {
	Entries []*pb.SyncContextEntry
	Hash    string
}

// NewPushContext stamps each entry's content hash, folds them into the push's
// context hash, and returns both. A push with no declared collections yields
// the empty-fold hash, which is exactly what a server holding no collections
// computes — so an uncoordinated project negotiates "unchanged" rather than
// re-reconciling nothing on every push.
func NewPushContext(entries []*pb.SyncContextEntry) *PushContext {
	return &PushContext{Entries: entries, Hash: venue.ContextHashOf(entries)}
}

// names returns the declared collection names, for the init negotiation.
func (p *PushContext) names() []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.Entries))
	for _, e := range p.Entries {
		if e != nil && e.Name != "" {
			out = append(out, e.Name)
		}
	}
	return out
}

// ChunkRef identifies a single uploaded chunk in the push commit manifest.
// Hash is the SHA-256 of the exact bytes uploaded (after optional zstd compression)
// and must match how the blob store keys the chunk so the worker can retrieve it.
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
// metadata; pushCtx is the context content type — the collections the recipe
// declares, which travel in the commit manifest so the structure the items are
// stored into lands in the same transaction as the items themselves. A nil
// pushCtx pushes content without touching the declared context, which is what
// a caller that has no recipe to read (or a --no-brand-style opt-out) wants.
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
func (c *BowrainClient) Push(ctx context.Context, blocksByItem map[string][]*model.Block, items []ItemMeta, pushCtx *PushContext, decisions []venue.UnitDecision, opts ...PushOption) (*SyncPushResponse, error) {
	var settings pushSettings
	for _, opt := range opts {
		opt(&settings)
	}
	// Guard a nil client: callers build the client from the recipe's server:
	// block, which is absent until the project is connected. Return a clear
	// error instead of a nil-pointer panic in projectPrefix/streamPrefix.
	if c == nil {
		return nil, errors.New("bowrain: project is not connected to a server — run 'kapi init --server <url>' to connect")
	}
	// 1. Compute Merkle hashes over the changed subset only (additive-only
	//    contract above): these are change indicators, not authoritative roots.
	itemHashes := make(map[string]string)
	blockHashesByItem := make(map[string]map[string]string)
	for itemName, blocks := range blocksByItem {
		blockHashes := make(map[string]string, len(blocks))
		for _, b := range blocks {
			identity := model.ComputeIdentity(b)
			// Keyed on the DURABLE identity, matching what the server stores as
			// source_id. Keyed on the reader's id instead, deleting a paragraph
			// renumbered every block below it and the push reported an untouched
			// file as wholly changed.
			blockHashes[convergence.BlockKey(b)] = identity.RecordHash()
		}
		blockHashesByItem[itemName] = blockHashes
		itemHashes[itemName] = venue.ComputeItemHash(blockHashes)
	}
	rootHash := venue.ComputeRootHash(itemHashes)

	// 2. Init — send item hashes and the declared context.
	initResp, err := c.pushInit(ctx, PushInitRequest{
		ItemHashes:          itemHashes,
		RootHash:            rootHash,
		ContextHash:         contextHashOf(pushCtx),
		Collections:         pushCtx.names(),
		ContentModelEpoch:   venue.ContentModelEpoch,
		AllowModelDowngrade: settings.allowDowngrade,
	})
	if err != nil {
		return nil, fmt.Errorf("push init: %w", err)
	}
	if initResp.Status == "unchanged" {
		// Content and context are already in force — but a caller carrying
		// decisions still has a record to land, so the push proceeds to an
		// empty-chunk commit rather than returning here. The caller only
		// passes decisions when they changed, so the common unchanged push
		// still takes this exit.
		if len(decisions) == 0 {
			return &SyncPushResponse{
				PushID:                PushUnchanged,
				UndeclaredCollections: initResp.UndeclaredCollections,
				ServerRef:             initResp.Ref,
			}, nil
		}
		commitResp, err := c.pushCommit(ctx, PushCommitRequest{
			Stream:      c.stream,
			Decisions:   decisions,
			ExpectedRef: settings.expected,
		})
		if err != nil {
			return nil, fmt.Errorf("push commit (decisions): %w", err)
		}
		if commitResp != nil {
			commitResp.UndeclaredCollections = initResp.UndeclaredCollections
			commitResp.ServerRef = initResp.Ref
		}
		return commitResp, nil
	}

	// The context reconcile is skipped when the server already holds this
	// context — the fast path's whole point. The entries are dropped from the
	// manifest rather than sent and ignored, so an unedited recipe costs
	// nothing beyond the hash it negotiated on.
	contexts := pushCtx.entriesIfChanged(initResp.ContextChanged)

	// 3. For each changed/new item, send block-level diff and collect needed blocks.
	//
	// We act ONLY on ChangedItems + NewItems. initResp.DeletedItems is
	// deliberately ignored: per the additive-only contract, ItemHashes covers
	// only the changed subset, so an item the server flags "deleted" merely
	// reflects items absent from this push, not items the user removed. Acting
	// on it would be data loss.
	allNeeded := map[string]map[string]struct{}{} // item → set of needed block IDs
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
		needed := map[string]struct{}{}
		for _, id := range diffResp.Needed {
			needed[id] = struct{}{}
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
			// The needed set answers the diff, and the diff was keyed on the
			// durable identity — so the lookup must be too. Keyed on b.ID here,
			// no needed key ever matched a reader id, and a push that had
			// negotiated 75k missing blocks uploaded zero chunks and reported
			// success.
			if _, ok := neededIDs[convergence.BlockKey(b)]; !ok {
				continue
			}
			sb := venue.BlockToProto(b, itemName)
			// Skeleton is format scaffolding that belongs at the connector edge,
			// not durably in the content store — the standing invariant keeps it
			// off the default push wire. The proto field stays reserved for the
			// future opt-in (backlog 014); until that exists, the default push
			// drops it so skeleton bytes never land in the staging chunk blobs.
			// The converter still round-trips it losslessly for the parity guard.
			sb.SkeletonJson = nil
			// Estimate the boundary from the block's full marshaled proto size
			// (source + targets + annotations + runs), not from SourceText alone.
			// Seal the current chunk *before* appending a block that would push the
			// marshaled size over the safe threshold, so the chunk never exceeds
			// the proxy upload's 2 MiB cap (#27). A single block larger than the
			// threshold still rides in its own chunk.
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
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("marshal items: %w", err)
	}
	commitResp, err := c.pushCommit(ctx, PushCommitRequest{
		UploadID: initResp.UploadID,
		// The server takes the stream from the authorized route path; this
		// field only matters to a deployment whose commit route carries no ref.
		Stream:            c.stream,
		Chunks:            chunks,
		Items:             itemsJSON,
		Contexts:          contexts,
		Decisions:         decisions,
		ExpectedRef:       settings.expected,
		BlockPropertyKeys: settings.propertyKeys,
		ContentModelEpoch: venue.ContentModelEpoch,
	})
	if err != nil {
		return nil, fmt.Errorf("push commit: %w", err)
	}
	if commitResp != nil {
		commitResp.UndeclaredCollections = initResp.UndeclaredCollections
		commitResp.ServerRef = initResp.Ref
		commitResp.ChunkCount = len(chunks)
		for _, ch := range chunks {
			commitResp.BlocksUploaded += ch.RecordCount
		}
	}

	return commitResp, nil
}

// contextHashOf returns the push's negotiated context hash, or "" when the
// caller pushes no context at all. The empty string is distinct from the
// empty-fold hash: it means "this push makes no claim about the declared
// context", which is what stops a content-only caller from looking like a
// recipe that just deleted every collection.
func contextHashOf(p *PushContext) string {
	if p == nil {
		return ""
	}
	return p.Hash
}

// entriesIfChanged returns the entries to put in the manifest: none when the
// server reported the context unchanged, and none when the caller pushes no
// context.
func (p *PushContext) entriesIfChanged(changed bool) []*pb.SyncContextEntry {
	if p == nil || !changed {
		return nil
	}
	return p.Entries
}

func (c *BowrainClient) pushInit(ctx context.Context, req PushInitRequest) (*PushInitResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal push init request: %w", err)
	}
	u := c.streamPrefix() + "/push/init"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
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
		return nil, NewStatusError("push init", resp.StatusCode, b)
	}
	var result PushInitResponse
	return &result, json.NewDecoder(resp.Body).Decode(&result)
}

func (c *BowrainClient) pushDiff(ctx context.Context, req PushDiffRequest) (*PushDiffResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal push diff request: %w", err)
	}
	u := c.streamPrefix() + "/push/diff"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
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
		return nil, NewStatusError("push diff", resp.StatusCode, b)
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
	resp, err := c.Do(httpReq)
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
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal push commit request: %w", err)
	}
	u := c.streamPrefix() + "/push/commit"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, NewStatusError("push commit", resp.StatusCode, b)
	}
	var result SyncPushResponse
	return &result, json.NewDecoder(resp.Body).Decode(&result)
}
