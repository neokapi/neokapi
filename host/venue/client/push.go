package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"

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

	// Transport is how this venue takes chunks: "direct" when it can grant
	// presigned PUTs to object storage, "proxy" when the bytes have to come
	// through the API. Stated at init because it decides how large a chunk may
	// be — the 2 MiB cap that shaped chunking is the API's request limit, and
	// nothing imposes it on a write that never reaches the API.
	Transport string `json:"transport,omitempty"`

	// Ref is the server's freshness ref for the stream, as of this
	// negotiation. It is what the commit that follows asserts, so a push does
	// not pay a second round trip to learn what it last saw. Absent from a
	// server too old to publish one.
	Ref *ref.Ref `json:"ref,omitempty"`
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

	// Scope is the set of paths this push is authoritative over: the recipe's
	// globs, or the resolved paths of a scoped push. Within it the declared
	// tree is exactly what the project holds; outside it nothing is touched.
	// A push declaring no scope removes no item — see core/venue.Scope.
	Scope venue.Scope `json:"scope,omitempty"`

	// Tree declares, per item this producer read, the block keys it holds and
	// the content hash of each — so the venue can remove what the source no
	// longer has, and can recognise a file that moved by what is inside it. A
	// push carries only what changed, so what it carries cannot say what is
	// gone; this is what says it. See core/venue.Tree.
	Tree venue.Tree `json:"tree,omitempty"`

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
	scope          venue.Scope
	tree           venue.Tree
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

// DeclareTree tells the venue what this producer read: the scope it is
// authoritative over, and every item within it, with the block keys and content
// hashes each holds now.
//
// The two together are what let a push say what is GONE. A payload of changed
// blocks cannot: a removed string simply stops being sent, and a venue that
// upserts what arrives keeps it forever. The tree says what each item holds,
// and the scope says which items this push speaks for — so absence inside the
// scope is an answer, and absence outside it is silence.
//
// The scope is also what makes a scoped push safe by construction rather than
// by nobody looking: `kapi push <subdir>` declares that subdir, and every file
// outside it is out of scope by the same rule that governs the rest.
//
// A push that declares no scope removes no item — an older producer makes no
// claim about what is missing, so its push stays purely additive.
func DeclareTree(scope venue.Scope, tree venue.Tree) PushOption {
	return func(s *pushSettings) {
		s.scope = scope
		s.tree = tree
	}
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

	// maxProxyChunkMarshaledBytes bounds the *marshaled* (uncompressed) size of
	// a single chunk's blocks when the chunk travels through the API. The proxy
	// upload path reads the request body with io.LimitReader(body, 2 MiB) and
	// silently truncates anything larger, which would make the client-computed
	// chunk hash diverge from the stored bytes and fail the commit's
	// BlobStore.Exists check.
	//
	// The 2 MiB cap applies to the *compressed* chunk. zstd never expands
	// real-world content, so a marshaled chunk kept under this threshold stays
	// safely under the compressed cap even in the pathological no-compression
	// case (incompressible payloads), with comfortable headroom for the proto
	// chunk envelope. Sizing the boundary from proto.Size of each block (which
	// accounts for source text, targets, annotations, skeleton, runs — the
	// whole serialized block) — rather than from SourceText alone — is what
	// prevents oversized chunks from slipping through.
	maxProxyChunkMarshaledBytes = 1536 * 1024 // 1.5 MiB

	// maxDirectChunkMarshaledBytes is the same bound when the chunk goes
	// straight to object storage. No request handler is in the path, so the API
	// imposes nothing; what remains is the producer's own memory, since a
	// window of chunks is held while it uploads in parallel.
	//
	// Sized from object-storage guidance, which puts useful part sizes at
	// 16–64 MB: below that band the per-request overhead starts to dominate,
	// above it a failure costs more to retry than it saves. This is the bottom
	// of the band on purpose — the bound is on the MARSHALED bytes, which are
	// then compressed, so the object that lands is smaller again, and the
	// window holds several of them at once.
	maxDirectChunkMarshaledBytes = 16 * 1024 * 1024 // 16 MiB
)

// Push performs a complete push: init → upload chunks → commit.
// blocksByItem maps item_name → blocks for that item; items is the item
// metadata; pushCtx is the context content type — the collections the recipe
// declares, which travel in the commit manifest so the structure the items are
// stored into lands in the same transaction as the items themselves. A nil
// pushCtx pushes content without touching the declared context, which is what
// a caller that has no recipe to read (or a --no-brand-style opt-out) wants.
//
// What this push says about the project's content.
//
// blocksByItem carries only the blocks the producer found changed when it
// diffed its scan against the venue's tree: an unchanged item is absent from
// the map, and a changed item carries only its changed blocks. The ItemHashes
// and RootHash sent at init are computed over that subset. They are change
// indicators for init's fast path, never Merkle roots over the whole project,
// and the client reads none of the per-item verdicts init returns
// (initResp.DeletedItems included).
//
// What is gone travels separately, as the declared scope and tree
// (DeclareTree). The scope is the set of paths this push is authoritative
// over; the tree lists every item read within it with the block keys each
// holds. Inside the scope the server deletes an item the tree omits and prunes
// the blocks an item no longer lists; outside it nothing is touched, and a
// push that declares no scope removes nothing. A collection the recipe no
// longer names is reported (initResp.UndeclaredCollections), never deleted.
func (c *BowrainClient) Push(ctx context.Context, blocksByItem map[string][]*model.Block, items []ItemMeta, pushCtx *PushContext, decisions []venue.UnitDecision, opts ...PushOption) (*SyncPushResponse, error) {
	var settings pushSettings
	for _, opt := range opts {
		opt(&settings)
	}
	// Guard a nil client: callers build the client from the recipe's server:
	// block, which is absent until the project is connected. Return a clear
	// error instead of a nil-pointer panic in projectPrefix/streamPrefix.
	if c == nil {
		return nil, errors.New("bowrain: project is not connected to a server. Run 'kapi init --server <url>' to connect")
	}
	// 1. Hashes over the blocks being sent, for init's fast path only. They are
	//    change indicators, not authoritative roots — what this push claims
	//    about the project's whole content is the declared tree, and what it is
	//    authoritative over is the declared scope.
	itemHashes := make(map[string]string)
	for itemName, blocks := range blocksByItem {
		blockHashes := make(map[string]string, len(blocks))
		for _, b := range blocks {
			// Keyed on the DURABLE identity, matching what the venue stores as
			// source_id. Keyed on the reader's id instead, deleting a paragraph
			// renumbered every block below it and the push reported an untouched
			// file as wholly changed.
			blockHashes[convergence.BlockKey(b)] = model.ComputeIdentity(b).RecordHash()
		}
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

	// initResp's item verdicts are not read at all any more. They were the
	// venue's answer to a question the producer now answers for itself, and the
	// deletion half of them was never actionable: without a declared scope, an
	// item absent from a push means "not looked at" as readily as "removed".
	// The declared tree and scope carry both halves honestly.
	//
	// The context reconcile is skipped when the server already holds this
	// context — the fast path's whole point. The entries are dropped from the
	// manifest rather than sent and ignored, so an unedited recipe costs
	// nothing beyond the hash it negotiated on.
	contexts := pushCtx.entriesIfChanged(initResp.ContextChanged)

	// 3. The blocks to send were decided before this call: the producer fetched
	// the venue's tree and diffed its scan against it locally, which is what
	// replaced a negotiation that descended one item at a time. What arrives
	// here IS the pack.
	//
	// How it travels is the venue's to say. A deployment whose blob store is a
	// directory has no presigning, and its pushes proxy exactly as before.
	transport := initResp.Transport
	if transport == "" {
		transport = TransportProxy
	}

	// 4. Stage and write the pack.
	var chunks []ChunkRef
	chunkIndex := 0

	// Sorted, so a push's chunks are the same bytes on every run and a retry
	// re-uploads to the same content-addressed keys.
	sendItems := make([]string, 0, len(blocksByItem))
	for itemName := range blocksByItem {
		sendItems = append(sendItems, itemName)
	}
	sort.Strings(sendItems)

	// Chunks are staged into a window and written a window at a time: the
	// window bounds how much of the pack is in memory at once, and is the unit
	// one grant of presigned URLs covers and one round of parallel PUTs writes.
	chunkLimit := maxProxyChunkMarshaledBytes
	if transport == TransportDirect {
		chunkLimit = maxDirectChunkMarshaledBytes
	}
	var window []*stagedChunk

	flushWindow := func() error {
		refs, err := c.uploadWindowOfChunks(ctx, initResp.UploadID, window)
		if err != nil {
			return err
		}
		chunks = append(chunks, refs...)
		window = nil
		return nil
	}

	seal := func(blocks []*pb.SyncBlock) error {
		staged, err := c.stageChunk(chunkIndex, "blocks", blocks)
		if err != nil {
			return fmt.Errorf("stage chunk %d: %w", chunkIndex, err)
		}
		chunkIndex++
		window = append(window, staged)
		if len(window) >= uploadWindow {
			return flushWindow()
		}
		return nil
	}

	for _, itemName := range sendItems {
		blocks := blocksByItem[itemName]
		var chunkBlocks []*pb.SyncBlock
		chunkBytes := 0

		for _, b := range blocks {
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
			// Seal the current chunk *before* appending a block that would push
			// the marshaled size over the threshold. A single block larger than
			// the threshold still rides in its own chunk.
			sbSize := proto.Size(sb)
			if len(chunkBlocks) > 0 && chunkBytes+sbSize > chunkLimit {
				if err := seal(chunkBlocks); err != nil {
					return nil, err
				}
				chunkBlocks = nil
				chunkBytes = 0
			}

			chunkBlocks = append(chunkBlocks, sb)
			chunkBytes += sbSize

			// Also cap by record count to keep chunks bounded for tiny blocks.
			if len(chunkBlocks) >= maxChunkRecords {
				if err := seal(chunkBlocks); err != nil {
					return nil, err
				}
				chunkBlocks = nil
				chunkBytes = 0
			}
		}

		// Flush remaining blocks.
		if len(chunkBlocks) > 0 {
			if err := seal(chunkBlocks); err != nil {
				return nil, err
			}
		}
	}
	if err := flushWindow(); err != nil {
		return nil, err
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
		Scope:             settings.scope,
		Tree:              settings.tree,
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
