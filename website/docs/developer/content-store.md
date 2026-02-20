---
title: Content Store
sidebar_position: 11
---

# Content Store

The Content Store provides versioned, content-addressable persistence for localization content. It serves as the central persistence layer for gokapi projects.

## Architecture

The store sits between connectors (which pull/push external content) and the processing pipeline (flows, tools, TM, terminology):

```
Connectors → ContentStore ← → Flows/Tools
                  ↕
              Versions
```

### Key Concepts

- **BlockIdentity**: Content-addressable hashing (SHA-256) for block deduplication and change detection
- **ContentRef**: Links blocks to their external connector source with sync tracking
- **DisplayHint**: UI rendering guidance (preview, context, max length, content type)
- **Version**: Named snapshot of project state with block-level diffing

## ContentStore Interface

```go
type ContentStore interface {
    // Project management
    CreateProject(ctx context.Context, p *Project) error
    GetProject(ctx context.Context, id string) (*Project, error)
    ListProjects(ctx context.Context) ([]*Project, error)
    UpdateProject(ctx context.Context, p *Project) error
    DeleteProject(ctx context.Context, id string) error

    // Block storage with content-addressable deduplication
    StoreBlocks(ctx context.Context, projectID string, blocks []*model.Block) error
    GetBlock(ctx context.Context, projectID, blockID string) (*StoredBlock, error)
    GetBlocks(ctx context.Context, query BlockQuery) ([]*StoredBlock, error)
    DeleteBlock(ctx context.Context, projectID, blockID string) error

    // Version management
    CreateVersion(ctx context.Context, projectID, label, description string) (*Version, error)
    GetVersion(ctx context.Context, versionID string) (*Version, error)
    ListVersions(ctx context.Context, projectID string) ([]*Version, error)
    Diff(ctx context.Context, fromVersion, toVersion string) (*VersionDiff, error)

    Close() error
}
```

## SQLite Backend

The default implementation uses SQLite via the shared `bowrain/storage` layer with WAL mode for concurrent access:

```go
store, err := store.NewSQLiteStore("project.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

### Schema

The SQLite backend uses four tables:

| Table | Purpose |
|-------|---------|
| `projects` | Project metadata and locale configuration |
| `blocks` | Block content with content-addressable hashes |
| `versions` | Named version snapshots |
| `version_blocks` | Block-to-version mapping for diff computation |

## Block Identity

Every stored block gets a content-addressable identity computed from its source text:

```go
identity := model.ComputeIdentity(block)
// identity.ContentHash = SHA-256 of normalized source text
// identity.ContextHash = SHA-256 of block name, type, and properties
```

This enables:
- **Deduplication**: Identical source text shares the same content hash
- **Change detection**: Version diffs compare content hashes instead of full text
- **Cache invalidation**: Translations can be cached by content hash

## Version Tracking

Versions are named snapshots of a project's block state:

```go
// Create a snapshot
v, err := store.CreateVersion(ctx, projectID, "v1.0", "Initial release")

// List versions
versions, err := store.ListVersions(ctx, projectID)

// Diff two versions
diff, err := store.Diff(ctx, v1.ID, v2.ID)
for _, change := range diff.Changes {
    fmt.Printf("%s: %s\n", change.BlockID, change.ChangeType)
}
```

## Flow Integration

Flows can be connected to the content store via `WithStore()`:

```go
executor := flow.NewFlowExecutor(
    flow.WithStore(contentStore, projectID),
)
```
