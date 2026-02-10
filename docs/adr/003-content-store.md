---
id: 003-content-store
sidebar_position: 3
title: "ADR-003: Content Store and Versioning"
---
# ADR-003: Content Store and Versioning

## Context

Localization projects involve multiple documents, translation state across
locales, translation memory entries, terminology, and preview renderings. A
pipeline alone (read, process, write) loses state between runs -- there is no
history, no diffing, and no way to perform incremental updates. Each extraction
cycle starts from scratch, reprocessing content that has not changed.

The Okapi Framework has no persistence layer. Projects are collections of loose
files managed by external tools (Trados, memoQ, or manual directory structures).
State is implicit -- spread across XLIFF files, TM databases, and folder
conventions that vary by team.

Versioned, content-addressable storage is a well-established pattern in other
domains (git for source code, package registries for artifacts). The same
pattern applies to localization content -- blocks are the objects, projects are
the streams, and versions are the commits.

The original KAZ archive format ([ADR-011](./003-content-store.md))
provided portable project packaging as a ZIP file, but it lacked versioning,
diffing, and store semantics. KAZ is better understood as a serialization format
for a store snapshot rather than as the store itself.

## Decision

### ContentStore Interface

The content store is the persistence layer that makes gokapi a platform rather
than just a pipeline. It manages projects, content-addressed blocks, and version
history.

```go
type ContentStore interface {
    // Project management
    CreateProject(ctx context.Context, project Project) error
    GetProject(ctx context.Context, id string) (*Project, error)
    ListProjects(ctx context.Context) ([]Project, error)

    // Content operations
    StoreBlocks(ctx context.Context, projectID string, blocks []*model.Block) error
    GetBlock(ctx context.Context, projectID, blockHash string) (*model.Block, error)
    GetBlocks(ctx context.Context, projectID string, opts BlockQuery) ([]*model.Block, error)

    // Version management
    CreateVersion(ctx context.Context, projectID string, message string) (*Version, error)
    GetVersion(ctx context.Context, projectID string, versionID string) (*Version, error)
    ListVersions(ctx context.Context, projectID string) ([]Version, error)
    Diff(ctx context.Context, projectID string, fromVersion, toVersion string) (*VersionDiff, error)

    // Export/Import
    ExportKAZ(ctx context.Context, projectID string, w io.Writer) error
    ImportKAZ(ctx context.Context, r io.Reader) (string, error)
}
```

Connectors ([ADR-005](./005-connector-system.md)) write extracted content into
the store. Flows ([ADR-004](./004-processing-engine.md)) process store content
through tool chains. The store sits between these two layers, providing the
durable state that connects them.

### Content-Addressed Block Storage

Blocks are stored by their content hash, derived from `BlockIdentity` as defined
in [ADR-002](./002-content-model.md). Same content produces the same hash,
which is stored once. This provides:

- **Deduplication**: "Click OK" appearing 50 times across documents is stored
  once. Translation effort and TM lookups happen once per unique block.
- **Diffing**: Compare versions by diffing hash sets. No need to parse documents
  to determine what changed.
- **Incremental sync**: Only transfer blocks whose hashes differ between source
  system and store. Connectors skip unchanged content entirely.

```go
type StoredBlock struct {
    ContentHash string                          // primary key, from BlockIdentity
    ContextHash string                          // from BlockIdentity
    Source      *model.Fragment                  // serialized source content
    Targets     map[model.LocaleID]*model.Fragment
    Annotations map[string]model.Annotation
    Properties  map[string]any
    ContentRef  *model.ContentRef               // link back to source system
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

The `ContentRef` links each block back to its origin -- a CMS entry, a file
path, a resource key. This enables round-tripping: translations flow back to the
source system via the same connector that extracted the content.

### Version Tracking

Each version is a snapshot of the project state -- the set of block hashes,
metadata, and locales at a point in time. Versions are immutable once created.

```go
type Version struct {
    ID        string
    ProjectID string
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

The `Diff` operation compares two versions by their block hash sets. Blocks with
the same `ContextHash` but different `ContentHash` appear as modifications --
the content at that position in the document changed. This is more meaningful
than line-level diffs for localization, where the unit of work is the
translatable segment.

### Incremental Extraction

When a connector pulls content from a source system, the store enables
incremental updates:

1. Compute `BlockIdentity` hashes for newly extracted blocks
2. Compare against existing block hashes in the current version
3. Only process new or changed blocks through the pipeline
4. Create a new version capturing the delta

This reduces processing time for large projects. A CMS with 10,000 pages where
50 changed since the last sync only processes those 50 blocks through
translation, TM leverage, and QA tools.

### SQLite Backend

The default backend uses SQLite via `modernc.org/sqlite` (pure Go, no CGO).
This shares the `internal/storage/` infrastructure layer with the Sievepen TM
system ([ADR-009](./009-translation-memory.md)) and the TermBase
([ADR-010](./010-terminology.md)).

Schema sketch:

```sql
CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    source_locale TEXT NOT NULL,
    target_locales TEXT NOT NULL,    -- JSON array of BCP-47 tags
    workspace_id TEXT NOT NULL DEFAULT '',  -- FK to workspaces (ADR-015)
    created_at TEXT NOT NULL
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

The `project_blocks` join table associates blocks with projects. A single block
can belong to multiple projects if the same content appears in different
contexts. The `blocks` table is global -- content-addressed storage means
identical blocks are never duplicated regardless of which project they belong to.

SQLite provides zero-deployment overhead for local use. The Bowrain desktop app
([ADR-012](./012-bowrain.md)) opens the store directly. A future
PostgreSQL backend can serve the team server scenario where multiple users share
a central store.

### KAZ as Portable Store Serialization

The `.kaz` archive format (ZIP) is the portable serialization of a store
snapshot -- a serialization of the object graph for file-based sharing.

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

The manifest contains source and target locales as BCP-47 tags in canonical form
([ADR-001](./001-vision.md)), an item list with format and block count,
and project-level metadata. Block indices store segment-level translation state
without requiring document re-parsing. Pre-rendered HTML previews enable fast
display in the editor.

KAZ is not the store itself -- it is an export format. The store is SQLite on
disk. KAZ enables:

- **Sharing**: Send projects via email, cloud storage, or version control
- **Offline work**: Bowrain opens KAZ files directly for offline editing
- **Migration**: Import into a different store instance or gokapi installation
- **Inspection**: Standard ZIP tools can examine the contents

On import, the store computes block hashes and deduplicates against existing
content. On export, the store serializes the current version's blocks, embeds
TM entries and terminology snapshots, and generates previews.

### Store and Pipeline Integration

The store connects to the rest of the architecture at well-defined boundaries:

```
Source System
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

The flow executor reads blocks from the store, streams them through the tool
chain, and writes results back. TM leverage
([ADR-009](./009-translation-memory.md)) and terminology lookup
([ADR-010](./010-terminology.md)) are tools in the pipeline that query their
own co-located SQLite databases.

## Alternatives Considered

- **Git for versioning**: Too granular -- line-level diffs are not meaningful
  for localization where the unit of work is the translatable block. Git also
  adds complexity for non-technical users.
- **PostgreSQL as default**: Requires an external service; SQLite provides
  zero-deployment overhead for local and desktop use. PostgreSQL remains an
  option for the server backend.
- **XLIFF as container**: XLIFF is an interchange format for translatable
  content, not a project container. It has no support for bundling previews,
  original source files, or project-level metadata alongside translation data.
- **Directory-based projects**: Harder to share atomically; no single-file
  packaging. Risk of partial state from incomplete copies.
- **SQLite as the only format (no ZIP)**: Harder to inspect and debug than
  ZIP archives. Standard tools can examine KAZ files; SQLite requires
  specialized tooling. KAZ also provides a clear boundary between portable
  snapshots and the live store.

## Consequences

- Content-addressed storage eliminates duplicate work on repeated content.
  Blocks that appear across documents or projects are stored and processed once.
- Version tracking enables "what changed since last sync" queries, making
  incremental extraction practical for large content repositories.
- The store sits between connectors and the pipeline -- connectors write to
  the store, flows process store content. This decouples extraction from
  processing.
- SQLite backend works without external dependencies. TM and terminology can
  be co-located in the same storage infrastructure, sharing connection pooling
  and migration tooling ([ADR-009](./009-translation-memory.md),
  [ADR-010](./010-terminology.md)).
- KAZ provides portable project packaging for sharing and offline use while
  the SQLite store handles the live, queryable state.
- Block-level granularity is the right level for localization -- not too fine
  (characters or words) and not too coarse (documents or pages). This aligns
  with the content model's `Block` as the fundamental translatable unit
  ([ADR-002](./002-content-model.md)).
- Future PostgreSQL backend for team server scenarios requires only a new
  `ContentStore` implementation -- the interface abstracts the storage layer.
- Projects are scoped to workspaces ([ADR-015](./015-auth-and-workspaces.md)).
  The `workspace_id` column links each project to its owning workspace. In local
  mode (`kapi serve`), a default workspace is used implicitly.
