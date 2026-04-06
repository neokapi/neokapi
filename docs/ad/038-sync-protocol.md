---
id: 038-sync-protocol
sidebar_position: 38
title: "AD-038: Sync Protocol — Chunked, Resumable, Direct-to-Storage"
---

# AD-038: Sync Protocol — Chunked, Resumable, Direct-to-Storage

## Context

The sync protocol is the primary data exchange path for Bowrain — it carries content, translations, terminology, and automation results between clients and the server. The current implementation has fundamental limitations:

- **Full payload in memory** on server, worker, and client
- **Single JSON blob** — no streaming, no chunking, no compression
- **Not resumable** — failure means restart from zero
- **No diff sync** — pushes the full dataset every time, even for 1-block changes
- **Thin contract** — only block text survives the sync boundary; annotations, properties, display hints, connector data are lost
- **Block-only** — no protocol for syncing terminology, TM entries, QA results, or automation outputs
- **Heuristic format detection** — server guesses format from file extension
- **No conflict detection** — concurrent pushes silently overwrite each other

The sync protocol must be the **single, extensible transport** for all project data — not just blocks. It must be efficient enough for 100K+ blocks and resilient enough for unreliable networks.

## Decision

### Core design: Three layers

```
1. Diff layer:   what needs to transfer (Merkle tree hash comparison)
2. Content layer: typed, versioned payloads that evolve independently
3. Transport layer: chunked, compressed, resumable, direct-to-storage
```

### Diff layer: Merkle tree hash negotiation

The protocol transfers only what changed since the last sync. A flat hash manifest (record_id to content_hash) scales to ~10K records but breaks at 100K+. Instead, we use a **two-level Merkle tree**:

```
Project root hash
  ├── Item "en.json"       hash: abc123  (hash of all block hashes in this item)
  │     ├── Block b1       hash: def456
  │     ├── Block b2       hash: ghi789
  │     └── ...
  ├── Item "messages.json"  hash: jkl012
  │     └── ...
  └── Terms collection      hash: mno345
        └── ...
```

**Push negotiation:**

```
1. Client computes item-level hashes (hash of sorted block hashes per item)
2. Client → API: POST /sync/push/init
   Request: { project_id, stream, item_hashes: {"en.json": "abc123", ...} }

3. Server compares item hashes:
   - Matching item hash → skip entirely (all blocks unchanged)
   - Mismatched hash → respond with "need block-level diff for this item"
   - New item → respond with "upload all blocks"
   - Missing item → respond with "deleted on client"

4. For mismatched items, client sends block-level hashes
   Client → API: POST /sync/push/diff
   Request: { upload_id, item: "en.json", block_hashes: {"b1": "def456", ...} }

5. Server responds with needed block IDs for that item
```

**Scaling:**

| Project size | Items     | Init request       | Diff requests   |
| ------------ | --------- | ------------------ | --------------- |
| 1K blocks    | 5 items   | 5 hashes (~500B)   | 0-2 item diffs  |
| 10K blocks   | 20 items  | 20 hashes (~2KB)   | 0-5 item diffs  |
| 100K blocks  | 100 items | 100 hashes (~10KB) | 0-10 item diffs |

The init request is always small (item count, not block count). Block-level comparison only happens for changed items.

**Fast path**: Client also sends a project root hash (hash of all item hashes). If it matches the server's root hash, nothing changed — zero diff computation.

**Conflict detection**: Each hash comparison includes an `expected_hash` for records being updated. If the server's current hash doesn't match the client's expectation (another client changed it), the server flags a conflict instead of silently overwriting.

```protobuf
message SyncPushInit {
  string project_id = 1;
  string stream = 2;
  repeated string content_types = 3;

  // Merkle tree: item-level hashes
  map<string, string> item_hashes = 4;  // item_name → hash of block hashes

  // Fast path: root hash of all item hashes
  string root_hash = 5;

  // Terms/TM have their own collection hashes
  string terms_hash = 6;
  string tm_hash = 7;
}

message SyncPushInitResponse {
  string upload_id = 1;
  string status = 2;  // "unchanged", "diff_required", "full_upload"

  // Items that need block-level diff
  repeated string changed_items = 3;

  // Items on server that client didn't include (deletions)
  repeated string deleted_items = 4;

  // New items the client has that server doesn't
  repeated string new_items = 5;

  // Stats
  int32 unchanged_item_count = 6;
}

message SyncItemDiff {
  string upload_id = 1;
  string item_name = 2;
  map<string, string> block_hashes = 3;  // block_id → content_hash
}

message SyncItemDiffResponse {
  repeated string needed = 1;     // blocks to upload (new or changed)
  repeated string deleted = 2;    // blocks server has that client doesn't
  repeated string conflicts = 3;  // blocks changed by another client
  repeated string chunk_urls = 4; // SAS URLs for uploading needed blocks
}
```

### Content layer: SyncChunk envelope

Each chunk is a typed envelope that can carry any content type. Content versioning follows protobuf's forward/backward compatibility — new fields are additive, old clients ignore unknown fields:

```protobuf
// SyncChunk is the unit of transfer. Each chunk contains a batch of
// typed records. A single push can mix content types across chunks.
message SyncChunk {
  string content_type = 1;    // "blocks", "terms", "tm", "media", "qa", "activity"
  int32 record_count = 2;

  // Exactly one of these is populated (determined by content_type):
  repeated SyncBlock blocks = 10;
  repeated SyncTerm terms = 11;
  repeated SyncTMEntry tm_entries = 12;
  repeated SyncQAResult qa_results = 13;
  repeated SyncActivity activities = 14;
  repeated SyncMedia media = 15;
  // Future types added here without changing the envelope or transport.
}
```

Adding a new content type requires adding a field to SyncChunk and a handler in the worker — zero transport or diff changes. Changing a content type (e.g., adding a field to SyncBlock) is a protobuf-compatible evolution.

### SyncBlock — full block model

Carries the complete Block through the sync boundary:

```protobuf
message SyncBlock {
  string id = 1;
  string item_name = 2;
  string name = 3;
  string type = 4;
  string mime_type = 5;
  bool translatable = 6;

  // Source content (structured segments with inline spans)
  repeated Segment source = 7;
  string source_text = 8;       // plain text convenience

  // Translations per locale (structured)
  map<string, SegmentList> targets = 9;

  // Metadata
  map<string, string> properties = 10;
  bytes annotations_json = 11;  // serialized annotation map

  // Structure
  bytes skeleton_json = 12;
  bool preserve_whitespace = 13;
  bytes display_hint_json = 14;
  bytes content_ref_json = 15;

  // Extensible
  map<string, string> connector_data = 16;

  // Conflict detection: the hash the client last saw for this block.
  // Server rejects if current hash differs (another client changed it).
  string expected_hash = 17;
}
```

### SyncTerm — terminology

```protobuf
message SyncTerm {
  string concept_id = 1;
  string source_term = 2;
  string source_locale = 3;
  repeated TermTranslation translations = 4;
  string definition = 5;
  string domain = 6;
  map<string, string> properties = 7;
  string status = 8;  // "approved", "pending", "deprecated"
}
```

### SyncTMEntry — translation memory

```protobuf
message SyncTMEntry {
  string id = 1;
  string source_locale = 2;
  string target_locale = 3;
  string source_text = 4;
  string target_text = 5;
  string origin = 6;       // "human", "mt", "ai"
  double score = 7;
  map<string, string> properties = 8;
}
```

### SyncMedia — binary assets

Binary assets flow through the same protocol. Small assets inline; large assets are uploaded as separate blob chunks.

```protobuf
message SyncMedia {
  string id = 1;
  string item_name = 2;
  string mime_type = 3;
  string filename = 4;
  string alt_text = 5;
  int64 size = 6;

  // Exactly one:
  bytes inline_data = 10;      // small assets inlined in chunk
  string blob_key = 11;        // large assets: separately uploaded blob

  // Locale variants
  string locale = 12;
  string source_media_id = 13;

  map<string, string> properties = 14;
}
```

### SyncItemMeta — item metadata

Declares everything about an item — no guessing:

```protobuf
message SyncItemMeta {
  string name = 1;
  string format = 2;           // "json", "xliff2", "markdown" — declared, not guessed
  string encoding = 3;
  string collection = 4;
  string block_index_json = 5;
  string preview_html = 6;
  string source_language = 7;
  map<string, string> connector_data = 8;
}
```

### SyncManifest — commit request

```protobuf
message SyncManifest {
  string upload_id = 1;
  string project_id = 2;
  string stream = 3;

  repeated ChunkRef chunks = 4;
  repeated SyncItemMeta items = 5;

  string actor_id = 6;
  string workspace_slug = 7;
  string connector_id = 8;
  map<string, string> context = 9;
}

message ChunkRef {
  int32 index = 1;
  string content_type = 2;
  string hash = 3;          // SHA-256 of compressed bytes
  int32 record_count = 4;
  int64 byte_size = 5;
}
```

### Transport layer

The API server is a **control plane only**. All data flows directly between client and blob storage.

```
Push:
  1. Client → API: POST /sync/push/init (Merkle tree hashes)
     ← { upload_id, changed_items, new_items, deleted_items }
     Fast path: root_hash match → { status: "unchanged" }

  2. For each changed item:
     Client → API: POST /sync/push/diff (block hashes for that item)
     ← { needed, deleted, conflicts, chunk_urls }

  3. Client → Blob Storage (parallel, direct, only needed records):
     PUT chunk_urls[0] ← zstd-compressed SyncChunk
     ...

  4. Client → API: POST /sync/push/commit
     ← 202 { push_id }

  5. Worker reads chunks, decompresses, routes by content type, stores

Pull:
  1. Client → API: GET /sync/pull?cursor=X&limit=1000
     ← zstd-compressed SyncPullResponse (changes since cursor)
```

**Transport modes**: The init response includes a `transport` field:

- `"direct"` — SAS URLs for Azure Blob (production)
- `"proxy"` — upload through the API server (local dev, self-hosted)

The client library handles both transparently. The proxy path is the fallback for deployments without Azure Blob.

### Compression: zstd with trained dictionary

- Dictionary trained on representative sync payloads (blocks + terms + TM)
- 80-90% compression for translation data
- Streaming: no full-payload buffering
- Library: `github.com/klauspost/compress/zstd`
- Dictionary shipped with CLI binary (~32-112KB)

### Chunking strategy

Chunks are sealed by **byte size**, not record count:

- Target: ~1MB uncompressed per chunk
- Accumulate records until the chunk reaches the target, then seal it
- This prevents oversized chunks from records with rich annotations
- Each chunk carries one content type (simpler routing in worker)
- Each chunk independently retryable

### Upload budget enforcement

SAS URLs are generated with constraints:

- `Content-Length` limit per chunk (2x target chunk size as headroom)
- Total upload budget per push (project storage quota)
- Server validates total bytes on commit against the budget
- Prevents malicious clients from uploading unlimited data to storage

### Hash caching

Project content hashes are cached in Redis for fast diff computation:

- Key: `sync:hashes:{project_id}:{item_name}` → hash map of block_id:content_hash
- Key: `sync:item_hashes:{project_id}` → hash map of item_name:item_hash
- Invalidated on block write (event-driven)
- Falls back to PostgreSQL query on cache miss
- Prevents loading 100K hashes from PG on every push/init

### Pull response

```protobuf
message SyncPullResponse {
  int64 cursor = 1;
  bool has_more = 2;

  repeated SyncBlock blocks = 10;
  repeated SyncTerm terms = 11;
  repeated SyncTMEntry tm_entries = 12;
  repeated SyncQAResult qa_results = 13;
  repeated SyncActivity activities = 14;
  repeated SyncMedia media = 15;   // metadata only; binary via blob URLs
}
```

### Worker processing

The worker routes chunks by content type:

```go
for _, chunk := range manifest.Chunks {
    data := downloadAndDecompress(chunk)
    switch chunk.ContentType {
    case "blocks":
        storeBlocks(data.Blocks, manifest)
    case "terms":
        storeTerms(data.Terms, manifest)
    case "tm":
        storeTMEntries(data.TMEntries, manifest)
    case "media":
        storeMedia(data.Media, manifest)
    }
}
invalidateHashCache(manifest.ProjectID)
publishEventPushCompleted(manifest)
```

## Implementation

### Phase 1: Protobuf contract

- Define all message types in `platform/proto/v1/sync.proto`
- Generate Go code
- Block ↔ SyncBlock converters with content_hash computation
- Term ↔ SyncTerm, Media ↔ SyncMedia converters
- Merkle tree hash computation (item-level from block hashes)

### Phase 2: Diff engine

- Server-side Merkle tree comparison (item-level, then block-level)
- Root hash fast path
- Conflict detection (expected_hash mismatch)
- Redis hash cache with event-driven invalidation
- Fallback to PG on cache miss

### Phase 3: Chunked upload infrastructure

- Extend `BlobStore` with `StageChunk`/`CommitUpload`
- Azure implementation (Block Blob StageBlock + CommitBlockList)
- Local filesystem implementation
- SAS URL generation with upload budget constraints
- Proxy upload endpoint for non-Azure deployments

### Phase 4: Push endpoints + worker

- `POST /sync/push/init` — Merkle diff, return changed items
- `POST /sync/push/diff` — block-level diff per item, return needed IDs + chunk URLs
- `POST /sync/push/commit` — validate manifest, check budget, enqueue worker
- Worker: download chunks, decompress, route by type, store, invalidate cache

### Phase 5: zstd compression

- Train dictionary on representative payloads
- `core/compression/` package with encoder/decoder pool
- Apply to chunk serialization + pull responses

### Phase 6: Client + CLI

- `BowrainClient`: `PushInit` (Merkle hashes), `PushDiff`, `UploadChunks`, `PushCommit`
- bowrain CLI: local Merkle tree computation from `.bowrain/` project
- bowrain CLI: byte-size-based chunking with parallel upload, progress bar
- bowrain CLI: conflict reporting (`bowrain push` shows conflicts, `--force` to overwrite)

### Phase 7: Pull

- Rich pull with full SyncBlock + SyncTerm + SyncMedia model
- zstd-compressed responses
- Cursor pagination

### Phase 8: Remove v1

- Delete old sync endpoints, types, detectFormat
- Clean dev DB reset

## Alternatives Considered

- **Flat hash manifest**: Sends all record hashes in one request. Scales to ~10K records but breaks at 100K+ (70MB manifest). The Merkle tree approach keeps the init request small (item count, not block count) while enabling block-level diff only where needed.

- **tus protocol**: Opaque byte streams, not structured data. Good for media, wrong for typed localization content.

- **gRPC streaming**: No resumability for bulk push. Doesn't support async "upload to blob, process later" model.

- **Last-write-wins without conflict detection**: Acceptable for source content (single source of truth) but causes silent data loss for translations. Expected-hash comparison catches conflicts with minimal overhead.

- **Full dataset push without diff**: Functional but wasteful. A 10K-block project with 5 changes transfers 10K blocks. With Merkle diff, it transfers item-level hashes (small), block-level hashes for ~1 changed item, then 5 blocks.

## Consequences

- **Merkle diff sync**: Init request is always small (item-level hashes). Block-level comparison only for changed items. Root hash fast path for unchanged projects.
- **Conflict detection**: Expected-hash prevents silent overwrites. Clients see conflicts and choose to resolve or force-push.
- **Redis hash cache**: Diff computation doesn't hit PG on every push. Event-driven invalidation keeps cache fresh.
- **Byte-size chunking**: Chunks are ~1MB regardless of record size. No oversized chunks from rich annotations.
- **Transport modes**: Direct-to-storage (Azure) and proxy (local/self-hosted) behind same client API.
- **Upload budget**: SAS URLs have size constraints. Server validates total on commit.
- API server is a thin control plane — never touches content bytes
- Typed content layer is extensible: new content types without transport changes
- Full Block model survives the sync boundary — no data loss
- Terminology, TM, and binary assets sync through the same protocol
- No heuristic detection — clients declare everything explicitly
- 80-90% compression reduces transfer and storage costs
- Parallel, resumable uploads — resilient to network issues
- Worker processes chunks independently — bounded memory per chunk
