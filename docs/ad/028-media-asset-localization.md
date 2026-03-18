---
id: 028-media-asset-localization
sidebar_position: 28
title: "AD-028: Media Asset Localization and Blob Storage"
---
# AD-028: Media Asset Localization and Blob Storage

## Context

Bowrain currently handles only text-based localization content. Binary assets — images embedded in DOCX files, audio files, screenshots, video thumbnails — flow through the pipeline as opaque placeholders (Span sentinels in Blocks) or are silently dropped. This creates three gaps:

1. **Sync gap**: When a user runs `bowrain push`, embedded images in DOCX/PPTX/EPUB files are not synced to the server. The server has no record of these assets, so server-side flows cannot reference or process them.

2. **Localization gap**: Some binary assets contain localizable content — text in screenshots, voiceover audio, culturally-specific imagery — but there is no workflow for managing locale-specific variants of the same logical asset.

3. **Processing gap**: Format extraction for some binary-rich formats (DOCX with embedded charts, InDesign files, Figma exports) may require tools or libraries unavailable in the Bowrain CLI environment. The server should be able to re-process such assets when the CLI cannot.

The existing `model.Media` type and `PartMedia` pipeline stage are defined but unused — no format reader emits `PartMedia` today. The content model, tool dispatch, plugin serialization, and flow tracing already support Media parts. This decision builds on that foundation.

### Industry Patterns

The localization industry converges on a common model for binary assets:

- **Reference key + locale variants**: One logical asset ID, multiple physical files per locale (Contentful, Unity Localization, XLIFF 2.0 Resource Data Module).
- **External binary storage**: TMS/XLIFF references assets by URI; binaries stored in DAM, CDN, or object storage — never inline in the localization database (Phrase, Crowdin, XTM Cloud).
- **Dependency tracking**: Text changes flag dependent audio/image assets for re-localization (Gridly game localization pattern).
- **Offline + pipeline hybrid**: Design assets (PSD, images) go through upload/download cycles; text-extractable media (subtitles, alt text) go through the translation pipeline (Phrase Studio, memoQ).

## Decision

### 1. Blob Storage Abstraction (framework module)

A new `BlobStore` interface in `core/storage/` abstracts binary asset storage, decoupled from the ContentStore which handles structured localization data:

```go
// core/storage/blob.go

// BlobStore provides content-addressed binary storage.
// Implementations must be safe for concurrent use.
type BlobStore interface {
    // Upload stores binary content and returns a storage key.
    // The key is content-addressed (SHA-256 of data) to enable deduplication.
    Upload(ctx context.Context, data []byte, opts UploadOptions) (*BlobRef, error)

    // Download retrieves binary content by storage key.
    Download(ctx context.Context, key string) (io.ReadCloser, error)

    // GenerateUploadURL returns a pre-signed URL for direct client upload.
    // Returns ErrNotSupported for backends that don't support pre-signed URLs.
    GenerateUploadURL(ctx context.Context, key string, opts SignOptions) (string, error)

    // GenerateDownloadURL returns a pre-signed URL for direct client download.
    // Returns ErrNotSupported for backends that don't support pre-signed URLs.
    GenerateDownloadURL(ctx context.Context, key string, opts SignOptions) (string, error)

    // Delete removes a blob by storage key.
    Delete(ctx context.Context, key string) error

    // Exists checks whether a blob with the given key exists.
    Exists(ctx context.Context, key string) (bool, error)
}

type UploadOptions struct {
    ContentType string // MIME type (e.g., "image/png")
    Filename    string // Original filename for Content-Disposition
}

type SignOptions struct {
    ExpiresIn time.Duration // SAS token / pre-signed URL TTL
}

type BlobRef struct {
    Key         string // Content-addressed storage key (SHA-256)
    Size        int64
    ContentType string
}
```

**Key design choices:**

- **Content-addressed keys** (SHA-256 of binary data): Identical images across documents are stored once. Same deduplication principle as blocks in the ContentStore.
- **Pre-signed URL support**: Clients upload/download directly to/from blob storage without proxying through the Bowrain server. This is critical for large assets.
- **Interface in framework module**: The abstraction lives in `core/storage/` alongside the existing SQLite helpers, with zero platform dependencies. Implementations live in `platform/` or `bowrain/`.

### 2. Azure Blob Storage Adapter (platform module)

The first blob storage implementation targets Azure Blob Storage with SAS token authentication:

```go
// platform/storage/azureblob/store.go

type AzureBlobStore struct {
    client        *azblob.Client
    containerName string
}

func New(accountURL, containerName string, cred azcore.TokenCredential) *AzureBlobStore
func NewWithConnectionString(connStr, containerName string) *AzureBlobStore
```

**SAS token flow:**

```
CLI push:
  1. CLI extracts asset, computes SHA-256 key
  2. CLI calls server: POST /api/v1/projects/:id/assets/upload-url
     → Server checks if key already exists (dedup), returns SAS upload URL
  3. CLI uploads directly to Azure Blob Storage via SAS URL
  4. CLI reports asset metadata to server alongside block push

CLI pull:
  1. Server returns asset metadata with SAS download URLs
  2. CLI downloads directly from Azure Blob Storage
  3. CLI writes localized files with embedded assets
```

**Azure Managed Identity**: Like the existing PostgreSQL integration (`platform/storage/postgres.go` `OpenPostgresAzure()`), the Azure adapter uses `azidentity.DefaultAzureCredential` for passwordless authentication in production. Connection string fallback for local development.

**Filesystem adapter** for local development and testing:

```go
// platform/storage/localblob/store.go

type LocalBlobStore struct {
    rootDir string
}
```

Stores blobs as `{rootDir}/{key[0:2]}/{key[2:4]}/{key}` (git-like sharding). `GenerateUploadURL`/`GenerateDownloadURL` return `ErrNotSupported` — the CLI uses direct Upload/Download when talking to a local server.

### 3. Asset Metadata in ContentStore

A new `assets` table tracks metadata about binary assets. Binaries live in BlobStore; only metadata lives in the relational database:

```sql
CREATE TABLE assets (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    item_name    TEXT NOT NULL DEFAULT '',    -- source file this asset belongs to
    source_id    TEXT NOT NULL DEFAULT '',    -- format-reader-assigned ID within the item
    blob_key     TEXT NOT NULL,               -- content-addressed key in BlobStore
    mime_type    TEXT NOT NULL,
    filename     TEXT NOT NULL DEFAULT '',    -- original filename
    size_bytes   INTEGER NOT NULL DEFAULT 0,
    alt_text     TEXT NOT NULL DEFAULT '',    -- extractable localized text
    properties   TEXT NOT NULL DEFAULT '{}',  -- JSON: dimensions, duration, codec, etc.
    stream       TEXT NOT NULL DEFAULT 'main',
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_assets_project_item ON assets(project_id, item_name);
CREATE UNIQUE INDEX idx_assets_blob ON assets(project_id, blob_key);
```

**Locale variants** are stored in a separate table, following the "reference key + locale variants" pattern:

```sql
CREATE TABLE asset_variants (
    asset_id     TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    locale       TEXT NOT NULL,               -- BCP-47 tag
    blob_key     TEXT NOT NULL,               -- locale-specific binary in BlobStore
    status       TEXT NOT NULL DEFAULT 'pending',  -- pending, draft, approved
    mime_type    TEXT NOT NULL DEFAULT '',    -- may differ from source (e.g., different codec)
    size_bytes   INTEGER NOT NULL DEFAULT 0,
    properties   TEXT NOT NULL DEFAULT '{}',
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (asset_id, locale)
);
```

**ContentStore additions:**

```go
// New methods on ContentStore interface
StoreAsset(ctx context.Context, projectID, stream string, asset *Asset) error
GetAsset(ctx context.Context, projectID, stream, assetID string) (*Asset, error)
ListAssets(ctx context.Context, projectID, stream, itemName string) ([]*Asset, error)
DeleteAsset(ctx context.Context, projectID, stream, assetID string) error

StoreAssetVariant(ctx context.Context, projectID string, variant *AssetVariant) error
GetAssetVariant(ctx context.Context, projectID, assetID, locale string) (*AssetVariant, error)
ListAssetVariants(ctx context.Context, projectID, assetID string) ([]*AssetVariant, error)
```

### 4. Model Extensions

Extend `model.Media` to carry blob references alongside (or instead of) inline data:

```go
// Updated core/model/media.go

type Media struct {
    ID         string
    MimeType   string
    Data       []byte            // Inline binary (small assets, pipeline-internal)
    BlobKey    string            // Content-addressed key in BlobStore (large assets)
    URI        string            // External reference (CDN URL, SAS URL)
    Filename   string            // Original filename
    AltText    string            // Extractable/translatable text
    Size       int64             // Size in bytes
    Properties map[string]string // Dimensions, duration, codec, etc.
}
```

The three storage modes are mutually prioritized: `BlobKey` > `URI` > `Data`. Pipeline tools check `BlobKey` first (server-managed), then `URI` (external reference), then `Data` (inline bytes).

### 5. Sync Protocol Extensions

**Push**: Assets are synced during `bowrain push` alongside blocks, unless excluded by config:

```yaml
# config.yaml additions
content:
  - path: docs/**/*.docx
    format: openxml
    assets: true              # sync embedded assets (default: true)
    asset_max_size: 50MB      # skip assets larger than this

  - path: src/locales/**/*.json
    format: json
    assets: false             # no binary assets in JSON files

# Project-wide asset exclusions
assets:
  exclude:
    - "*.psd"                 # skip Photoshop files
    - "*.ai"                  # skip Illustrator files
  max_size: 100MB             # global max asset size
```

**New sync API endpoints:**

```
POST   /api/v1/projects/:id/assets/upload-url    # Get SAS upload URL
POST   /api/v1/projects/:id/assets               # Register asset metadata
GET    /api/v1/projects/:id/assets                # List assets (with download URLs)
GET    /api/v1/projects/:id/assets/:aid           # Get asset (with download URL)
DELETE /api/v1/projects/:id/assets/:aid           # Delete asset
POST   /api/v1/projects/:id/assets/:aid/variants  # Upload locale variant
GET    /api/v1/projects/:id/assets/:aid/variants  # List locale variants
```

**Push algorithm extension (step 1.5, after block extraction):**

```
1.  Scan local files → extract blocks → compute hashes
1.5 Extract embedded assets → compute SHA-256 keys
    For each asset:
      a. Check .sync-cache → skip if key unchanged
      b. POST /assets/upload-url with key → server returns SAS URL (or "exists")
      c. If new: upload to blob storage via SAS URL
      d. POST /assets with metadata (key, mime, filename, item_name, source_id)
2.  Diff block hashes against .sync-cache → identify changed blocks
    ...
```

**Pull algorithm extension (step 5.5, after writing blocks):**

```
5.  For each item with changes: write translated files
5.5 For each asset variant (status=approved):
      a. GET /assets/:id → includes SAS download URL per locale
      b. Download locale-specific binary from blob storage
      c. Embed in localized output file (DOCX writer replaces image by source_id)
6.  Update .sync-cache
```

**Sync cache extension:**

```json
{
  "files": {
    "docs/manual.docx": {
      "mtime": "...",
      "blocks": { ... },
      "assets": {
        "image1.png": "sha256:a1b2c3...",
        "chart.emf": "sha256:d4e5f6..."
      }
    }
  }
}
```

### 6. Format Reader Changes (Progressive)

Format readers opt into asset extraction progressively. The existing Span sentinel approach continues working — formats that don't emit `PartMedia` are unaffected.

**Phase 1: OpenXML (DOCX/PPTX)**

The OpenXML reader gains a config option to emit `PartMedia` for embedded images:

```go
// openxml/config.go addition
type Config struct {
    // ... existing fields ...
    ExtractMedia bool // Emit PartMedia parts for embedded images/objects
}
```

When `ExtractMedia` is true:
- Images from `word/media/` are emitted as `PartMedia` with `BlobKey` (SHA-256 of image bytes)
- The image Span sentinel (`\uE101`) in blocks gets a `Data` field linking to the Media's `ID`
- The OpenXML writer reconstructs the document using locale-specific variants when available

**Phase 2: Other formats** — EPUB (images in XHTML), ODF, HTML (referenced images), PDF (embedded assets) follow the same pattern: config flag → PartMedia emission → writer reconstruction.

### 7. Server-Side Re-Processing

When the CLI cannot extract localizable content from an asset (e.g., text in a screenshot, embedded charts with labels), it pushes the raw binary and tags it for server-side processing:

```go
type Asset struct {
    // ... fields from section 3 ...
    ProcessingStatus string // "unprocessed", "processing", "processed", "failed"
    ProcessingHint   string // "ocr", "chart-text", "subtitle-extract", etc.
}
```

**Server-side processing flow:**

```
1. CLI pushes asset with ProcessingStatus="unprocessed", ProcessingHint="ocr"
2. Server queues asset for processing (EventBus: EventAssetUploaded)
3. Server-side tool (OCR, ASR, chart-text extractor) processes the asset:
   a. Extracts text → creates Blocks linked to the asset
   b. Updates ProcessingStatus="processed"
4. Extracted blocks appear in the regular block sync flow
5. On pull, CLI receives translated blocks + locale-variant assets
```

This leverages the existing `EventBus` and automation system ([AD-011](./011-automation.md)). Processing tools run as server-side flows, using the same `tool.Tool` interface as CLI-side tools.

**Progressive capability:**
- Phase 1: CLI extracts what it can (alt text, simple metadata). Assets it can't process are pushed raw.
- Phase 2: Server gains OCR tool (AI-powered, using existing `ai/provider.LLMProvider` with vision models).
- Phase 3: Server gains ASR/subtitle tools for audio/video assets.
- Phase 4: Server gains image generation tools for locale-variant image creation (text overlay replacement).

### 8. Block-Asset Dependencies

Assets can be linked to the blocks they appear in, enabling the Gridly-style "text changed → flag asset for re-localization" pattern:

```sql
CREATE TABLE block_asset_refs (
    project_id TEXT NOT NULL,
    block_id   TEXT NOT NULL,
    asset_id   TEXT NOT NULL,
    ref_type   TEXT NOT NULL DEFAULT 'embedded', -- embedded, context, generated
    stream     TEXT NOT NULL DEFAULT 'main',
    PRIMARY KEY (project_id, block_id, asset_id)
);
```

When a block's source text changes (detected via content hash), all linked assets with `ref_type='generated'` (e.g., voiceover audio generated from dialogue text) are flagged for re-processing. Assets with `ref_type='embedded'` (e.g., image in a DOCX paragraph) are informational — they help translators understand context but don't trigger re-processing.

### 9. Change Log Integration

Asset mutations are recorded in the existing change log for incremental sync:

```
change_type values (new):
  - asset_added       : new asset uploaded
  - asset_modified    : asset metadata or binary updated
  - asset_removed     : asset deleted
  - variant_added     : locale variant uploaded
  - variant_modified  : locale variant updated
  - variant_approved  : locale variant approved
```

This allows `bowrain pull` to fetch only changed assets since the last cursor, using the same cursor-based incremental sync mechanism as blocks.

## Implementation Phases

### Phase 1: Foundation (BlobStore + Azure adapter + local adapter)
- `BlobStore` interface in `core/storage/`
- Azure Blob Storage adapter in `platform/storage/azureblob/`
- Local filesystem adapter in `platform/storage/localblob/`
- Asset metadata tables in ContentStore (assets, asset_variants)
- ContentStore methods (StoreAsset, GetAsset, ListAssets, etc.)
- REST API endpoints for asset CRUD and SAS URL generation
- `model.Media` extensions (BlobKey, Filename, Size)

### Phase 2: Sync Protocol
- Config schema additions (assets flag, exclude patterns, max_size)
- Sync cache extension (per-file asset hashes)
- Push algorithm: extract → dedup → upload via SAS → register metadata
- Pull algorithm: download variants → embed in localized output
- REST client methods for asset operations
- Change log integration for asset events

### Phase 3: Format Extraction
- OpenXML reader: `ExtractMedia` config → PartMedia emission
- OpenXML writer: locale-variant image replacement during reconstruction
- Block-asset dependency tracking (block_asset_refs table)

### Phase 4: Server-Side Processing
- Asset processing queue via EventBus
- OCR tool for text extraction from images (AI vision model)
- Processing status tracking and block linkage
- Server-side flow integration

## Alternatives Considered

- **Store binaries in the relational database (BLOB columns)**: Simple but doesn't scale. PostgreSQL TOAST handles large objects poorly under concurrent access. Azure Blob Storage is purpose-built for binary content, with CDN integration, tiering, and direct client access via SAS tokens.

- **Store binaries inline in the sync protocol (base64 in JSON)**: Increases sync payload by 33% (base64 overhead), doesn't support streaming, can't leverage CDN or direct upload. Pre-signed URLs decouple the control plane (metadata in REST API) from the data plane (binaries in blob storage).

- **Single BlobStore implementation (Azure only)**: Would make local development require Azure credentials or emulator. The local filesystem adapter is trivial and enables `bowrain serve` to work out of the box.

- **Embed asset binaries in model.Media.Data during sync**: Works for small assets but fails for large files. The 3-tier priority (`BlobKey` > `URI` > `Data`) allows small assets (icons, thumbnails) to flow inline through the pipeline while large assets (high-res images, video) use blob storage.

- **Separate Asset Store interface (not extending ContentStore)**: Would create a parallel persistence path with its own project/stream scoping, duplicating logic. Assets are project-scoped, stream-scoped content — they belong in ContentStore alongside blocks and items.

- **Generic cloud storage abstraction (S3 + GCS + Azure)**: Over-engineering for the current need. Azure is the deployment target. The `BlobStore` interface is cloud-agnostic, so S3/GCS adapters can be added later without changing any consumer code.

## Consequences

- **Bowrain becomes asset-aware without becoming a DAM.** It tracks locale variants, metadata, and dependencies. It does not provide DAM features (rights management, brand portals, creative workflows).

- **The sync protocol gains a binary data plane.** Blocks continue to flow as JSON through REST endpoints. Binaries flow through blob storage via SAS URLs. The two planes are coordinated by asset metadata in the ContentStore.

- **Format readers can progressively adopt PartMedia.** Formats that don't emit PartMedia continue working unchanged. The Span sentinel approach remains valid for simple image placeholders. PartMedia adds richer metadata and enables server-side processing.

- **Server-side processing enables capabilities the CLI can't provide.** OCR, ASR, and AI-powered image adaptation require GPU resources and large models. The server can run these as background flows, with results appearing in the regular sync cycle.

- **Azure Blob Storage is the first-class binary backend.** The `BlobStore` interface is implementation-agnostic, but Azure is the production target. Local filesystem serves development and testing.

- **Asset sync is opt-out, not opt-in.** Assets sync by default unless excluded in config. This ensures nothing is silently lost when users push binary-rich documents. The `assets: false` config flag and exclude patterns provide control.

- **Content-addressed deduplication applies to binaries.** The same company logo embedded in 50 DOCX files is stored once in blob storage. SHA-256 keys make dedup automatic and reliable.

- **The change log unifies block and asset sync.** A single cursor tracks both text and binary changes. `bowrain pull` receives blocks and asset variants through the same incremental mechanism — no separate sync state to manage.
