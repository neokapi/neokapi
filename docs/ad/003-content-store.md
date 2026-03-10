---
id: 003-content-store
sidebar_position: 3
title: "AD-003: Content Store and Versioning"
---
# AD-003: Content Store and Versioning

## Context

The Bowrain Server needs a persistence layer that tracks translation projects, content-addressed blocks, version history, and metadata. A streaming pipeline alone (read, process, write) loses state between runs — there is no history, no diffing, and no way to perform incremental updates. Each extraction cycle would start from scratch, reprocessing content that has not changed.

The ContentStore is the **server-side persistence layer** that makes Bowrain a platform rather than just a pipeline. It manages projects, content-addressed blocks, and version history for the multi-user, multi-workspace server environment.

**This is distinct from the Bowrain CLI project model** ([AD-016](./016-kapi-project-model.md)). Bowrain CLI operates on local files with `.bowrain/` project directories. The ContentStore is the backend database for Bowrain Server.

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

    // Lifecycle
    Close() error
}
```

**Key responsibilities:**
- **Project CRUD** — Multi-tenant project management within workspaces ([AD-015](./015-auth-and-workspaces.md))
- **Content-addressed storage** — Deduplicated block storage by content hash
- **Version tracking** — Immutable snapshots of project state

### Content-Addressed Block Storage

Blocks are stored by their content hash, derived from `BlockIdentity` as defined in [AD-002](./002-content-model.md). Same content produces the same hash, which is stored once. This provides:

- **Deduplication**: "Click OK" appearing 50 times across documents is stored once. Translation effort and TM lookups happen once per unique block.
- **Diffing**: Compare versions by diffing hash sets. No need to re-parse documents to determine what changed.
- **Incremental sync**: Only transfer blocks whose hashes differ between client and server. `bowrain pull/push` skips unchanged content entirely ([AD-016](./016-kapi-project-model.md)).

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

### Storage Backends

Two storage backends are available, selected by the `DATABASE_URL` connection string:

**SQLite** (default) uses `modernc.org/sqlite` (pure Go, no CGO), sharing the `bowrain/storage/` infrastructure layer with the Sievepen TM system and TermBase. The schema includes tables for projects, blocks, versions, and a project-blocks join table for content-addressed deduplication across projects. SQLite is the default for local development, single-instance deployments, and the desktop app.

**PostgreSQL** uses `pgx` (pure Go) with connection pooling. All modules (ContentStore, AuthStore, TM, TermBase, JobStore, QuotaStore) share a single connection pool, with each module managing its own schema namespace via independent migration tables. PostgreSQL supports Azure Managed Identity authentication for passwordless production deployments — the server acquires Entra ID tokens automatically when `DATABASE_AUTH=azure` is set.

Both backends implement the same `ContentStore` interface. The `Rebind()` function converts SQLite's `?` placeholders to PostgreSQL's `$N` placeholders, allowing shared query logic where possible. Each backend has independent migration sequences: SQLite uses incremental migrations (one per schema change), while PostgreSQL uses consolidated migrations (all SQLite migrations merged into a single CREATE statement per module).

The server selects the backend at startup based on the connection string prefix (`postgres://` or `postgresql://` for PostgreSQL, anything else for SQLite). All six persistence modules (content, auth, TM, termbase, jobs, quotas) use the same backend within a deployment.

See [Content Store Schema](/docs/notes/content-store-schema) for the full SQL CREATE TABLE statements and migration details.

### Server API Integration

The ContentStore is exposed via Bowrain Server's REST API with routes for project CRUD, block operations, and sync (pull/push).

See [Content Store Schema](/docs/notes/content-store-schema) for the REST API route listing and database-level details.

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

**For Bowrain CLI:**
- Bowrain CLI is the **file connector** for Bowrain Server
- `bowrain push` reads local files → sends blocks to ContentStore (via API)
- `bowrain pull` fetches blocks from ContentStore (via API) → writes local files
- Bowrain CLI does not access the ContentStore directly — it is a REST API client

## Alternatives Considered

- **Git for versioning**: Too granular — line-level diffs are not meaningful for localization where the unit of work is the translatable block. Git also adds complexity for non-technical users.

- **PostgreSQL only**: Requires an external service; SQLite provides zero-deployment overhead for local, single-instance, and desktop use. Both backends are now production-ready, with PostgreSQL recommended for SaaS and multi-instance deployments.

- **XLIFF as container**: XLIFF is an interchange format for translatable content, not a project container. It has no support for bundling previews, original source files, or project-level metadata alongside translation data.

- **Directory-based projects** (no database): Harder to query and version. The ContentStore provides SQL queryability, atomic transactions, and content-addressed deduplication. File-based projects are the Bowrain CLI's domain ([AD-016](./016-kapi-project-model.md)), not the server's.

- **Shared store between CLI and server**: Bowrain CLI operates on local files, not a local ContentStore. Introducing a local database would add unnecessary complexity. The `.bowrain/.sync-cache` file provides lightweight sync tracking (block hashes + cursor) without requiring SQLite.

## Consequences

- ContentStore is **Bowrain Server only** — it powers the multi-user, multi-workspace platform backend.

- **Bowrain CLI does not use ContentStore directly.** It operates on local files with `.bowrain/` project directories ([AD-016](./016-kapi-project-model.md)) and syncs with the server via REST API.

- Content-addressed storage eliminates duplicate work on repeated content. Blocks that appear across documents or projects are stored and processed once.

- Version tracking enables "what changed since last sync" queries, making incremental extraction practical for large content repositories.

- The store sits between connectors and the pipeline — connectors write to the store, flows process store content. This decouples extraction from processing.

- SQLite backend works without external dependencies. PostgreSQL backend supports production SaaS deployments with connection pooling and Azure Managed Identity. TM, terminology, jobs, and quotas are co-located in the same storage infrastructure, sharing the connection pool and migration tooling ([AD-009](./009-translation-memory.md), [AD-010](./010-terminology.md)).

- Block-level granularity is the right level for localization — not too fine (characters or words) and not too coarse (documents or pages). This aligns with the content model's `Block` as the fundamental translatable unit ([AD-002](./002-content-model.md)).

- Projects are scoped to workspaces ([AD-015](./015-auth-and-workspaces.md)). The `workspace_id` column links each project to its owning workspace.

- The ContentStore interface abstracts the storage layer — SQLite and PostgreSQL are both production-ready, and additional backends require only a new implementation.

- Bowrain CLI and Bowrain Server share the same block hashing algorithm (`BlockIdentity`), enabling efficient sync without re-parsing or re-processing unchanged content.
