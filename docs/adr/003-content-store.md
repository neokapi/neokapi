---
id: 003-content-store
sidebar_position: 3
title: "ADR-003: Content Store and Versioning"
---
# ADR-003: Content Store and Versioning

## Context

The Bowrain Server needs a persistence layer that tracks translation projects, content-addressed blocks, version history, and metadata. A streaming pipeline alone (read, process, write) loses state between runs — there is no history, no diffing, and no way to perform incremental updates. Each extraction cycle would start from scratch, reprocessing content that has not changed.

The ContentStore is the **server-side persistence layer** that makes Bowrain a platform rather than just a pipeline. It manages projects, content-addressed blocks, and version history for the multi-user, multi-workspace server environment.

**This is distinct from Kapi's project model** ([ADR-016](./016-kapi-project-model.md)). Kapi operates on local files with `.kapi/` project directories. The ContentStore is the backend database for Bowrain Server.

Versioned, content-addressable storage is a well-established pattern in other domains (git for source code, package registries for artifacts). The same pattern applies to localization content — blocks are the objects, projects are the streams, and versions are the commits.

## Decision

### ContentStore Interface

The ContentStore is the server-side persistence layer behind Bowrain Server's REST and gRPC APIs:

```go
type ContentStore interface {
    // Project management
    CreateProject(ctx context.Context, project Project) error
    GetProject(ctx context.Context, id string) (*Project, error)
    ListProjects(ctx context.Context) ([]Project, error)
    UpdateProject(ctx context.Context, project Project) error
    DeleteProject(ctx context.Context, id string) error

    // Content operations (content-addressed blocks)
    StoreBlocks(ctx context.Context, projectID string, blocks []*model.Block) error
    GetBlock(ctx context.Context, projectID, blockHash string) (*model.Block, error)
    GetBlocks(ctx context.Context, projectID string, opts BlockQuery) ([]*model.Block, error)
    DeleteBlock(ctx context.Context, projectID, blockHash string) error

    // Version management
    CreateVersion(ctx context.Context, projectID string, label string, description string) (*Version, error)
    GetVersion(ctx context.Context, projectID string, versionID string) (*Version, error)
    ListVersions(ctx context.Context, projectID string) ([]Version, error)
    Diff(ctx context.Context, projectID string, fromVersion, toVersion string) (*VersionDiff, error)

    // Export/Import (KAZ snapshots)
    ExportKAZ(ctx context.Context, projectID string, w io.Writer) error
    ImportKAZ(ctx context.Context, r io.Reader) (string, error)

    // Lifecycle
    Close() error
}
```

**Key responsibilities:**
- **Project CRUD** — Multi-tenant project management within workspaces ([ADR-015](./015-auth-and-workspaces.md))
- **Content-addressed storage** — Deduplicated block storage by content hash
- **Version tracking** — Immutable snapshots of project state
- **KAZ export/import** — Portable snapshots for backup and sharing

### Content-Addressed Block Storage

Blocks are stored by their content hash, derived from `BlockIdentity` as defined in [ADR-002](./002-content-model.md). Same content produces the same hash, which is stored once. This provides:

- **Deduplication**: "Click OK" appearing 50 times across documents is stored once. Translation effort and TM lookups happen once per unique block.
- **Diffing**: Compare versions by diffing hash sets. No need to re-parse documents to determine what changed.
- **Incremental sync**: Only transfer blocks whose hashes differ between client and server. Kapi `pull/push` skips unchanged content entirely ([ADR-016](./016-kapi-project-model.md)).

```go
type StoredBlock struct {
    ContentHash string                          // primary key, from BlockIdentity
    ContextHash string                          // from BlockIdentity
    Source      *model.Fragment                 // serialized source content
    Targets     map[model.LocaleID]*model.Fragment
    Annotations map[string]model.Annotation
    Properties  map[string]any
    ContentRef  *model.ContentRef               // link back to source system
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

The `ContentRef` links each block back to its origin — a CMS entry, a file path, a resource key. This enables round-tripping: translations flow back to the source system via the connector that extracted the content ([ADR-005](./005-connector-system.md)).

### Version Tracking

Each version is a snapshot of the project state — the set of block hashes, metadata, and locales at a point in time. Versions are immutable once created.

```go
type Version struct {
    ID        string
    ProjectID string
    Label     string             // "v1.2.0", "Pre-launch", "2026-02-15"
    Message   string             // "Extracted from CMS", "French translations complete"
    BlockRefs []string           // content hashes of blocks in this version
    Locales   []model.LocaleID
    CreatedAt time.Time
    CreatedBy string
}

type VersionDiff struct {
    Added    []*model.Block      // blocks in `to` but not in `from`
    Removed  []*model.Block      // blocks in `from` but not in `to`
    Modified []BlockChange       // blocks with same context hash but different content hash
}
```

The `Diff` operation compares two versions by their block hash sets. Blocks with the same `ContextHash` but different `ContentHash` appear as modifications — the content at that position in the document changed. This is more meaningful than line-level diffs for localization, where the unit of work is the translatable segment.

### SQLite Backend

The default backend uses SQLite via `modernc.org/sqlite` (pure Go, no CGO). This shares the `internal/storage/` infrastructure layer with the Sievepen TM system ([ADR-009](./009-translation-memory.md)) and the TermBase ([ADR-010](./010-terminology.md)).

Schema:

```sql
CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    source_locale TEXT NOT NULL,
    target_locales TEXT NOT NULL,    -- JSON array of BCP-47 tags
    workspace_id TEXT NOT NULL DEFAULT '',  -- FK to workspaces (ADR-015)
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_projects_workspace ON projects(workspace_id);

CREATE TABLE blocks (
    content_hash TEXT PRIMARY KEY,
    context_hash TEXT NOT NULL,
    source TEXT NOT NULL,            -- serialized Fragment JSON
    targets TEXT,                    -- JSON map: locale -> Fragment
    annotations TEXT,                -- JSON
    properties TEXT,                 -- JSON
    content_ref TEXT,                -- JSON ContentRef
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE versions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    label TEXT,
    message TEXT,
    block_refs TEXT NOT NULL,        -- JSON array of content hashes
    locales TEXT,                    -- JSON array of BCP-47 tags
    created_at TEXT NOT NULL,
    created_by TEXT
);

CREATE TABLE project_blocks (
    project_id TEXT NOT NULL REFERENCES projects(id),
    content_hash TEXT NOT NULL REFERENCES blocks(content_hash),
    PRIMARY KEY (project_id, content_hash)
);
```

The `project_blocks` join table associates blocks with projects. A single block can belong to multiple projects if the same content appears in different contexts. The `blocks` table is global — content-addressed storage means identical blocks are never duplicated regardless of which project they belong to.

SQLite provides zero-deployment overhead for local development and single-instance deployments. A future PostgreSQL backend can serve the multi-instance team server scenario where multiple server replicas share a central database.

### KAZ as Portable Snapshot Format

The `.kaz` archive format (ZIP) is a portable serialization of a ContentStore project snapshot:

```
project.kaz (ZIP)
+-- manifest.yaml           # project metadata, source/target locales
+-- blocks/
|   +-- <item>.json         # block index per source item
+-- preview/
|   +-- <item>.html         # HTML preview for editor display
+-- items/
|   +-- <file>              # original source files (optional)
+-- tm/
|   +-- entries.json        # embedded TM entries
+-- terms/
|   +-- concepts.json       # terminology snapshot
+-- version.json            # version metadata
```

**KAZ is server-side only.** It is an export format from the ContentStore, used for:
- **Backup**: Export project state to archival storage
- **Sharing**: Send snapshots to external translators or partners
- **Migration**: Import into a different Bowrain Server instance
- **Offline editing**: Bowrain desktop app can open KAZ files directly

**Kapi does not use KAZ files.** Kapi operates on local file systems with `.kapi/` project directories ([ADR-016](./016-kapi-project-model.md)). When syncing with Bowrain Server, Kapi uses the REST API to pull/push blocks incrementally — no KAZ archive is created.

### Server API Integration

The ContentStore is exposed via Bowrain Server's REST API ([ADR-013](./013-cli-and-server.md)):

```
# Project operations
POST   /api/v1/workspaces/:ws/projects
GET    /api/v1/workspaces/:ws/projects
GET    /api/v1/workspaces/:ws/projects/:id
PUT    /api/v1/workspaces/:ws/projects/:id
DELETE /api/v1/workspaces/:ws/projects/:id

# Block operations
GET    /api/v1/workspaces/:ws/projects/:id/blocks
PUT    /api/v1/workspaces/:ws/projects/:id/blocks/:hash/translation

# Sync operations (for Kapi pull/push)
GET    /api/v1/workspaces/:ws/projects/:id/sync-state
POST   /api/v1/workspaces/:ws/projects/:id/pull
POST   /api/v1/workspaces/:ws/projects/:id/push

# KAZ export/import
GET    /api/v1/workspaces/:ws/projects/:id/export
POST   /api/v1/workspaces/:ws/projects/import
```

**Kapi interacts with the store via API**, not directly. The `kapi pull/push` commands call the sync endpoints, which query the ContentStore, compute diffs, and return only changed blocks ([ADR-016](./016-kapi-project-model.md)).

### Store and Pipeline Integration

The ContentStore connects to the rest of the architecture at well-defined boundaries:

```
Source System (CMS, Design Tool, Code Repo)
     |
     v
 Connector (ADR-005) -- extracts content
     |
     v
 ContentStore -- persists blocks, tracks versions
     |
     v
 Flow (ADR-004) -- processes blocks through tools
     |
     v
 ContentStore -- stores translated blocks, creates version
     |
     v
 Connector (ADR-005) -- writes back to source system
```

**For Bowrain Server:**
- Connectors (CMS, design, code) pull content → store in ContentStore
- Flows process store content through tools
- Connectors push translations back to source systems

**For Kapi:**
- Kapi is the **file connector** for Bowrain Server
- `kapi push` reads local files → sends blocks to ContentStore (via API)
- `kapi pull` fetches blocks from ContentStore (via API) → writes local files
- Kapi does not access the ContentStore directly — it is a REST API client

## Alternatives Considered

- **Git for versioning**: Too granular — line-level diffs are not meaningful for localization where the unit of work is the translatable block. Git also adds complexity for non-technical users.

- **PostgreSQL as default**: Requires an external service; SQLite provides zero-deployment overhead for local and single-instance use. PostgreSQL remains an option for multi-instance deployments.

- **XLIFF as container**: XLIFF is an interchange format for translatable content, not a project container. It has no support for bundling previews, original source files, or project-level metadata alongside translation data.

- **Directory-based projects** (no database): Harder to query and version. The ContentStore provides SQL queryability, atomic transactions, and content-addressed deduplication. File-based projects are Kapi's domain ([ADR-016](./016-kapi-project-model.md)), not the server's.

- **Shared store between Kapi and server**: Kapi operates on local files, not a local ContentStore. Introducing a local database for Kapi would add unnecessary complexity. The `.kapi/.sync-cache` file provides lightweight sync tracking (block hashes + cursor) without requiring SQLite.

## Consequences

- ContentStore is **Bowrain Server only** — it powers the multi-user, multi-workspace platform backend.

- **Kapi does not use ContentStore directly.** Kapi operates on local files with `.kapi/` project directories ([ADR-016](./016-kapi-project-model.md)) and syncs with the server via REST API.

- Content-addressed storage eliminates duplicate work on repeated content. Blocks that appear across documents or projects are stored and processed once.

- Version tracking enables "what changed since last sync" queries, making incremental extraction practical for large content repositories.

- The store sits between connectors and the pipeline — connectors write to the store, flows process store content. This decouples extraction from processing.

- SQLite backend works without external dependencies. TM and terminology are co-located in the same storage infrastructure, sharing connection pooling and migration tooling ([ADR-009](./009-translation-memory.md), [ADR-010](./010-terminology.md)).

- KAZ provides portable project snapshots for backup, sharing, and offline editing. It is a **server export format**, not a Kapi working format.

- Block-level granularity is the right level for localization — not too fine (characters or words) and not too coarse (documents or pages). This aligns with the content model's `Block` as the fundamental translatable unit ([ADR-002](./002-content-model.md)).

- Projects are scoped to workspaces ([ADR-015](./015-auth-and-workspaces.md)). The `workspace_id` column links each project to its owning workspace.

- The ContentStore interface abstracts the storage layer — future backends (PostgreSQL, cloud object storage) require only a new implementation.

- Kapi and Bowrain Server share the same block hashing algorithm (`BlockIdentity`), enabling efficient sync without re-parsing or re-processing unchanged content.
