---
sidebar_position: 1
title: "Content Store Schema"
---

# Content Store Schema

This note provides implementation details for [AD-004](/architecture-decisions/004-content-store).

## Database Schema

The Content Store schema is shared across SQLite (CLI) and PostgreSQL (server) backends, with the Sievepen TM system ([Framework AD-009](https://neokapi.github.io/web/neokapi/docs/architecture/009-translation-memory)) and TermBase ([Framework AD-010](https://neokapi.github.io/web/neokapi/docs/architecture/010-terminology)). The schema below shows the table definitions using SQLite syntax for readability; server backends use equivalent types with `$N` parameter placeholders instead of `?`.

```sql
CREATE TABLE projects (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    source_locale  TEXT NOT NULL,
    target_locales TEXT NOT NULL DEFAULT '',  -- JSON array of BCP-47 tags
    properties     TEXT NOT NULL DEFAULT '{}',
    created_at     TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE items (
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    format       TEXT NOT NULL DEFAULT '',
    item_type    TEXT NOT NULL DEFAULT 'file',
    source_bytes BLOB,
    block_index  TEXT NOT NULL DEFAULT '{}',
    properties   TEXT NOT NULL DEFAULT '{}',
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (project_id, name)
);

CREATE TABLE blocks (
    id           TEXT NOT NULL,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    item_name    TEXT NOT NULL DEFAULT '',
    source_id    TEXT NOT NULL DEFAULT '',
    name         TEXT NOT NULL DEFAULT '',
    type         TEXT NOT NULL DEFAULT '',
    mime_type    TEXT NOT NULL DEFAULT '',
    translatable INTEGER NOT NULL DEFAULT 1,
    content_hash TEXT NOT NULL DEFAULT '',
    context_hash TEXT NOT NULL DEFAULT '',
    source_json  TEXT NOT NULL DEFAULT '[]',    -- serialized Fragment JSON
    targets_json TEXT NOT NULL DEFAULT '{}',    -- JSON map: locale -> segments
    properties   TEXT NOT NULL DEFAULT '{}',
    annotations  TEXT NOT NULL DEFAULT '{}',
    stored_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (project_id, id)
);
CREATE UNIQUE INDEX idx_blocks_source_id ON blocks(project_id, item_name, source_id)
    WHERE source_id != '';

CREATE TABLE versions (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    label       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    block_count INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE version_blocks (
    version_id   TEXT NOT NULL REFERENCES versions(id) ON DELETE CASCADE,
    block_id     TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    PRIMARY KEY (version_id, block_id)
);

CREATE TABLE streams (
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    parent      TEXT NOT NULL DEFAULT '',
    base_cursor INTEGER NOT NULL DEFAULT 0,
    archived    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    created_by  TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (project_id, name)
);

CREATE TABLE change_log (
    seq          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id   TEXT NOT NULL,
    block_id     TEXT NOT NULL,
    change_type  TEXT NOT NULL,  -- source_added, source_modified, source_removed,
                                -- target_added, target_modified
    stream       TEXT NOT NULL DEFAULT 'main',
    locale       TEXT,
    content_hash TEXT,
    logged_at    TEXT NOT NULL
);
CREATE INDEX idx_changelog_project_seq ON change_log(project_id, seq);
CREATE INDEX idx_changelog_project_locale ON change_log(project_id, locale, seq);
CREATE INDEX idx_changelog_stream ON change_log(project_id, stream, seq);
```

Blocks are scoped to projects with a composite primary key `(project_id, id)`. The `source_id` column tracks the format-reader-assigned ID (e.g., "tu1" from PO format), with a unique index ensuring no duplicates within an item. The `version_blocks` join table associates blocks with version snapshots. The `streams` table tracks content branches within a project ([AD-005: Streams](/architecture-decisions/005-streams)). The append-only `change_log` enables cursor-based incremental sync, with a `stream` column scoping changes to their branch.

PostgreSQL is the server's storage backend, supporting connection pooling, concurrent writes, and multi-instance deployments where multiple server replicas share a central database.

## REST API Routes

The ContentStore is exposed via Bowrain Server's REST API ([AD-011](/architecture-decisions/011-rest-api)):

```
# Project operations (JWT-protected)
POST   /api/v1/projects
GET    /api/v1/projects
GET    /api/v1/projects/:id
PUT    /api/v1/projects/:id
DELETE /api/v1/projects/:id

# Block operations
POST   /api/v1/projects/:id/blocks
GET    /api/v1/projects/:id/blocks

# Version operations
POST   /api/v1/projects/:id/versions
GET    /api/v1/projects/:id/versions

# Sync operations (JWT or ClaimToken)
POST   /api/v1/projects/:id/sync/push
GET    /api/v1/projects/:id/sync/pull
GET    /api/v1/projects/:id/sync/blocks
GET    /api/v1/projects/:id/sync/status
GET    /api/v1/projects/:id/changes

# Workspace-scoped sync routes (same handlers)
POST   /api/v1/workspaces/:ws/projects/:id/sync/push
GET    /api/v1/workspaces/:ws/projects/:id/sync/pull
GET    /api/v1/workspaces/:ws/projects/:id/sync/blocks
GET    /api/v1/workspaces/:ws/projects/:id/sync/status
```

kapi interacts with the store via API, not directly. The `kapi pull/push` commands call the sync endpoints, which query the ContentStore, compute diffs, and return only changed blocks ([AD-009](/architecture-decisions/009-sync-protocol)).

## Migration Strategy

### PostgreSQL: Single consolidated migration

The PostgreSQL content store schema (`platform/store/migrations_pg.go`) uses a **single migration** containing the complete schema. There are no incremental migrations — we start from scratch on each fresh database.

When adding new tables or columns:

- **Add to the single migration** in `migrations_pg.go` — do not add migration 2
- If deployed databases exist, add a separate migration with `ALTER TABLE IF NOT EXISTS` guards
- The migration table is `store_schema_migrations`

Other stores follow the same pattern:

- `jobs_schema_migrations` — translation jobs (single migration)
- `extraction_schema_migrations` — extraction jobs (single migration)
- `auth_schema_migrations`, `brand_schema_migrations`, etc. — may still have incremental migrations

### SQLite: Incremental migrations

SQLite (`platform/store/migrations.go`) uses incremental migrations (currently at version 33). Each new table or column gets a new version. The migration table is `schema_migrations`.
