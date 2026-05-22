---
id: 004-content-store
sidebar_position: 4
title: "AD-004: Content Store and Versioning"
---

# AD-004: Content Store and Versioning

## Summary

The ContentStore is Bowrain's server-side persistence layer. It tracks
projects, items, content-addressed blocks with per-locale target segments,
version snapshots, streams, an append-only change log, and asset metadata.
PostgreSQL is the production backend; SQLite serves single-instance and
self-hosted deployments. All access is mediated by the Bowrain Server REST
API — the bowrain CLI never touches the store directly.

## Context

The framework's streaming pipeline processes a document from read to write
without retaining state. That is the right shape for a CLI tool, but a
multi-user platform needs to know what blocks exist, which translations are
current, what changed since the last sync, and what a past version looked
like.

Content-addressable storage is the well-established pattern for this:
blocks are objects, projects are streams of state, and versions are named
points along the stream. Git works this way for source code; a localization
platform benefits from the same pattern one granularity finer — at the
translatable block level rather than the line or file level.

The ContentStore is the persistence substrate the rest of the platform is
built on: connectors write to it, flows process against it, the editor reads
from it, and the sync protocol exposes it to remote clients.

## Decision

### ContentStore Interface

The `ContentStore` interface in `bowrain/core/store/` defines the persistence
contract. Implementations live in `bowrain/store/` (SQLite and PostgreSQL):

```go
type ContentStore interface {
    // Project management
    CreateProject(ctx context.Context, project Project) error
    GetProject(ctx context.Context, id string) (*Project, error)
    ListProjects(ctx context.Context, workspaceID string) ([]Project, error)
    UpdateProject(ctx context.Context, project Project) error
    DeleteProject(ctx context.Context, id string) error

    // Content-addressed block operations
    StoreBlocks(ctx context.Context, projectID string, blocks []*model.Block) error
    GetBlock(ctx context.Context, projectID, blockID string) (*model.Block, error)
    GetBlocks(ctx context.Context, projectID string, opts BlockQuery) ([]*model.Block, error)
    DeleteBlock(ctx context.Context, projectID, blockID string) error

    // Versions
    CreateVersion(ctx context.Context, projectID, label, description string) (*Version, error)
    GetVersion(ctx context.Context, projectID, versionID string) (*Version, error)
    ListVersions(ctx context.Context, projectID string) ([]Version, error)
    Diff(ctx context.Context, projectID, fromVersion, toVersion string) (*VersionDiff, error)

    // Assets (see AD-007)
    StoreAsset(ctx context.Context, projectID, stream string, asset *Asset) error
    GetAsset(ctx context.Context, projectID, stream, assetID string) (*Asset, error)
    // ...

    Close() error
}
```

The interface lives in `bowrain/core/store/` so that packages depending on
shared types don't need to import the heavy `bowrain/store/` implementation.

### Content-Addressed Block Storage

Blocks are keyed by content hash, computed from `BlockIdentity` in the
framework content model — see
[AD-framework-002: Content Model](https://neokapi.github.io/web/neokapi/docs/architecture/002-content-model).
Identical content produces the same hash and is stored once.

This gives:

- **Deduplication.** "Click OK" appearing 50 times across documents costs
  one row. TM lookups happen once per unique block. Memory usage scales
  with unique content, not total occurrences.
- **Diffing.** Two versions differ exactly where their hash sets differ;
  re-parsing documents is unnecessary.
- **Incremental sync.** Only blocks whose hashes differ between client and
  server are transferred. See [AD-009: Sync Protocol](009-sync-protocol.md).

```go
type StoredBlock struct {
    ContentHash string                          // from BlockIdentity, primary identifier
    ContextHash string                          // from BlockIdentity
    Source      *model.Fragment
    Targets     map[model.LocaleID]*model.Fragment
    Annotations map[string]model.Annotation
    Properties  map[string]any
    ContentRef  *model.ContentRef               // link back to the source system
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

The `ContentRef` links each block back to its origin — a CMS entry, a file
path, a resource key — enabling round-trip back to the source system via
the connector that extracted the content ([AD-008: Connector System](008-connector-system.md)).

### Version Tracking

A version is an immutable snapshot of a project: the set of block hashes,
metadata, and locales at a point in time.

```go
type Version struct {
    ID        string
    ProjectID string
    Label     string              // "v1.2.0", "Pre-launch", "2026-02-15"
    Message   string
    BlockRefs []string            // content hashes in this version
    Locales   []model.LocaleID
    CreatedAt time.Time
    CreatedBy string
}

type VersionDiff struct {
    Added    []*model.Block       // blocks in `to` but not in `from`
    Removed  []*model.Block       // blocks in `from` but not in `to`
    Modified []BlockChange        // blocks with same context hash but different content hash
}
```

`Diff` compares two versions by hash sets. Blocks with the same
`ContextHash` but different `ContentHash` appear as modifications — the
content at that position changed. This produces a semantically meaningful
diff for localization, which cares about translatable units rather than
line-level changes.

### Schema

The full schema covers projects, items (source documents), blocks (content
hash per block, with source and target segments), versions, stream
metadata, an append-only change log, assets, asset variants, and
block-asset references.

Sketch (SQLite syntax for readability; PostgreSQL uses equivalent types):

```sql
CREATE TABLE projects (
    id             TEXT PRIMARY KEY,
    workspace_id   TEXT NOT NULL,
    name           TEXT NOT NULL,
    source_locale  TEXT NOT NULL,
    target_locales TEXT NOT NULL DEFAULT '',   -- JSON array
    properties     TEXT NOT NULL DEFAULT '{}',
    created_at     TIMESTAMP NOT NULL,
    updated_at     TIMESTAMP NOT NULL
);

CREATE TABLE items (
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    format       TEXT NOT NULL DEFAULT '',
    item_type    TEXT NOT NULL DEFAULT 'file',
    source_bytes BLOB,
    block_index  TEXT NOT NULL DEFAULT '{}',
    properties   TEXT NOT NULL DEFAULT '{}',
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL,
    PRIMARY KEY (project_id, name)
);

-- Blocks hold source content + project metadata only. Targets and
-- annotations live in their own kind-specific tables (see below)
-- so per-locale editing, QA-finding feeds, and automation-run logs
-- each get the indexes their access patterns need.
CREATE TABLE blocks (
    id           TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    item_name    TEXT NOT NULL DEFAULT '',
    source_id    TEXT NOT NULL DEFAULT '',    -- format-reader-assigned ID
    name         TEXT NOT NULL DEFAULT '',
    type         TEXT NOT NULL DEFAULT '',
    mime_type    TEXT NOT NULL DEFAULT '',
    translatable INTEGER NOT NULL DEFAULT 1,
    content_hash TEXT NOT NULL DEFAULT '',
    context_hash TEXT NOT NULL DEFAULT '',
    source_json  TEXT NOT NULL DEFAULT '[]',  -- serialized Fragment JSON
    properties   TEXT NOT NULL DEFAULT '{}',
    stored_at    TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL,
    PRIMARY KEY (project_id, id)
);

-- Per-locale translation targets. Hot read path: dashboards, editor,
-- sync export. Hash-partitioned by project_id on Postgres.
CREATE TABLE translations (
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    stream        TEXT NOT NULL DEFAULT 'main',
    block_id      TEXT NOT NULL,
    locale        TEXT NOT NULL,
    text          TEXT NOT NULL DEFAULT '',     -- flat text for simple queries
    segments_json TEXT NOT NULL DEFAULT '[]',   -- rich []*Segment round-trip
    provider      TEXT NOT NULL DEFAULT '',     -- source attribution (ai/human/webhook:deepl)
    metadata      TEXT NOT NULL DEFAULT '{}',
    updated_at    TIMESTAMP NOT NULL,
    PRIMARY KEY (project_id, stream, block_id, locale)
);

-- Semantic annotations (TM hits, term hits, QA findings, translator notes).
-- Access pattern: "all QA findings for this project, newest first".
CREATE TABLE annotations (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    stream     TEXT NOT NULL DEFAULT 'main',
    block_id   TEXT NOT NULL,
    kind       TEXT NOT NULL,
    payload    TEXT NOT NULL DEFAULT '{}',
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (project_id, stream, block_id, kind)
);

-- Plugin catchall for overlay kinds that don't fit the purpose-built
-- tables above (skeletons, custom plugin outputs, etc.).
CREATE TABLE overlays_ext (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    stream     TEXT NOT NULL DEFAULT 'main',
    block_id   TEXT NOT NULL,
    kind       TEXT NOT NULL,
    payload    TEXT NOT NULL DEFAULT '{}',
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (project_id, stream, block_id, kind)
);

CREATE TABLE versions (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    label       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    block_count INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMP NOT NULL
);

CREATE TABLE version_blocks (
    version_id   TEXT NOT NULL REFERENCES versions(id) ON DELETE CASCADE,
    block_id     TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    PRIMARY KEY (version_id, block_id)
);

CREATE TABLE change_log (
    seq          BIGSERIAL PRIMARY KEY,
    project_id   TEXT NOT NULL,
    block_id     TEXT NOT NULL,
    change_type  TEXT NOT NULL,           -- source_added, source_modified, target_added, ...
    stream       TEXT NOT NULL DEFAULT 'main',
    locale       TEXT,
    content_hash TEXT,
    logged_at    TIMESTAMP NOT NULL
);
CREATE INDEX idx_changelog_project_seq ON change_log(project_id, seq);
CREATE INDEX idx_changelog_stream ON change_log(project_id, stream, seq);
```

Blocks are scoped to projects with a composite primary key `(project_id,
id)`. The `source_id` column tracks the format-reader-assigned ID (e.g.
`tu1` from PO), with a unique index ensuring no duplicates within an item.
The `version_blocks` join associates blocks with version snapshots.

Targets and annotations sit in their own kind-specific tables. The
`blockstore.Store` adapter (`bowrain/store/blockstore/`) dispatches by
kind prefix: `targets/<locale>` → `translations`, `annotations/<name>`
→ `annotations`, everything else → `overlays_ext`. Callers of the
`ContentStore.StoreBlocks` / `GetBlocks` surface don't see the split
— `StoredBlock.Targets` / `.Annotations` are populated via a batched
join after the source rows are scanned. The three overlay tables are
hash-partitioned by `project_id` (MODULUS 8) on Postgres so per-project
queries hit one partition and drop-project is partition-DROP.

The `streams` table and per-stream change log scoping are defined in
[AD-005: Streams](005-streams.md). Asset tables are defined in
[AD-007: Media and Blob Storage](007-media-and-blob-storage.md).

### Storage Backend

**PostgreSQL** via `pgx` is the production backend. All six persistence
modules — ContentStore, AuthStore, Sievepen (TM), TermBase, JobStore,
QuotaStore — share a single connection pool, each managing its own schema
namespace via an independent migration table
(`store_schema_migrations`, `auth_schema_migrations`, etc.).

PostgreSQL supports Azure Managed Identity authentication for passwordless
production deployments: the server acquires Entra ID tokens automatically
when `DATABASE_AUTH=azure` is set, making connection credentials disappear
from configuration.

**SQLite** is the backend for single-instance and self-hosted deployments
and for the desktop app's local offline cache. It shares the same schema
shape with incremental migrations (`schema_migrations`).

The server requires a `DATABASE_URL` connection string (`postgres://`,
`postgresql://`, or `sqlite://`).

### Migration Strategy

PostgreSQL uses a single consolidated migration containing the complete
schema; fresh databases start from that one definition. When deployed
databases exist, a separate migration with `ALTER TABLE IF NOT EXISTS`
guards adds new tables and columns.

SQLite uses incremental migrations. Each new table or column gets a new
version. This matches the single-machine deployment pattern where
migrations apply in place as the CLI or desktop app updates.

### Server API

The ContentStore is exposed via REST under workspace-scoped routes:

```
POST   /api/v1/workspaces/:ws/projects
GET    /api/v1/workspaces/:ws/projects
GET    /api/v1/workspaces/:ws/projects/:id
PUT    /api/v1/workspaces/:ws/projects/:id
DELETE /api/v1/workspaces/:ws/projects/:id

POST   /api/v1/workspaces/:ws/projects/:id/blocks
GET    /api/v1/workspaces/:ws/projects/:id/blocks
POST   /api/v1/workspaces/:ws/projects/:id/versions
GET    /api/v1/workspaces/:ws/projects/:id/versions

# Sync routes (see AD-009)
POST   /api/v1/workspaces/:ws/projects/:id/sync/push/init
POST   /api/v1/workspaces/:ws/projects/:id/sync/push/diff
POST   /api/v1/workspaces/:ws/projects/:id/sync/push/commit
GET    /api/v1/workspaces/:ws/projects/:id/sync/pull
GET    /api/v1/workspaces/:ws/projects/:id/changes
```

gRPC serves the desktop app via `EditorService` for real-time operations
(streaming presence, block watches). HTTP and gRPC are multiplexed on the
same port via h2c protocol detection.

### Integration Pipeline

The ContentStore sits at the center of the platform:

```
Source System (CMS, Design Tool, Code Repo, File system)
     │
     ▼
 Connector (AD-008) — extracts content
     │
     ▼
 ContentStore — persists blocks, tracks versions, appends change log
     │
     ▼
 Flow (framework) — processes blocks through tools
     │
     ▼
 ContentStore — stores translated blocks, creates version snapshot
     │
     ▼
 Connector (AD-008) — writes translations back to the source system
```

The bowrain CLI does not access the ContentStore directly. It is a REST
API client that syncs local files against the store via the sync protocol
— see [AD-009: Sync Protocol](009-sync-protocol.md). `kapi push` sends
blocks to the store; `kapi pull` fetches changes from the store.

### Deduplication Across Sources

Because block identity is `BlockIdentity`-derived and not connector- or
format-specific, identical content across projects can reference the same
block. A marketing tagline that lives in a CMS entry and a design file
produces the same block hash and, within a workspace, can share
translations and TM scoring.

### Incremental Extraction

When a connector re-runs against a source system, it computes block hashes
and only re-stores blocks whose hashes differ. The change log records only
actual changes, so downstream consumers (automation, the pull endpoint)
see exactly what moved.

## Consequences

- Identical content stored once: storage scales with unique content, not
  total occurrences.
- Version diffing is a hash set operation, not a document re-parse.
- PostgreSQL is the production backend with connection pooling, concurrent
  writes, and Azure Managed Identity support. SQLite serves single-instance
  and self-hosted.
- TM, terminology, auth, jobs, and quotas are co-located in the same
  storage infrastructure and share the connection pool.
- Block-level granularity is the right level for localization — not too
  fine (characters or words) and not too coarse (documents or pages).
- Every project is workspace-scoped; every block is project-scoped.
- The ContentStore interface is stable; additional backends require only a
  new implementation.
- The bowrain CLI is a thin REST client and carries none of the
  persistence complexity.

## Related

- [AD-001: Bowrain Vision and Module Architecture](001-vision-and-modules.md)
- [AD-005: Streams](005-streams.md)
- [AD-007: Media and Blob Storage](007-media-and-blob-storage.md)
- [AD-008: Connector System](008-connector-system.md)
- [AD-009: Sync Protocol](009-sync-protocol.md)
- [AD-framework-002: Content Model](https://neokapi.github.io/web/neokapi/docs/architecture/002-content-model)
