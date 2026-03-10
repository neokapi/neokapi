---
sidebar_position: 1
title: "Content Store Schema"
---
# Content Store Schema

This note provides implementation details for [AD-003](/docs/ad/003-content-store).

## SQLite Schema

The default backend uses SQLite via `modernc.org/sqlite` (pure Go, no CGO). This shares the `bowrain/storage/` infrastructure layer with the Sievepen TM system ([AD-009](/docs/ad/009-translation-memory)) and the TermBase ([AD-010](/docs/ad/010-terminology)).

```sql
CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    source_locale TEXT NOT NULL,
    target_locales TEXT NOT NULL,    -- JSON array of BCP-47 tags
    workspace_id TEXT NOT NULL DEFAULT '',  -- FK to workspaces (AD-015)
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

The `project_blocks` join table associates blocks with projects. A single block can belong to multiple projects if the same content appears in different contexts. The `blocks` table is global -- content-addressed storage means identical blocks are never duplicated regardless of which project they belong to.

SQLite provides zero-deployment overhead for local development and single-instance deployments. A future PostgreSQL backend can serve the multi-instance team server scenario where multiple server replicas share a central database.

## REST API Routes

The ContentStore is exposed via Bowrain Server's REST API ([AD-013](/docs/ad/013-cli-and-server)):

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

# Sync operations (for Bowrain CLI pull/push)
GET    /api/v1/workspaces/:ws/projects/:id/sync-state
POST   /api/v1/workspaces/:ws/projects/:id/pull
POST   /api/v1/workspaces/:ws/projects/:id/push

```

Bowrain CLI interacts with the store via API, not directly. The `bowrain pull/push` commands call the sync endpoints, which query the ContentStore, compute diffs, and return only changed blocks ([AD-016](/docs/ad/016-kapi-project-model)).

