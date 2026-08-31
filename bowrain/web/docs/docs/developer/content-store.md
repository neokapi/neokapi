---
title: Content Store
sidebar_position: 11
---

# Content Store

The Content Store provides versioned, content-addressable persistence for content. It serves as the central persistence layer for neokapi projects.

## Architecture

The store sits between connectors (which pull/push external content) and the processing pipeline (flows, tools, content memory, terms):

```
Connectors → ContentStore ← → Flows/Tools
                  ↕
              Versions
```

### Key Concepts

- **BlockIdentity**: content-addressable hashing (SHA-256) for block deduplication and change detection
- **ContentRef**: links blocks to their external connector source with sync tracking
- **DisplayHint**: UI rendering guidance (preview, context, max length, content type)
- **Version**: named snapshot of project state with block-level diffing

## ContentStore Interface

`ContentStore` (`bowrain/core/store/store.go`) is the union of role
interfaces, one per concern. All content operations are **stream-scoped**: an
empty stream name defaults to `"main"`, which every project implicitly has.

```go
// ContentStore is the primary persistence interface for content,
// the union of the role interfaces. All content operations are stream-scoped.
type ContentStore interface {
    ProjectStore    // projects: create, get, list, update, delete
    StreamStore     // streams within a project
    CollectionStore // collections (stream-scoped)
    ItemStore       // items (stream-scoped)
    BlockStore      // blocks, notes, history (stream-scoped)
    VersionStore    // named versions + diffs (stream-scoped)
    ChangeFeed      // incremental sync change log (stream-scoped)
    AssetStore      // assets and locale variants

    Close() error
}
```

Representative signatures; note the `stream` parameter throughout:

```go
// BlockStore
StoreBlocks(ctx context.Context, projectID, stream string, blocks []*model.Block) error
GetBlocks(ctx context.Context, query BlockQuery) ([]*StoredBlock, error)

// VersionStore
CreateVersion(ctx context.Context, projectID, stream, label, description string) (*Version, error)
Diff(ctx context.Context, fromVersion, toVersion string) (*VersionDiff, error)
```

## Backends

Two backends implement `ContentStore`, with different roles:

- **PostgreSQL** is the server's only backend. `bowrain-server` refuses to
  start without a `postgres://` database URL and builds all of its stores on
  that connection. This is the source of truth for every workspace.
- **SQLite** (`bowrain/store/sqlitestore`) backs the desktop app's local
  working copy: a cache for speed and offline edits that mirrors the server
  and is never a source of truth.

```go
import "github.com/neokapi/neokapi/bowrain/store/sqlitestore"

store, err := sqlitestore.NewSQLiteStore("working-copy.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

Both backends share one logical schema: projects, streams, collections,
items, blocks, versions, the change log, and assets.

## Block Identity

Every stored block gets a content-addressable identity computed from its source text:

```go
identity := model.ComputeIdentity(block)
// identity.ContentHash = SHA-256 of normalized source text
// identity.ContextHash = SHA-256 of block name, type, and properties
```

This enables:

- **Deduplication**: identical source text shares the same content hash
- **Change detection**: version diffs compare content hashes instead of full text
- **Cache invalidation**: results can be cached by content hash

## Version Tracking

Versions are named snapshots of a project's block state:

```go
// Create a snapshot of a stream
v, err := store.CreateVersion(ctx, projectID, "main", "v1.0", "Initial release")

// List a stream's versions
versions, err := store.ListVersions(ctx, projectID, "main")

// Diff two versions
diff, err := store.Diff(ctx, v1.ID, v2.ID)
for _, change := range diff.Changes {
    fmt.Printf("%s: %s\n", change.BlockID, change.ChangeType)
}
```

## Flow Integration

Server-side flows read from and write to the content store through the flow
service rather than a flow-executor option: a run loads the project's blocks
from the store, executes the flow, and stores the produced targets back. See
[Server-side flows](/server/flows).
