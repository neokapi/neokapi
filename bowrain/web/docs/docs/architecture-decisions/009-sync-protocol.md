---
id: 009-sync-protocol
sidebar_position: 9
title: "AD-009: Sync Protocol"
---

# AD-009: Sync Protocol

## Summary

The sync protocol is Bowrain's single, extensible transport for project
data — blocks, terms, TM entries, media, QA results, and automation
outputs. It uses a two-level Merkle tree for minimal-transfer diff
negotiation, a typed `SyncChunk` envelope for extensibility, zstd
compression (with an optional, not-yet-shipped dictionary hook),
byte-size chunking, and direct-to-storage uploads via pre-signed SAS
URLs. Push is asynchronous:
the client uploads chunks to object storage, the server enqueues a
background worker, the worker ingests with bulk INSERT under a single
transaction per item.

## Context

The sync protocol is the primary data exchange path between bowrain CLI,
bowrain-server, and any other client that speaks to the platform. It
carries the full Block model (not just text), terminology, TM, binary
assets, and automation results. It must scale to 100K+ blocks per
project, survive unreliable networks, detect concurrent writes, and stay
extensible as new content types arrive.

Several constraints shape the design:

- **API server responsiveness.** The API process is the control plane;
  it validates, negotiates, and returns quickly. Heavy DB work happens
  in the worker, not in the request lifecycle.
- **Bandwidth efficiency.** 100K-block projects can't afford to transfer
  the full dataset on every sync. Merkle trees identify changed subtrees
  minimally.
- **Concurrent writes.** Multiple clients may edit the same project
  simultaneously. Silent last-write-wins is unacceptable for
  translations.
- **Async ingestion.** Bulk INSERT with COPY-style batching outperforms
  per-row INSERT by 10-100x. The worker controls its own batching and
  retry cadence.
- **Extensibility.** New content types (automation results, review
  threads, audit events) must ship without breaking the transport.

## Decision

### Three Layers

```
1. Diff layer:      what needs to transfer (Merkle tree hash comparison)
2. Content layer:   typed, versioned payloads that evolve independently
3. Transport layer: chunked, compressed, resumable, direct-to-storage
```

### Diff Layer: Two-Level Merkle Tree

```
Project root hash
  ├── Item "en.json"         hash: abc123  (hash of sorted block hashes)
  │     ├── Block b1          hash: def456
  │     ├── Block b2          hash: ghi789
  │     └── ...
  ├── Item "messages.json"    hash: jkl012
  │     └── ...
  └── Terms collection        hash: mno345
```

Push negotiation:

1. **Init.** The client computes item-level hashes (sorted block hashes
   per item) and a project root hash. It sends
   `POST /sync/push/init` with these hashes.
2. **Fast path.** If the root hash matches, the server replies
   `{ status: "unchanged" }` — zero further work.
3. **Item diff.** The server compares item hashes:
   - Matching item hash → skip entirely (all blocks unchanged).
   - Mismatched hash → "need block-level diff for this item".
   - New item on the client → "upload all blocks".
   - Missing item (present on server, absent from client) → "deleted".
4. **Block diff.** For mismatched items, the client sends block hashes
   via `POST /sync/push/diff`. The server returns which block IDs are
   needed, which are deleted, which have conflicts, and the SAS URLs
   for uploading needed chunks.

Scaling:

| Project size | Items     | Init request       | Diff requests   |
| ------------ | --------- | ------------------ | --------------- |
| 1K blocks    | 5 items   | 5 hashes (~500B)   | 0-2 item diffs  |
| 10K blocks   | 20 items  | 20 hashes (~2KB)   | 0-5 item diffs  |
| 100K blocks  | 100 items | 100 hashes (~10KB) | 0-10 item diffs |

The init request is always small (item count, not block count).
Block-level comparison only happens for items that actually changed.

### Conflict Detection

Every hash comparison includes an `expected_hash` for records being
updated. If the server's current hash doesn't match the client's
expectation, the server flags a conflict instead of silently
overwriting. The client sees conflicts and chooses to resolve them or
force-push with `--force`.

```protobuf
message SyncPushInit {
  string project_id = 1;
  string stream = 2;
  repeated string content_types = 3;

  // Merkle tree: item-level hashes
  map<string, string> item_hashes = 4;

  // Fast path: root hash of all item hashes
  string root_hash = 5;

  // Terms/TM have their own collection hashes
  string terms_hash = 6;
  string tm_hash = 7;
}

message SyncPushInitResponse {
  string upload_id = 1;
  string status = 2;  // "unchanged", "diff_required", "full_upload"
  repeated string changed_items = 3;
  repeated string deleted_items = 4;
  repeated string new_items = 5;
  int32 unchanged_item_count = 6;
  string transport = 7;  // "direct" or "proxy"
}

message SyncItemDiff {
  string upload_id = 1;
  string item_name = 2;
  map<string, string> block_hashes = 3;  // block_id → content_hash
}

message SyncItemDiffResponse {
  repeated string needed = 1;     // blocks to upload
  repeated string deleted = 2;    // blocks server has that client doesn't
  repeated string conflicts = 3;  // blocks changed by another client
  repeated string chunk_urls = 4; // SAS URLs for uploading needed blocks
}
```

### Content Layer: SyncChunk Envelope

Each chunk is a typed envelope that can carry any content type.
Protobuf provides forward/backward compatibility — new fields are
additive, old clients ignore unknown fields:

```protobuf
message SyncChunk {
  string content_type = 1;    // "blocks", "terms", "tm", "media", "qa", "activity"
  int32 record_count = 2;

  // Exactly one populated (determined by content_type):
  repeated SyncBlock blocks = 10;
  repeated SyncTerm terms = 11;
  repeated SyncTMEntry tm_entries = 12;
  repeated SyncQAResult qa_results = 13;
  repeated SyncActivity activities = 14;
  repeated SyncMedia media = 15;
}
```

Adding a new content type requires adding a field to `SyncChunk` and a
handler in the worker — zero transport or diff changes. Changing a
content type (e.g. adding a field to `SyncBlock`) is a protobuf-
compatible evolution.

#### SyncBlock — Full Block Model

```protobuf
message SyncBlock {
  string id = 1;
  string item_name = 2;
  string name = 3;
  string type = 4;
  string mime_type = 5;
  bool translatable = 6;

  repeated Segment source = 7;
  string source_text = 8;

  map<string, SegmentList> targets = 9;

  map<string, string> properties = 10;
  bytes annotations_json = 11;

  bytes skeleton_json = 12;
  bool preserve_whitespace = 13;
  bytes display_hint_json = 14;
  bytes content_ref_json = 15;

  map<string, string> connector_data = 16;

  // Conflict detection: the hash the client last saw for this block
  string expected_hash = 17;
}
```

The full Block model — including structured source and target
segments, annotations, skeleton, display hints, and connector data —
survives the sync boundary. No data loss.

#### SyncTerm, SyncTMEntry, SyncMedia

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

  string locale = 12;
  string source_media_id = 13;
  map<string, string> properties = 14;
}
```

See [AD-007: Media and Blob Storage](007-media-and-blob-storage.md)
for the blob upload coordination.

#### SyncItemMeta and SyncManifest

Item metadata declares format, encoding, collection, source language,
and connector context — no heuristic detection:

```protobuf
message SyncItemMeta {
  string name = 1;
  string format = 2;           // declared, not guessed
  string encoding = 3;
  string collection = 4;
  string block_index_json = 5;
  string preview_html = 6;
  string source_language = 7;
  map<string, string> connector_data = 8;
}

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

### Transport Layer: Direct-to-Storage

The API server is a control plane only. All data flows directly between
client and blob storage.

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
     PUT chunk_urls[1] ← zstd-compressed SyncChunk
     ...

  4. Client → API: POST /sync/push/commit
     ← 202 { push_id }

  5. Worker reads chunks, decompresses, routes by content type, stores.

Pull:
  1. Client → API: GET /sync/pull?cursor=X&limit=1000
     ← zstd-compressed SyncPullResponse (changes since cursor)
```

The `init` response includes a `transport` field:

- `"direct"` — SAS URLs for Azure Blob (production).
- `"proxy"` — upload through the API server (local dev, self-hosted
  without Azure).

The client library handles both transparently.

### Compression

zstd, applied per chunk:

- Standard zstd at the default speed level. Repetitive translation data
  (the common case) compresses well — large repetitive payloads reach
  better than 10x in the package's own tests.
- Streaming — no full-payload buffering on client or worker.
- Library: `github.com/klauspost/compress/zstd`, wrapped by an
  encoder/decoder pool (`core/storage/compression`) for zero-allocation
  reuse across requests.
- **Dictionary support is an optional hook, not yet wired.** The pool
  accepts a trained dictionary (`zstd.WithEncoderDict` /
  `WithDecoderDicts`), and the client's `EnableCompression(dict)` takes
  one, but no dictionary is embedded in or shipped with the CLI — the
  default path passes `nil`, so production uses standard zstd without a
  shipped dictionary. A trained dictionary would most help small payloads,
  which lack enough data to build shared context on the fly; training and
  embedding one remains future work.

### Chunking

Chunks are sealed by byte size, not record count:

- Target: ~1MB uncompressed per chunk.
- Accumulate records until the chunk reaches the target, then seal.
- Prevents oversized chunks from records with rich annotations.
- Each chunk carries one content type (simpler routing in the worker).
- Each chunk independently retryable.

### Upload Budget Enforcement

SAS URLs are generated with constraints:

- `Content-Length` limit per chunk (2x target size as headroom).
- Total upload budget per push (project storage quota).
- Server validates total bytes on commit against the budget.
- Prevents malicious clients from uploading unlimited data.

### Hash Cache

Project content hashes are cached in Redis for fast diff computation:

- `sync:hashes:{project_id}:{item_name}` → map of block_id → content_hash.
- `sync:item_hashes:{project_id}` → map of item_name → item_hash.
- Invalidated on block write (event-driven).
- Falls back to PostgreSQL query on cache miss.

Without the cache, the server would load 100K hashes from PostgreSQL on
every push init. With it, diff negotiation is sub-100ms for
100K-block projects.

### Async Ingestion

Push is asynchronous. The API server's responsibility shrinks to:
validate auth, negotiate the diff, receive the manifest, enqueue a
worker job, return 202 immediately. The heavy DB work moves to the
worker.

```
Client → API → Blob Storage (chunks via SAS URLs) → 202 Accepted (push_id)
Worker → Read blob chunks → Decompress → Validate → Bulk INSERT → DB
Client → Poll /sync/status?push_id=X
```

The worker:

1. Reads chunks from storage (streaming, not all-in-memory).
2. Validates schema for each chunk.
3. Groups records by item and processes in batches.
4. Bulk INSERT using PostgreSQL multi-row INSERT with ON CONFLICT
   (10-100x faster than per-row).
5. Single transaction per item with proper error handling.
6. Publishes `EventPushCompleted` when all items are stored.
7. Invalidates the Redis hash cache.
8. Deletes the blob after successful processing.

Bulk INSERT example:

```sql
INSERT INTO blocks (...) VALUES
  ($1,...), ($2,...), ($3,...), ... ($50,...)
ON CONFLICT (project_id, id) DO UPDATE SET ...
```

For PostgreSQL, `COPY` is even faster but doesn't support upsert.
Multi-row INSERT with ON CONFLICT is the pragmatic choice.

### Rate Limiting

Per-project limits on the push endpoint:

- Max 10 pushes per minute per project (configurable).
- Max 50MB per push.
- Max 10,000 blocks per push.
- Returns 429 Too Many Requests when exceeded.

### Pull Pagination

`GET /sync/pull` is cursor-paginated to bound response size:

- Default limit: 1,000 blocks per response.
- Max limit: 10,000 blocks.
- Cursor-based pagination via `offset` parameter.
- Response includes `next_offset` when more blocks exist.

The pull response carries multiple content types in one stream:

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

### Worker Processing

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

### Endpoints

```
POST /api/v1/workspaces/:ws/projects/:id/sync/push/init
POST /api/v1/workspaces/:ws/projects/:id/sync/push/diff
POST /api/v1/workspaces/:ws/projects/:id/sync/push/commit
GET  /api/v1/workspaces/:ws/projects/:id/sync/pull
GET  /api/v1/workspaces/:ws/projects/:id/sync/status?push_id=X
GET  /api/v1/workspaces/:ws/projects/:id/changes
```

Workspace-scoped routes enforce tenancy at the transport layer.

### Operational Alignment

The API server and worker must use the **same blob store**. In
production, both use Azure Blob Storage via Managed Identity. In local
dev, both must point to the same filesystem directory
(`BLOB_STORAGE_LOCAL_DIR` / `LOCAL_BLOB_DIR` set to the same path).
Misaligned stores cause push jobs to fail silently with "chunk download
failed". The dev Makefile pins both to `/tmp/bowrain-blobs`; Azure
deployments pass `AZURE_STORAGE_ACCOUNT_URL` and
`AZURE_STORAGE_CONTAINER` to the worker's container app.

## Consequences

- The init request is always small (item-level hashes), regardless of
  project size. Block-level diff runs only for items that actually
  changed. Root hash fast path catches unchanged projects in a single
  round-trip.
- Conflict detection via `expected_hash` prevents silent overwrites;
  clients see conflicts and choose to resolve or force.
- Redis hash cache keeps diff computation sub-100ms for large
  projects.
- Byte-size chunking keeps chunks ~1MB regardless of record size —
  no oversized chunks from rich annotations.
- Transport modes: direct-to-storage (Azure) or proxy (local /
  self-hosted) behind the same client API.
- The API server never touches content bytes — it's a thin control
  plane. All data flows through blob storage.
- The typed content layer is extensible: new content types add a field
  to `SyncChunk` and a worker handler, with no transport changes.
- The full Block model survives the sync boundary — annotations,
  properties, skeleton, display hints, and connector data all round-trip.
- Terminology, TM, and binary assets sync through the same protocol.
- Per-chunk zstd reduces transfer and storage costs; an optional trained
  dictionary is supported by the compression pool but not yet shipped.
- Async ingestion keeps the API responsive under any push load; the
  worker batches with bulk INSERT for 10-100x faster DB writes.
- Rate limiting and pagination bound resource usage per tenant.

## Related

- [AD-004: Content Store and Versioning](004-content-store.md)
- [AD-005: Streams](005-streams.md)
- [AD-007: Media and Blob Storage](007-media-and-blob-storage.md)
- [AD-008: Connector System](008-connector-system.md)
- [AD-framework-002: Content Model](https://neokapi.github.io/web/neokapi/docs/architecture/002-content-model)
