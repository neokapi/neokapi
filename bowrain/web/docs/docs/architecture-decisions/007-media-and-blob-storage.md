---
id: 007-media-and-blob-storage
sidebar_position: 7
title: "AD-007: Media and Blob Storage"
---

# AD-007: Media and Blob Storage

## Summary

Binary assets — images embedded in DOCX, audio files, screenshots, video
thumbnails — flow through a dedicated data plane. The framework defines a
`BlobStore` interface with content-addressed keys; Bowrain provides Azure
Blob Storage and local filesystem implementations. The ContentStore
carries asset metadata and locale variants; binaries live in blob storage.
Clients upload and download directly via SAS URLs, bypassing the API
server. The framework's `PartMedia` pipeline stage carries a blob
reference rather than the binary.

## Context

Localization content is not only text. DOCX documents contain embedded
images with captions that need translation; screenshots contain UI text
that needs OCR; voiceover audio carries dialog that needs re-recording
per locale; design exports carry alt text, chart labels, and
culturally-specific imagery. A localization platform that cannot track,
localize, and round-trip binary assets silently drops them.

Three concerns need addressing together:

1. **Sync gap.** Embedded binaries should flow from client to server and
   back without being dropped on the floor.
2. **Localization gap.** Logical assets have locale-specific variants
   (translated screenshots, dubbed audio, regionalized imagery).
3. **Processing gap.** Some binary assets (screenshots with text, charts
   with embedded labels) require processing the CLI cannot perform
   locally (OCR, ASR, vision-model analysis). The server is the natural
   home for these.

The industry pattern is clear: store binaries in object storage, track
metadata in the relational database, reference binaries by
content-addressed key, and deliver them via pre-signed URLs for direct
client access.

## Decision

### BlobStore Interface (Framework)

The `BlobStore` interface lives in `core/storage/` alongside the
framework's SQLite helpers, with zero platform dependencies:

```go
package storage

var ErrNotSupported = errors.New("operation not supported by this backend")
var ErrBlobNotFound = errors.New("blob not found")

type BlobStore interface {
    Upload(ctx context.Context, data []byte, opts UploadOptions) (*BlobRef, error)
    Download(ctx context.Context, key string) (io.ReadCloser, error)
    GenerateUploadURL(ctx context.Context, key string, opts SignOptions) (string, error)
    GenerateDownloadURL(ctx context.Context, key string, opts SignOptions) (string, error)
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
}

type UploadOptions struct {
    ContentType string // MIME type (e.g. "image/png")
    Filename    string // Original filename for Content-Disposition
}

type SignOptions struct {
    ExpiresIn time.Duration // Pre-signed URL TTL (default 1 hour)
}

type BlobRef struct {
    Key         string // SHA-256 hex of content
    Size        int64
    ContentType string
}
```

Design choices:

- **Content-addressed keys.** `Upload` computes `sha256(data)` and uses
  the hex digest as the storage key. If the key already exists, `Upload`
  returns the existing `BlobRef` without writing. This mirrors block
  content-addressing in the ContentStore.
- **Pre-signed URLs.** Clients upload and download directly to and from
  blob storage without proxying through the API server. Backends that
  can't generate pre-signed URLs return `ErrNotSupported`, and callers
  fall back to proxied upload/download.
- **Interface in the framework module.** The abstraction stays in
  `core/storage/` with no platform dependencies. Implementations live in
  `bowrain/storage/`.

### Azure Blob Storage Adapter (Bowrain)

The production implementation in `bowrain/storage/azureblob/` uses the
Azure SDK for Go with SAS token authentication:

```go
type Store struct {
    client        *azblob.Client
    containerName string
}

// New uses Azure Managed Identity (production)
func New(accountURL, containerName string) (*Store, error) {
    cred, _ := azidentity.NewDefaultAzureCredential(nil)
    client, _ := azblob.NewClient(accountURL, cred, nil)
    return &Store{client: client, containerName: containerName}, nil
}

// NewWithConnectionString is the local-dev fallback
func NewWithConnectionString(connStr, containerName string) (*Store, error) { ... }
```

Blob naming: `{project_id}/{key[0:2]}/{key[2:4]}/{key}` — project-scoped
with git-like sharding to avoid directory listing bottlenecks.

SAS URL generation uses User Delegation SAS under Managed Identity —
the server never handles a storage account key:

```go
func (s *Store) GenerateUploadURL(ctx context.Context, key string, opts SignOptions) (string, error) {
    expiry := opts.ExpiresIn
    if expiry == 0 { expiry = time.Hour }
    udk, _ := s.client.ServiceClient().GetUserDelegationCredential(ctx, ...)
    sasURL, _ := blobClient.GetSASURL(sas.BlobPermissions{Write: true, Create: true}, expiry, ...)
    return sasURL, nil
}
```

Environment variables:

| Variable                          | Description        | Example                                   |
| --------------------------------- | ------------------ | ----------------------------------------- |
| `BLOB_STORAGE_BACKEND`            | `azure` or `local` | `azure`                                   |
| `AZURE_STORAGE_ACCOUNT_URL`       | Account URL        | `https://myaccount.blob.core.windows.net` |
| `AZURE_STORAGE_CONTAINER`         | Container name     | `bowrain-assets`                          |
| `AZURE_STORAGE_CONNECTION_STRING` | Dev fallback       | `DefaultEndpointsProtocol=...`            |
| `BLOB_STORAGE_LOCAL_DIR`          | Local root         | `/var/lib/bowrain/blobs`                  |

### Local Filesystem Adapter

`bowrain/storage/localblob/` stores blobs as files on disk:

```go
type Store struct {
    rootDir string
}
```

Layout: `{rootDir}/{key[0:2]}/{key[2:4]}/{key}`.

`GenerateUploadURL` and `GenerateDownloadURL` return
`storage.ErrNotSupported`. The CLI detects this and falls back to direct
upload and download through proxy endpoints on the server:

```
POST /api/v1/projects/:id/assets/upload     (multipart/form-data)
GET  /api/v1/projects/:id/assets/:aid/download
```

These proxy endpoints exist only for backends without pre-signed URL
support. The Azure adapter always uses direct SAS upload and download.

**Alignment constraint:** the API server and background worker must use
the same blob store. In production, both use Azure Blob via Managed
Identity. In local dev, both must point to the same filesystem directory
(`BLOB_STORAGE_LOCAL_DIR` / `LOCAL_BLOB_DIR` set to the same path; the
dev Makefile pins both to `/tmp/bowrain-blobs`). Misaligned stores cause
push jobs to fail silently with "chunk download failed".

### Asset Metadata in the ContentStore

Binaries live in the BlobStore; asset metadata lives in the relational
database.

```sql
CREATE TABLE assets (
    id                TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    item_name         TEXT NOT NULL DEFAULT '',    -- source file this asset belongs to
    source_id         TEXT NOT NULL DEFAULT '',    -- format-reader-assigned ID within the item
    blob_key          TEXT NOT NULL,               -- content-addressed key in BlobStore
    mime_type         TEXT NOT NULL,
    filename          TEXT NOT NULL DEFAULT '',
    size_bytes        INTEGER NOT NULL DEFAULT 0,
    alt_text          TEXT NOT NULL DEFAULT '',
    properties        TEXT NOT NULL DEFAULT '{}',
    processing_status TEXT NOT NULL DEFAULT 'none',
    processing_hint   TEXT NOT NULL DEFAULT '',
    stream            TEXT NOT NULL DEFAULT 'main',
    created_at        TIMESTAMP NOT NULL,
    updated_at        TIMESTAMP NOT NULL
);
CREATE INDEX idx_assets_project_item ON assets(project_id, item_name);
CREATE UNIQUE INDEX idx_assets_blob ON assets(project_id, blob_key)
    WHERE stream = 'main';
```

`processing_status` values: `none`, `pending`, `processing`,
`processed`, `failed`. `processing_hint` values: `ocr`, `chart-text`,
`subtitle-extract`, `asr`.

Locale variants follow the "reference key + locale variants" pattern:

```sql
CREATE TABLE asset_variants (
    asset_id    TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    locale      TEXT NOT NULL,                     -- BCP-47 tag
    blob_key    TEXT NOT NULL,                     -- locale-specific binary
    status      TEXT NOT NULL DEFAULT 'pending',   -- pending, draft, approved
    mime_type   TEXT NOT NULL DEFAULT '',
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    properties  TEXT NOT NULL DEFAULT '{}',
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL,
    PRIMARY KEY (asset_id, locale)
);
```

Block-asset dependencies enable the "text changed → flag asset for
re-localization" pattern:

```sql
CREATE TABLE block_asset_refs (
    project_id TEXT NOT NULL,
    block_id   TEXT NOT NULL,
    asset_id   TEXT NOT NULL,
    ref_type   TEXT NOT NULL DEFAULT 'embedded',  -- embedded, context, generated
    stream     TEXT NOT NULL DEFAULT 'main',
    PRIMARY KEY (project_id, block_id, asset_id)
);
```

`ref_type=generated` assets (voiceover audio derived from dialogue text)
are flagged for re-processing when the source block's content hash
changes. `embedded` assets are informational — they help translators see
context but don't drive re-processing.

### Model Extensions

The framework's `model.Media` carries a blob reference, not the binary:

```go
type Media struct {
    ID         string
    MimeType   string
    Data       []byte            // Inline binary (small, pipeline-internal)
    BlobKey    string            // Content-addressed key in BlobStore (large assets)
    URI        string            // External reference (CDN URL, SAS URL)
    Filename   string            // Original filename
    AltText    string            // Extractable / translatable text
    Size       int64
    Properties map[string]string // Dimensions, duration, codec, etc.
}
```

The three storage modes are mutually prioritized: `BlobKey` > `URI` >
`Data`. Pipeline tools check `BlobKey` first (server-managed), then
`URI` (external reference), then `Data` (inline bytes). Small assets
(icons, thumbnails) flow inline through the pipeline; large assets
(high-res images, video) use blob storage.

### Format Reader Integration

The OpenXML reader (DOCX/PPTX) has an `ExtractMedia` flag:

```go
type Config struct {
    // ... other fields ...
    ExtractMedia bool // Emit PartMedia parts for embedded images/objects
}
```

When enabled:

1. The reader opens the DOCX ZIP and iterates `word/media/*` entries.
2. For each media file, it computes SHA-256 and creates `model.Media{ID,
   MimeType, BlobKey, Data, Filename, Size}`.
3. A `PartMedia` is emitted before the block that references the image.
4. In the referencing block's Span, `Data` points to
   `"ref:media:{media_id}"`, linking the Span to the Media.
5. The writer substitutes locale-variant Media by source ID when
   available.

Other binary-rich formats (EPUB, ODF, HTML-referenced images, PDF
embedded assets) follow the same pattern: a config flag enables
`PartMedia` emission and variant substitution on write.

### Sync Protocol Integration

The sync protocol ([AD-009: Sync Protocol](009-sync-protocol.md))
carries assets through the same `SyncChunk` envelope as blocks. Small
assets inline in a chunk; large assets upload as separate blobs with
the chunk carrying only the metadata.

Push algorithm extension (after block extraction):

```
1.5 Extract embedded assets → compute SHA-256 keys
    For each asset:
      a. Check .sync-cache → skip if key unchanged
      b. POST /assets/upload-url with key → server returns SAS URL (or "exists")
      c. If new: upload to blob storage via SAS URL
      d. POST /assets with metadata (key, mime, filename, item_name, source_id)
```

Pull algorithm extension:

```
5.5 For each asset variant (status=approved):
      a. GET /assets/:id → includes SAS download URL per locale
      b. Download locale-specific binary from blob storage
      c. Embed in localized output file (DOCX writer replaces image by source_id)
```

Sync cache tracks per-file asset hashes alongside block hashes so
unchanged binaries are skipped on subsequent syncs.

### Server-Side Processing

When the CLI cannot extract localizable content from an asset (text in
a screenshot, embedded chart labels), it pushes the raw binary tagged
with a `processing_hint`:

```
1. CLI pushes asset with ProcessingStatus="unprocessed", ProcessingHint="ocr"
2. Server emits EventAssetUploaded
3. Server-side tool (OCR, ASR, chart-text extractor) processes the asset:
   a. Extracts text → creates Blocks linked to the asset
   b. Updates ProcessingStatus="processed"
4. Extracted blocks appear in the regular block sync flow
5. On pull, CLI receives translated blocks + locale-variant assets
```

Processing tools run as server-side flows using the same `tool.Tool`
interface as CLI-side tools. Vision and speech models come from the
same `aiprovider.LLMProvider` abstraction as translation tools.

### REST API

Asset endpoints:

```
POST   /api/v1/projects/:id/assets/upload-url     # Get SAS upload URL
POST   /api/v1/projects/:id/assets                 # Register asset metadata
GET    /api/v1/projects/:id/assets                 # List assets (with download URLs)
GET    /api/v1/projects/:id/assets/:aid            # Get asset (with download URL)
DELETE /api/v1/projects/:id/assets/:aid
POST   /api/v1/projects/:id/assets/:aid/variants/upload-url
POST   /api/v1/projects/:id/assets/:aid/variants   # Upload locale variant metadata
GET    /api/v1/projects/:id/assets/:aid/variants
```

### Change Log Integration

Asset mutations appear in the existing change log so `kapi pull`
fetches only changed assets since the last cursor:

```
change_type values:
  asset_added       : new asset uploaded
  asset_modified    : asset metadata or binary updated
  asset_removed     : asset deleted
  variant_added     : locale variant uploaded
  variant_modified  : locale variant updated
  variant_approved  : locale variant approved
```

This unifies block and asset sync under one cursor: a single monotonic
sequence tracks both text and binary changes through the same
incremental sync mechanism.

### Configuration

Asset sync is opt-out, not opt-in:

```yaml
content:
  - path: docs/**/*.docx
    format: openxml
    assets: true           # sync embedded assets (default: true)
    asset_max_size: 50MB   # skip assets larger than this

assets:
  enabled: true            # master toggle (default: true)
  exclude:
    - "*.psd"
    - "*.ai"
    - "thumbnail_*"
  max_size: 100MB
```

Resolution order: `assets.enabled: false` globally, then
`content[].assets: false` per entry, then exclude patterns, then size
limits.

## Consequences

- Bowrain becomes asset-aware without becoming a DAM. It tracks locale
  variants, metadata, and dependencies but does not provide DAM
  features (rights management, brand portals, creative workflows).
- The sync protocol gains a binary data plane alongside the block JSON
  plane, coordinated by asset metadata in the ContentStore.
- Content-addressed deduplication applies to binaries — the same
  company logo embedded in 50 DOCX files is stored once in blob storage.
- Server-side processing enables capabilities the CLI can't provide:
  OCR, ASR, AI-powered image adaptation.
- Azure Blob Storage is the first-class binary backend; local
  filesystem serves development and testing.
- Format readers progressively adopt `PartMedia`. Formats that don't
  emit it continue working through the existing Span sentinel approach
  unchanged.
- One change log cursor covers both text and binary changes.

## Related

- [AD-004: Content Store and Versioning](004-content-store.md)
- [AD-008: Connector System](008-connector-system.md)
- [AD-009: Sync Protocol](009-sync-protocol.md)
- [AD-framework-002: Content Model](https://neokapi.github.io/web/neokapi/docs/architecture/002-content-model)
