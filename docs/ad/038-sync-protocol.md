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
- **Thin contract** — only block text survives the sync boundary; annotations, properties, display hints, connector data are lost
- **Block-only** — no protocol for syncing terminology, TM entries, QA results, or automation outputs
- **Heuristic format detection** — server guesses format from file extension

The sync protocol must be the **single, extensible transport** for all project data — not just blocks. It must be efficient enough for 100K+ blocks and resilient enough for unreliable networks.

## Decision

### Core design: Typed chunks over direct-to-storage transport

The sync protocol is built on two orthogonal layers:

1. **Transport layer**: Chunked, compressed, resumable, direct-to-storage upload/download
2. **Content layer**: Typed, versioned payloads that can evolve independently of the transport

```
Transport: how data moves
  ├── Chunked upload via SAS URLs (parallel, resumable)
  ├── zstd compression with trained dictionary
  └── Manifest-based commit

Content: what data moves (evolves independently)
  ├── SyncBlock     — translatable content with full model
  ├── SyncTerm      — terminology concepts
  ├── SyncTMEntry   — translation memory entries
  ├── SyncMedia     — binary assets (images, audio, video)
  ├── SyncQAResult  — quality check results
  ├── SyncActivity  — automation outputs
  └── (future types added without transport changes)
```

### Content-addressed diff sync

The protocol transfers only what changed since the last sync. This is the critical efficiency mechanism — a 10,000-block project where 5 blocks changed transfers only those 5 blocks.

**How it works:**

Every record has a content hash computed from its meaningful fields:
- **Blocks**: hash of source text + properties + annotations (already exists as `content_hash` via `model.ComputeIdentity`)
- **Terms**: hash of source term + translations + status
- **TM entries**: hash of source + target + locale
- **Media**: SHA-256 of binary content (already content-addressed in BlobStore)

The client maintains a local **hash manifest** — a map of `record_id → content_hash` for everything it has synced. On push, the client sends this manifest to the server. The server compares against its stored hashes and responds with the set of IDs that need uploading (new or changed). The client uploads only the diff.

```
Push (diff-based):
  1. Client computes content_hash for each local record
  2. Client → API: POST /sync/push/init
     Request: SyncPushInit {
       project_id, stream,
       hashes: { "block-id-1": "abc123", "block-id-2": "def456", ... },
       content_types: ["blocks", "terms"]
     }
  3. Server compares client hashes against stored hashes
     Response: SyncPushInitResponse {
       upload_id,
       needed: ["block-id-3", "block-id-7"],   // changed or new
       deleted: ["block-id-99"],                 // server has, client doesn't
       chunk_urls: [SAS URLs],                   // for uploading the diff
       unchanged_count: 9995                     // for progress reporting
     }
  4. Client uploads ONLY the needed records as chunks
  5. Client → API: POST /sync/push/commit
     Response: 202 { push_id }

Pull (cursor-based — already diff by design):
  1. Client → API: GET /sync/pull?cursor=X&limit=1000
     Response: only changes since cursor (already works this way)
```

**First sync** (no local hashes): Client sends all hashes; server responds with all IDs as "needed." Full upload, same as today but with the rich contract.

**Subsequent syncs**: Client sends hashes; server says "only these 5 changed." Dramatic reduction in transfer.

**Deletion detection**: If the client's hash manifest includes IDs the client no longer has locally, the `deleted` response tells the client what was removed upstream. The server can also detect client-side deletions by comparing the manifest against its stored set.

### Hash manifest optimization

For large projects (100K+ blocks), sending the full hash manifest on every push is itself expensive. Two optimizations:

**1. Manifest hash**: Client computes a single hash of the sorted hash manifest. If server's manifest hash matches, no diff needed — nothing changed.

```
POST /sync/push/init
  { manifest_hash: "sha256-of-sorted-hashes" }

Response (fast path):
  { status: "unchanged" }  // no upload needed
```

**2. Incremental manifest**: Client only sends hashes for records modified since the last sync cursor. The server fills in the rest from its stored state.

```
POST /sync/push/init
  { cursor: 12345, changed_hashes: { "block-3": "new-hash", "block-7": "new-hash" } }
```

### Transport layer

The API server is a **control plane only** — it generates upload URLs and enqueues jobs. All data flows directly between client and blob storage.

```
Push:
  1. Client → API: POST /sync/push/init (with hash manifest)
     ← { upload_id, needed: [...], chunk_urls: [SAS URLs] }

  2. Client → Blob Storage (parallel, direct, only needed records):
     PUT chunk_urls[0] ← zstd-compressed SyncChunk (diff only)
     PUT chunk_urls[1] ← zstd-compressed SyncChunk
     ...

  3. Client → API: POST /sync/push/commit
     Request: SyncManifest
     Response: 202 { push_id }

  4. Worker reads chunks, decompresses, routes by content type, stores

Pull:
  1. Client → API: GET /sync/pull?cursor=X&limit=1000
     Response: zstd-compressed SyncPullResponse (changes since cursor)
```

### Content layer: SyncChunk envelope

Each chunk is a typed envelope that can carry any content type. The envelope is versioned so the wire format can evolve without breaking the transport:

```protobuf
// SyncChunk is the unit of transfer. Each chunk contains a batch of
// typed records. A single push can mix content types across chunks.
message SyncChunk {
  int32 version = 1;          // envelope version (currently 1)
  string content_type = 2;    // "blocks", "terms", "tm", "qa", "activity", "media"
  int32 record_count = 3;

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

This separation means:
- **Adding a new content type** (e.g., brand voice scores) requires adding a field to `SyncChunk` and a handler in the worker — zero transport changes
- **Changing the block model** (e.g., adding a field to `SyncBlock`) is a protobuf-compatible evolution — old clients ignore new fields, new clients handle both
- **The transport layer never parses content** — it moves compressed bytes

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

Binary assets (images, audio, video, screenshots) flow through the same chunked transport. Small assets inline their bytes; large assets are uploaded as separate blob chunks and referenced by key.

```protobuf
message SyncMedia {
  string id = 1;
  string item_name = 2;       // which item this asset belongs to
  string mime_type = 3;        // "image/png", "audio/mp3"
  string filename = 4;
  string alt_text = 5;         // accessible alternative text
  int64 size = 6;

  // Exactly one of these:
  bytes inline_data = 10;      // small assets (< 256KB) inlined in the chunk
  string blob_key = 11;        // large assets: key of a separately uploaded blob chunk

  // Locale variants
  string locale = 12;          // locale this variant is for (empty = source)
  string source_media_id = 13; // links locale variant to source asset

  map<string, string> properties = 14;
}
```

**Small assets** (icons, badges < 256KB): Serialized inline in the SyncChunk alongside blocks. No separate upload needed.

**Large assets** (screenshots, videos): Client uploads the binary as a separate blob chunk (using the same SAS URL mechanism), then references the blob key in `SyncMedia.blob_key`. The worker stores the blob reference without loading the binary into memory.

This follows the existing `model.Media` pattern ([AD-029](./029-media-asset-localization.md)) where `Data []byte` is for inline and `BlobKey string` is for large assets stored in `BlobStore`.

### SyncItemMeta — item metadata

Declares everything about an item — no guessing:

```protobuf
message SyncItemMeta {
  string name = 1;
  string format = 2;           // "json", "xliff2", "markdown" — declared, not guessed
  string encoding = 3;         // "utf-8"
  string collection = 4;
  string block_index_json = 5;
  string preview_html = 6;
  string source_language = 7;
  map<string, string> connector_data = 8;
}
```

### SyncPushInit — diff negotiation

The init request carries content hashes so the server can compute the diff:

```protobuf
message SyncPushInit {
  string project_id = 1;
  string stream = 2;
  repeated string content_types = 3;  // what types the client wants to push

  // Hash manifest: record_id → content_hash.
  // Server compares against stored hashes to determine the diff.
  map<string, string> hashes = 4;

  // Optimization: if manifest_hash matches server's, skip diff computation.
  string manifest_hash = 5;  // SHA-256 of sorted(hashes)

  // Incremental mode: only hashes changed since this cursor.
  // Server fills in unchanged hashes from its state.
  int64 cursor = 6;
}

message SyncPushInitResponse {
  string upload_id = 1;

  // Records the client must upload (new or changed).
  repeated string needed = 2;

  // Records the server has that the client didn't include (deletions).
  repeated string deleted = 3;

  // Pre-signed URLs for chunk uploads (one per estimated chunk).
  repeated string chunk_urls = 4;

  // Stats for progress reporting.
  int32 unchanged_count = 5;
  int32 needed_count = 6;

  // Fast path: if manifest_hash matched, status="unchanged" and no upload needed.
  string status = 7;  // "diff_computed", "unchanged"
}
```

### SyncManifest — commit request

Describes the complete push with all context:

```protobuf
message SyncManifest {
  string upload_id = 1;
  string project_id = 2;
  string stream = 3;

  // What was uploaded
  repeated ChunkRef chunks = 4;
  repeated SyncItemMeta items = 5;

  // Who and why
  string actor_id = 6;
  string workspace_slug = 7;
  string connector_id = 8;
  map<string, string> context = 9;  // extensible push metadata
}

message ChunkRef {
  int32 index = 1;
  string content_type = 2;  // which content type this chunk carries
  string hash = 3;          // SHA-256 of compressed bytes
  int32 record_count = 4;
  int64 byte_size = 5;
}
```

### SyncPullResponse — rich pull

```protobuf
message SyncPullResponse {
  int64 cursor = 1;
  bool has_more = 2;

  // Mixed content types in a single response
  repeated SyncBlock blocks = 10;
  repeated SyncTerm terms = 11;
  repeated SyncTMEntry tm_entries = 12;
  repeated SyncQAResult qa_results = 13;
  repeated SyncActivity activities = 14;
  repeated SyncMedia media = 15;   // metadata only; binary via blob URLs
}
```

### Compression: zstd with trained dictionary

- Dictionary trained on representative sync payloads (blocks + terms + TM)
- 80-90% compression for translation data
- Streaming: no full-payload buffering
- Library: `github.com/klauspost/compress/zstd`
- Dictionary shipped with CLI binary (~32-112KB)

### Chunking strategy

- ~500 records per chunk (blocks, terms, or TM entries)
- Target ~1MB compressed per chunk
- Each chunk carries one content type (simpler routing in worker)
- A push can have mixed-type chunks: 18 block chunks + 2 term chunks
- Each chunk independently retryable

### Direct-to-storage upload

- **Azure**: SAS URLs with Write permission, 1-hour expiry
- **Local dev**: Simple upload endpoint or local filesystem paths
- Client uploads chunks in parallel (4-8 goroutines)
- `BlobStore` interface extended with `StageChunk`/`CommitUpload`

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
        storeMedia(data.Media, manifest)  // refs blob keys, no binary in memory
    // future types handled here
    }
}
publishEventPushCompleted(manifest)
```

## Implementation

### Phase 1: Protobuf contract
- Define all message types in `platform/proto/v1/sync.proto`
- Generate Go code
- Block ↔ SyncBlock converters with content_hash computation
- Term ↔ SyncTerm converters
- Media ↔ SyncMedia converters

### Phase 2: Diff engine
- Server-side hash comparison: load stored hashes, compute diff against client manifest
- Manifest hash fast path (single hash comparison, no per-record diff)
- Incremental manifest mode (cursor-based, only changed hashes)
- Deletion detection (set difference between client and server manifests)

### Phase 3: Chunked upload infrastructure
- Extend `BlobStore` with `StageChunk`/`CommitUpload`
- Azure implementation (Block Blob StageBlock + CommitBlockList)
- Local filesystem implementation
- SAS URL generation for chunk upload

### Phase 4: Push endpoints + worker
- `POST /sync/push/init` — diff negotiation, generate upload URLs for needed records only
- `POST /sync/push/commit` — validate manifest, enqueue worker
- Worker: download chunks, decompress, route by type, store
- Remove old sync push handler and v1 endpoints

### Phase 5: zstd compression
- Train dictionary on representative payloads
- `core/compression/` package
- Apply to chunk serialization + pull responses

### Phase 6: Client + CLI
- `BowrainClient`: `PushInit` (with hash manifest), `UploadChunks`, `PushCommit`
- bowrain CLI: local hash manifest storage (`.bowrain/.sync-hashes`)
- bowrain CLI: chunked parallel push with progress bar, diff stats
- bowrain CLI: `bowrain push --terms` to include terminology

### Phase 7: Pull
- Rich pull with full SyncBlock + SyncTerm + SyncMedia model
- zstd-compressed responses
- Cursor pagination (already diff by design)

### Phase 8: Remove v1
- Delete old sync endpoints, types, detectFormat
- Clean dev DB reset

### Phase 7: Remove v1
- Delete old `HandleSyncPush`, `HandleSyncPull`, `HandleSyncGetBlocks`
- Delete old `SyncPushRequest`, `BlockInput`, `SyncPushResponse`
- Delete `detectFormat` workaround
- Clean DB reset for dev

## Alternatives Considered

- **tus protocol**: Opaque byte streams, not structured data. Good for media, wrong for typed localization content.

- **gRPC streaming**: No resumability for bulk push. Doesn't support async "upload to blob, process later" model.

- **Versioned v1/v2 coexistence**: Adds maintenance burden for a protocol with zero production users. Clean replacement is simpler.

- **Single content type per push**: Forces separate pushes for blocks vs. terms. Mixed-type chunks are more ergonomic for connectors that produce blocks + terminology together.

- **Full dataset push without diff**: Requires sending all hashes on every push. For 100K+ blocks this manifest itself is ~3MB. The manifest hash fast path and incremental mode mitigate this.

## Consequences

- **Diff-based sync**: Only changed records transfer — a 5-block change in a 10,000-block project uploads 5 blocks, not 10,000
- **Content-addressed**: Identical content is never re-transferred or re-stored
- API server is a thin control plane — never touches content bytes
- Sync protocol is extensible: new content types without transport changes
- Full Block model survives the sync boundary — no data loss
- Terminology, TM, and binary assets sync through the same protocol as blocks
- No heuristic detection — clients declare everything explicitly
- 80-90% compression reduces transfer and storage costs
- Parallel, resumable uploads — resilient to network issues
- Worker processes chunks independently — bounded memory per chunk
- Clean break from v1 — no backward compatibility debt
- Manifest hash fast path: unchanged projects skip diff entirely (single hash comparison)
