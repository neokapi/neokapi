---
id: 003-content-store
sidebar_position: 3
title: "AD-003: Content Store and Versioning"
---
# AD-003: Content Store and Versioning

## Context

The Bowrain Server needs a persistence layer that tracks translation projects, content-addressed blocks, version history, and metadata. A streaming pipeline alone (read, process, write) loses state between runs — there is no history, no diffing, and no way to perform incremental updates. Each extraction cycle would start from scratch, reprocessing content that has not changed.

The ContentStore is the **server-side persistence layer** that makes Bowrain a platform rather than just a pipeline. It manages projects, content-addressed blocks, and version history for the multi-user, multi-workspace server environment.

**This is distinct from Kapi's project model** ([AD-016](./016-kapi-project-model.md)). Kapi operates on local files with `.kapi/` project directories. The ContentStore is the backend database for Bowrain Server.

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
- **Project CRUD** — Multi-tenant project management within workspaces ([AD-015](./015-auth-and-workspaces.md))
- **Content-addressed storage** — Deduplicated block storage by content hash
- **Version tracking** — Immutable snapshots of project state
- **KAZ export/import** — Portable snapshots for backup and sharing

### Content-Addressed Block Storage

Blocks are stored by their content hash, derived from `BlockIdentity` as defined in [AD-002](./002-content-model.md). Same content produces the same hash, which is stored once. This provides:

- **Deduplication**: "Click OK" appearing 50 times across documents is stored once. Translation effort and TM lookups happen once per unique block.
- **Diffing**: Compare versions by diffing hash sets. No need to re-parse documents to determine what changed.
- **Incremental sync**: Only transfer blocks whose hashes differ between client and server. Kapi `pull/push` skips unchanged content entirely ([AD-016](./016-kapi-project-model.md)).

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

The `ContentRef` links each block back to its origin — a CMS entry, a file path, a resource key. This enables round-tripping: translations flow back to the source system via the connector that extracted the content ([AD-005](./005-connector-system.md)).

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

The default backend uses SQLite via `modernc.org/sqlite` (pure Go, no CGO), sharing the `bowrain/storage/` infrastructure layer with the Sievepen TM system and TermBase. The schema includes tables for projects, blocks, versions, and a project-blocks join table for content-addressed deduplication across projects.

See [Content Store Schema](/docs/notes/content-store-schema) for the full SQL CREATE TABLE statements and migration details.

### KAZ as Portable Snapshot Format

KAZ archives (ZIP) provide portable project snapshots for backup, sharing, migration, and offline editing. KAZ is server-side only -- Kapi uses REST API sync, not KAZ files.

### Server API Integration

The ContentStore is exposed via Bowrain Server's REST API with routes for project CRUD, block operations, sync (pull/push), and KAZ export/import.

See [Content Store Schema](/docs/notes/content-store-schema) for the KAZ archive layout, REST API route listing, and database-level details.

### Store and Pipeline Integration

The ContentStore connects to the rest of the architecture at well-defined boundaries:

```
Source System (CMS, Design Tool, Code Repo)
     |
     v
 Connector (AD-005) -- extracts content
     |
     v
 ContentStore -- persists blocks, tracks versions
     |
     v
 Flow (AD-004) -- processes blocks through tools
     |
     v
 ContentStore -- stores translated blocks, creates version
     |
     v
 Connector (AD-005) -- writes back to source system
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

- **Directory-based projects** (no database): Harder to query and version. The ContentStore provides SQL queryability, atomic transactions, and content-addressed deduplication. File-based projects are Kapi's domain ([AD-016](./016-kapi-project-model.md)), not the server's.

- **Shared store between Kapi and server**: Kapi operates on local files, not a local ContentStore. Introducing a local database for Kapi would add unnecessary complexity. The `.kapi/.sync-cache` file provides lightweight sync tracking (block hashes + cursor) without requiring SQLite.

## Consequences

- ContentStore is **Bowrain Server only** — it powers the multi-user, multi-workspace platform backend.

- **Kapi does not use ContentStore directly.** Kapi operates on local files with `.kapi/` project directories ([AD-016](./016-kapi-project-model.md)) and syncs with the server via REST API.

- Content-addressed storage eliminates duplicate work on repeated content. Blocks that appear across documents or projects are stored and processed once.

- Version tracking enables "what changed since last sync" queries, making incremental extraction practical for large content repositories.

- The store sits between connectors and the pipeline — connectors write to the store, flows process store content. This decouples extraction from processing.

- SQLite backend works without external dependencies. TM and terminology are co-located in the same storage infrastructure, sharing connection pooling and migration tooling ([AD-009](./009-translation-memory.md), [AD-010](./010-terminology.md)).

- KAZ provides portable project snapshots for backup, sharing, and offline editing. It is a **server export format**, not a Kapi working format.

- Block-level granularity is the right level for localization — not too fine (characters or words) and not too coarse (documents or pages). This aligns with the content model's `Block` as the fundamental translatable unit ([AD-002](./002-content-model.md)).

- Projects are scoped to workspaces ([AD-015](./015-auth-and-workspaces.md)). The `workspace_id` column links each project to its owning workspace.

- The ContentStore interface abstracts the storage layer — future backends (PostgreSQL, cloud object storage) require only a new implementation.

- Kapi and Bowrain Server share the same block hashing algorithm (`BlockIdentity`), enabling efficient sync without re-parsing or re-processing unchanged content.
