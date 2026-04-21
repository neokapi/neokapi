---
sidebar_position: 7
title: "Media Asset Storage"
---

# Media Asset Storage

This note provides implementation details for [AD-007](/bowrain/architecture-decisions/007-media-and-blob-storage).

## BlobStore Interface

Defined in `core/storage/blob.go` (framework module, zero platform dependencies):

```go
package storage

import (
    "context"
    "io"
    "time"
)

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
    ContentType string
    Filename    string
}

type SignOptions struct {
    ExpiresIn time.Duration // Default: 1 hour
}

type BlobRef struct {
    Key         string // SHA-256 hex of content
    Size        int64
    ContentType string
}
```

**Content-addressing**: The `Upload` implementation computes `sha256(data)` and uses the hex digest as the storage key. If the key already exists, Upload returns the existing `BlobRef` without writing (dedup). This mirrors the `BlockIdentity` content-addressing in the ContentStore.

## Azure Blob Storage Adapter

Located in `platform/storage/azureblob/`:

```go
package azureblob

import (
    "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
    "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

type Store struct {
    client        *azblob.Client
    containerName string
}

// New creates an Azure Blob Store using Azure Managed Identity (production).
func New(accountURL, containerName string) (*Store, error) {
    cred, err := azidentity.NewDefaultAzureCredential(nil)
    if err != nil {
        return nil, fmt.Errorf("azure identity: %w", err)
    }
    client, err := azblob.NewClient(accountURL, cred, nil)
    if err != nil {
        return nil, fmt.Errorf("azure blob client: %w", err)
    }
    return &Store{client: client, containerName: containerName}, nil
}

// NewWithConnectionString creates an Azure Blob Store for local development.
func NewWithConnectionString(connStr, containerName string) (*Store, error) {
    client, err := azblob.NewClientFromConnectionString(connStr, nil)
    if err != nil {
        return nil, fmt.Errorf("azure blob client: %w", err)
    }
    return &Store{client: client, containerName: containerName}, nil
}
```

**Blob naming convention**: `{project_id}/{key[0:2]}/{key[2:4]}/{key}` — project-scoped with git-like sharding to avoid directory listing bottlenecks.

**SAS token generation** uses User Delegation SAS (Managed Identity) in production:

```go
func (s *Store) GenerateUploadURL(ctx context.Context, key string, opts SignOptions) (string, error) {
    expiry := opts.ExpiresIn
    if expiry == 0 {
        expiry = time.Hour
    }
    // User Delegation Key for SAS without storage account key
    udk, err := s.client.ServiceClient().GetUserDelegationCredential(ctx, ...)
    sasURL, err := blobClient.GetSASURL(sas.BlobPermissions{Write: true, Create: true}, expiry, &sas.SignOptions{...})
    return sasURL, nil
}

func (s *Store) GenerateDownloadURL(ctx context.Context, key string, opts SignOptions) (string, error) {
    // Same pattern, Read permission instead of Write
    sasURL, err := blobClient.GetSASURL(sas.BlobPermissions{Read: true}, expiry, &sas.SignOptions{...})
    return sasURL, nil
}
```

**Environment variables:**

| Variable                          | Description        | Example                                   |
| --------------------------------- | ------------------ | ----------------------------------------- |
| `BLOB_STORAGE_BACKEND`            | Backend type       | `azure`, `local`                          |
| `AZURE_STORAGE_ACCOUNT_URL`       | Azure account URL  | `https://myaccount.blob.core.windows.net` |
| `AZURE_STORAGE_CONTAINER`         | Container name     | `bowrain-assets`                          |
| `AZURE_STORAGE_CONNECTION_STRING` | Dev fallback       | `DefaultEndpointsProtocol=...`            |
| `BLOB_STORAGE_LOCAL_DIR`          | Local backend root | `/var/lib/bowrain/blobs`                  |

## Local Filesystem Adapter

Located in `platform/storage/localblob/`:

```go
package localblob

type Store struct {
    rootDir string
}

func New(rootDir string) *Store
```

**Storage layout**: `{rootDir}/{key[0:2]}/{key[2:4]}/{key}` (same sharding as Azure path).

`GenerateUploadURL` and `GenerateDownloadURL` return `storage.ErrNotSupported`. When the CLI detects this error, it falls back to direct `Upload`/`Download` through the server proxy:

```
POST /api/v1/projects/:id/assets/upload     (multipart/form-data, server proxies to BlobStore)
GET  /api/v1/projects/:id/assets/:aid/download  (server streams from BlobStore)
```

These proxy endpoints are only needed for backends that don't support pre-signed URLs. The Azure adapter always uses direct SAS upload/download.

## Asset Metadata Schema

### assets table

```sql
CREATE TABLE assets (
    id                TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    item_name         TEXT NOT NULL DEFAULT '',
    source_id         TEXT NOT NULL DEFAULT '',
    blob_key          TEXT NOT NULL,
    mime_type         TEXT NOT NULL,
    filename          TEXT NOT NULL DEFAULT '',
    size_bytes        INTEGER NOT NULL DEFAULT 0,
    alt_text          TEXT NOT NULL DEFAULT '',
    properties        TEXT NOT NULL DEFAULT '{}',
    processing_status TEXT NOT NULL DEFAULT 'none',
    processing_hint   TEXT NOT NULL DEFAULT '',
    stream            TEXT NOT NULL DEFAULT 'main',
    created_at        TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at        TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_assets_project_item ON assets(project_id, item_name);
CREATE UNIQUE INDEX idx_assets_blob ON assets(project_id, blob_key)
    WHERE stream = 'main';
```

**processing_status values**: `none` (no processing needed), `pending` (queued), `processing`, `processed`, `failed`.

**processing_hint values**: `ocr` (extract text from image), `chart-text` (extract labels from chart), `subtitle-extract` (extract captions from video), `asr` (speech-to-text from audio).

### asset_variants table

```sql
CREATE TABLE asset_variants (
    asset_id    TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    locale      TEXT NOT NULL,
    blob_key    TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    mime_type   TEXT NOT NULL DEFAULT '',
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    properties  TEXT NOT NULL DEFAULT '{}',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (asset_id, locale)
);
```

**status values**: `pending` (needs localization), `draft` (variant uploaded, not reviewed), `approved` (ready for inclusion in localized output).

### block_asset_refs table

```sql
CREATE TABLE block_asset_refs (
    project_id TEXT NOT NULL,
    block_id   TEXT NOT NULL,
    asset_id   TEXT NOT NULL,
    ref_type   TEXT NOT NULL DEFAULT 'embedded',
    stream     TEXT NOT NULL DEFAULT 'main',
    PRIMARY KEY (project_id, block_id, asset_id)
);
CREATE INDEX idx_block_asset_refs_asset ON block_asset_refs(project_id, asset_id);
```

**ref_type values**: `embedded` (asset appears inline in block content), `context` (asset provides visual context for translation), `generated` (asset was generated from block text, e.g., voiceover).

## REST API Endpoints

### Asset CRUD

```
POST   /api/v1/projects/:id/assets/upload-url
  Request:  { "blob_key": "sha256hex", "content_type": "image/png", "size": 102400 }
  Response: { "upload_url": "https://...?sv=...&sig=...", "exists": false }
           or { "exists": true }  (dedup: blob already stored)

POST   /api/v1/projects/:id/assets
  Request:  { "blob_key": "sha256hex", "item_name": "docs/manual.docx",
              "source_id": "image1", "mime_type": "image/png",
              "filename": "diagram.png", "size_bytes": 102400,
              "alt_text": "Architecture diagram", "properties": {"width": "800", "height": "600"} }
  Response: { "id": "ast_xxx", "blob_key": "sha256hex", ... }

GET    /api/v1/projects/:id/assets?item_name=docs/manual.docx
  Response: { "assets": [{ "id": "ast_xxx", ..., "download_url": "https://...?sig=..." }] }

GET    /api/v1/projects/:id/assets/:aid
  Response: { "id": "ast_xxx", ..., "download_url": "https://...?sig=..." }

DELETE /api/v1/projects/:id/assets/:aid
```

### Locale Variants

```
POST   /api/v1/projects/:id/assets/:aid/variants/upload-url
  Request:  { "locale": "fr-FR", "blob_key": "sha256hex", "content_type": "image/png" }
  Response: { "upload_url": "https://...?sig=..." }

POST   /api/v1/projects/:id/assets/:aid/variants
  Request:  { "locale": "fr-FR", "blob_key": "sha256hex", "mime_type": "image/png",
              "size_bytes": 98304, "status": "draft" }
  Response: { "asset_id": "ast_xxx", "locale": "fr-FR", ... }

GET    /api/v1/projects/:id/assets/:aid/variants
  Response: { "variants": [{ "locale": "fr-FR", "status": "approved", "download_url": "..." }] }
```

### Sync Integration

Asset changes appear in the existing change log. The pull endpoint includes asset changes:

```
GET /api/v1/projects/:id/sync/pull?cursor=X&locales=fr-FR
  Response: {
    "changes": [ ... block changes ... ],
    "asset_changes": [
      { "seq": 4822, "asset_id": "ast_xxx", "change_type": "variant_added",
        "locale": "fr-FR", "blob_key": "sha256hex" }
    ],
    "new_cursor": 4825,
    "has_more": false
  }
```

## Config Schema Additions

```yaml
# config.yaml
content:
  - path: docs/**/*.docx
    format: openxml
    assets: true # default: true for formats that support PartMedia
    asset_max_size: 50MB # per-asset size limit (default: 100MB)

# Project-wide asset settings
assets:
  enabled: true # master toggle (default: true)
  exclude: # glob patterns for filenames to skip
    - "*.psd"
    - "*.ai"
    - "thumbnail_*"
  max_size: 100MB # global per-asset size limit
```

**Resolution order for asset inclusion:**

1. `assets.enabled: false` → skip all assets globally
2. `content[].assets: false` → skip assets for this content entry
3. `assets.exclude` patterns → skip matching filenames
4. `content[].asset_max_size` or `assets.max_size` → skip oversized assets

## Sync Cache Extension

```json
{
  "files": {
    "docs/manual.docx": {
      "mtime": "2026-03-15T10:25:00Z",
      "size": 2048576,
      "blocks": {
        "heading1": "a1b2c3d4..."
      },
      "assets": {
        "image1.png": {
          "blob_key": "e5f6a7b8...",
          "size": 102400,
          "mime_type": "image/png"
        }
      }
    }
  }
}
```

## OpenXML Media Extraction

Current behavior (wml.go lines 362-366, 538-549): `<w:drawing>` elements emit `\uE101` sentinel → Span with `Type=media:image`, `Data="<w:drawing/>"`.

**New behavior when `ExtractMedia=true`:**

1. Reader opens the DOCX ZIP, iterates `word/media/*` entries
2. For each media file: compute SHA-256, create `model.Media{ID, MimeType, BlobKey, Data, Filename, Size}`
3. Emit `PartMedia` before the block that references the image
4. In the referencing block's Span, set `Data` to `"ref:media:{media_id}"` (linking Span → Media)
5. Writer checks for locale-variant Media by source_id, substitutes in the output ZIP

**Relationship diagram:**

```
PartMedia (Media{ID:"img1", BlobKey:"sha256:abc...", MimeType:"image/png"})
    ↓ (emitted before)
PartBlock (Block with Span{Type:"media:image", Data:"ref:media:img1"})
    ↓ (linked via source_id)
Asset (metadata in ContentStore, binary in BlobStore)
    ↓ (has variants)
AssetVariant (locale="fr-FR", blob_key="sha256:def...", status="approved")
```
