---
id: 023-identity-system
sidebar_position: 23
title: "AD-023: Identity System"
---

# AD-023: Short IDs and Dual Block Identity

## Context

The platform originally used UUID v4 identifiers for all entities (projects,
workspaces, users, blocks, events, jobs). UUIDs are globally unique but
have practical drawbacks:

- **36 characters** in canonical form (`f47ac10b-58cc-4372-a567-0e02b2c3d479`)
  make URLs and API responses unnecessarily verbose.
- **Not URL-friendly**: hyphens and length make them awkward in REST paths
  and browser address bars.
- **External dependency**: required `github.com/google/uuid`.

A separate problem existed for blocks specifically: format readers assign
IDs from the source format (e.g., `tu1`, `tu2` in XLIFF), but these IDs
are not unique across files. When multiple files with overlapping reader
IDs are stored in the same project, the visual editor and content store
cannot distinguish which file's translation to apply.

## Decision

### Short Base62 IDs

All entity IDs across the Bowrain module are generated using 8-character
base62-encoded random strings via `core/id`:

```go
// core/id/id.go
func New() string {
    var buf [8]byte
    if _, err := rand.Read(buf[:]); err != nil {
        panic("crypto/rand failed: " + err.Error())
    }
    out := make([]byte, 8)
    for i, b := range buf {
        out[i] = base62[int(b)%len(base62)]
    }
    return string(out)
}
```

Properties:

- **8 characters**, base62 alphabet (0-9, A-Z, a-z)
- **~47.6 bits** of entropy per ID
- **`crypto/rand`** for cryptographic randomness
- **Zero external dependencies** — no uuid, nanoid, or shortid library
- **URL-safe** — no special characters, no encoding needed

`id.New()` replaces `uuid.NewString()` for projects, workspaces, users,
invites, refresh tokens, API tokens, events, subscriptions, notifications,
notes, review queue entries, jobs, and credentials.

### Dual Block Identity

Blocks use a two-level ID system that separates **internal IDs**
(project-unique, randomly generated) from **source IDs** (format-reader
assigned, file-scoped):

| Property   | Internal ID                          | Source ID                   |
| ---------- | ------------------------------------ | --------------------------- |
| Generator  | `id.New()` (8-char base62)           | Format reader (e.g., `tu1`) |
| Uniqueness | Per project                          | Per file within project     |
| Stored in  | `blocks.id` (primary key)            | `blocks.source_id`          |
| Used by    | API responses, editor, internal refs | Export roundtrip matching   |

**Ingestion path** (`StoreBlocksForItem`): When blocks arrive from a
format reader with an `itemName` (file path), the incoming `block.ID` is
treated as a `source_id`. The store looks up whether this (project,
itemName, sourceID) triple has been seen before — if so, the existing
internal ID is reused; if not, a new internal ID is generated. Both IDs
are persisted.

**Re-save path** (`StoreBlocks`): When blocks are stored without an
`itemName` (e.g., after translation, TM leverage, or editor edits), the
block already carries an internal ID from a previous `GetBlocks()` call.
No source ID remapping occurs.

**Export path**: When writing translated files, the system re-parses the
source file to get format-reader IDs, then matches them against stored
`source_id` values to inject translations into the correct positions.

### Database Schema

```sql
-- Primary key is (project_id, id) — internal IDs unique per project
CREATE TABLE blocks (
    id          TEXT NOT NULL,
    project_id  TEXT NOT NULL,
    item_name   TEXT NOT NULL DEFAULT '',
    source_id   TEXT NOT NULL DEFAULT '',
    -- ... content fields ...
    PRIMARY KEY (project_id, id)
);

-- Source IDs are unique per file within a project
CREATE UNIQUE INDEX idx_blocks_source_id
    ON blocks(project_id, item_name, source_id)
    WHERE source_id != '';
```

The `StoredBlock` type exposes both IDs:

```go
type StoredBlock struct {
    *model.Block
    ProjectID   string
    ItemName    string
    SourceID    string    // format-reader ID; empty for non-file blocks
    ContentHash string
    ContextHash string
    StoredAt    time.Time
    UpdatedAt   time.Time
}
```

## Alternatives Considered

- **UUIDs**: Globally unique and collision-proof, but 36 characters are
  excessive for a platform where IDs appear in URLs, API responses, and
  CLI output. At ~47.6 bits of entropy, short IDs have negligible collision
  probability for per-project scopes (millions of entities before concern).

- **Sequential IDs**: Predictable, which leaks information about entity
  count and creation order. Random IDs avoid enumeration attacks.

- **Nanoid/shortid libraries**: Solve the same problem but add an external
  dependency. The implementation is 10 lines of code using `crypto/rand`
  and does not justify a dependency.

- **Single block ID**: Using format-reader IDs directly causes collisions
  when multiple files share the same ID space (e.g., two XLIFF files
  both containing `tu1`). Project-unique internal IDs eliminate this
  ambiguity.

- **Composite keys for blocks** (project + file + reader ID): Would work
  for disambiguation but requires all downstream consumers (editor,
  API, TM, etc.) to carry three-part keys. A single internal ID is
  simpler for API consumers and storage.

## Consequences

- URLs and API responses are shorter and more readable
  (`/projects/aB3xK9mL` vs `/projects/f47ac10b-58cc-4372-a567-0e02b2c3d479`).

- No external dependency for ID generation — `crypto/rand` from the
  standard library is the only requirement.

- Blocks from different files with overlapping format-reader IDs
  (e.g., `tu1` in two XLIFF files) are correctly distinguished in the
  content store and visual editor.

- Export roundtrip works correctly: source IDs map re-parsed blocks back
  to stored translations, while internal IDs serve all internal references.

- The `source_id` column and unique index add minimal storage overhead
  but prevent a class of subtle data corruption bugs where translations
  from one file would overwrite another's.

- Existing migrations handle the transition: blocks stored before the
  dual-ID system have empty `source_id` values and fall back to
  matching by internal ID during export.
