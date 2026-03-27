---
id: 038-sync-protocol-v2
sidebar_position: 38
title: "AD-038: Sync Protocol v2 — Chunked, Resumable, Direct-to-Storage"
---
# AD-038: Sync Protocol v2 — Chunked, Resumable, Direct-to-Storage

## Context

The sync protocol ([AD-037](./037-async-content-ingestion.md)) moved push processing from the API server to a background worker, solving the "server holds DB connections" problem. However, the data still flows through the server as a single JSON blob — the server marshals the entire payload into memory, uploads it to blob storage, and the worker downloads and unmarshals it all. This has fundamental limitations:

- **Full payload in memory** on server, worker, and client
- **No streaming** — a 50MB push is held as `[]byte` three times
- **No resumability** — if upload fails at 90%, start from zero
- **No parallelism** — single-threaded upload/download
- **No compression** — JSON is verbose for repetitive translation data
- **Thin contract** — `BlockInput` carries only ID, text, name, type, item_name; the rich Block model (annotations, properties, display hints, content refs, skeleton) is lost in transit
- **Heuristic format detection** — `detectFormat("en.json")` guesses "json" from the extension instead of the client declaring the actual format

The sync protocol is the primary data ingestion path for Bowrain. It must be efficient, reliable, and carry the full richness of the content model.

## Decision

### Architecture: Chunked upload with manifest, direct-to-storage

The API server never touches content bytes. Clients upload compressed chunks directly to blob storage via pre-signed URLs, then submit a manifest that triggers background processing.

```
Push flow:
  1. Client → API: POST /v2/sync/push/init
     ← { upload_id, chunk_urls: [SAS URLs], metadata_schema_version }

  2. Client → Azure Blob (parallel, direct):
     PUT chunk_urls[0] ← zstd-compressed batch of blocks
     PUT chunk_urls[1] ← zstd-compressed batch of blocks
     ...

  3. Client → API: POST /v2/sync/push/commit
     → { upload_id, manifest }
     ← 202 Accepted { push_id }

  4. Worker reads chunks from blob, decompresses, validates, stores

Pull flow:
  1. Client → API: GET /v2/sync/pull
     ← zstd-compressed response with cursor pagination
```

### Rich sync contract

The sync contract carries the full Block model, not a thin subset:

```protobuf
// SyncBlock is the unit of content in the sync protocol.
message SyncBlock {
  string id = 1;
  string item_name = 2;
  string name = 3;
  string type = 4;
  string mime_type = 5;
  bool translatable = 6;

  // Source content
  repeated Segment source = 7;
  string source_text = 8;  // convenience: plain text of source

  // Translations per locale
  map<string, SegmentList> targets = 9;

  // Metadata
  map<string, string> properties = 10;
  map<string, bytes> annotations = 11;  // serialized annotations

  // Structure
  Skeleton skeleton = 12;
  bool preserve_whitespace = 13;
  DisplayHint display_hint = 14;
  ContentRef content_ref = 15;

  // Connector-specific data (extensible)
  map<string, string> connector_data = 16;
}
```

### Rich item metadata

Items declare their format, encoding, and connector context — no heuristic detection:

```protobuf
message SyncItemMeta {
  string name = 1;           // e.g., "src/locales/en.json"
  string format = 2;         // declared format: "json", "xliff2", "markdown"
  string encoding = 3;       // e.g., "utf-8"
  string collection = 4;     // collection name (auto-created if missing)
  string block_index = 5;    // serialized BlockIndex for preview
  string preview_html = 6;   // format-aware preview HTML
  string source_language = 7; // source locale for this item

  // Connector context (extensible)
  map<string, string> connector_data = 8;  // e.g., CMS entry ID, last sync hash
}
```

### Push manifest

The commit request describes what was uploaded and the project-level context:

```protobuf
message SyncPushManifest {
  string upload_id = 1;
  string project_id = 2;
  string stream = 3;

  // Chunks uploaded to blob storage
  repeated ChunkRef chunks = 4;

  // Item metadata
  repeated SyncItemMeta items = 5;

  // Push context
  string actor_id = 6;
  string workspace_slug = 7;
  string connector_id = 8;     // which connector produced this push
  map<string, string> context = 9;  // extensible push-level metadata
}

message ChunkRef {
  int32 index = 1;
  string hash = 2;        // SHA-256 of compressed chunk
  int32 block_count = 3;
  int64 byte_size = 4;
}
```

### Compression: zstd with dictionary

Translation data is highly repetitive (same keys, property names, annotation structures). A trained zstd dictionary captures these patterns for 80-90% compression:

- Dictionary trained on representative sync payloads
- Shipped with the CLI binary (32-112KB)
- Streaming compression — no full-payload buffering
- `Content-Encoding: zstd` for pull responses
- Library: `github.com/klauspost/compress/zstd`

### Chunking strategy

Blocks are grouped into chunks of ~500 blocks each. Each chunk is:
1. Serialized (protobuf or JSON with rich model)
2. Compressed with zstd + dictionary
3. Uploaded independently to blob storage

Chunk size target: ~1MB compressed. For a typical 10,000-block push:
- 20 chunks × ~500 blocks × ~2KB avg = 10MB raw
- Compressed to ~1-2MB total
- Upload in parallel (4-8 goroutines)
- Each chunk is independently retryable

### Direct-to-storage upload

The init endpoint generates pre-signed SAS URLs (Azure) or NATS object store keys (local). Clients upload directly to storage, bypassing the API server entirely:

- **Azure production**: SAS URLs with Write permission, 1-hour expiry
- **Local dev**: Direct POST to a simple upload endpoint (or local filesystem paths)

The API server's `GenerateUploadURL` already exists in the `BlobStore` interface.

### Resumability

Each chunk is an independent upload. If chunk 3 of 20 fails:
1. Client retries chunk 3 only
2. Other 19 chunks are already stored
3. Content-addressed chunks (SHA-256) make retries idempotent

The manifest commit checks that all chunks exist before triggering processing.

### Pull v2

Pull also benefits from the rich model:

```
GET /v2/sync/pull?cursor=X&limit=1000
Accept-Encoding: zstd

Response (zstd compressed):
{
  "changes": [SyncBlock with full model],
  "cursor": ...,
  "has_more": true
}
```

### Backward compatibility

v1 endpoints remain for existing clients. v2 is a new path (`/v2/sync/`). Migration is gradual — clients upgrade at their own pace.

## Implementation

### Phase 1: Rich contract + protobuf
- Define `SyncBlock`, `SyncItemMeta`, `SyncPushManifest` protobuf messages
- Generate Go code
- Implement serialization/deserialization
- Update Block ↔ SyncBlock converters

### Phase 2: Chunked upload infrastructure
- Add `StageChunk` and `CommitChunks` to `BlobStore` interface
- Azure implementation using Block Blob `StageBlock` + `CommitBlockList`
- Local implementation using temp directory assembly
- Pre-signed URL generation for chunk upload

### Phase 3: Push v2 endpoints
- `POST /v2/sync/push/init` — validate, generate upload URLs
- `POST /v2/sync/push/commit` — validate manifest, enqueue worker
- Worker: read chunks, decompress, validate, store with full metadata

### Phase 4: zstd compression
- Train dictionary on representative payloads
- `core/compression/` package with encoder/decoder pool
- Apply to chunk serialization and pull responses

### Phase 5: Client v2
- bowrain CLI: chunked push with parallel upload, resumability
- Client library: `PushInit`, `UploadChunks`, `PushCommit`
- Progress reporting to user

### Phase 6: Pull v2
- Rich pull response with full SyncBlock model
- zstd-compressed responses
- Cursor-based pagination (existing)

## Alternatives Considered

- **tus protocol**: Designed for opaque byte streams, not structured data. Adds protocol complexity (HEAD offset recovery, concatenation) without matching the "batch of typed blocks" model. The resumability benefit can be achieved more simply with independent chunk uploads.

- **gRPC streaming**: Excellent for real-time editor sync but lacks resumability for bulk push. Browser clients need grpc-web. Doesn't support the "upload to blob, process later" async model.

- **Single large upload with server-side chunking**: Still requires the API server to receive and hold the full payload. Defeats the purpose of direct-to-storage.

## Consequences

- API server never touches content bytes — stays thin and responsive
- Clients upload directly to blob storage in parallel — fast, resumable
- Rich contract preserves the full Block model through the sync boundary
- No heuristic format detection — clients declare formats explicitly
- Connector-specific metadata flows through the sync protocol
- zstd compression reduces transfer size by 80-90% for translation data
- Each chunk is independently retryable — resilient to network issues
- Protobuf serialization is compact and strongly typed
- v1 backward compatibility maintained
