---
sidebar_position: 8
title: "Kapi Sync Protocol"
---
# Kapi Sync Protocol

This note provides implementation details for [AD-016](/docs/ad/016-kapi-project-model).

## `config.yaml` Full Schema

```yaml
# Project identity and locales
project:
  name: my-app
  source_locale: en-US
  target_locales: [fr-FR, de-DE, ja-JP]

# Optional: Bowrain Server connection
server:
  url: https://bowrain.example.com
  project_id: abc123
  workspace: my-team
  # Auth token comes from: kapi auth login --server URL

# File mappings: local paths <-> remote items
mappings:
  - local: src/locales/**/*.json
    remote: ui/strings/{path}
    format: json

  - local: content/docs/**/*.md
    remote: docs/{path}
    format: markdown

  - local: public/pages/**/*.html
    remote: pages/{filename}
    format: html

# Hooks: flows that run automatically
hooks:
  pre-push: [qa-check, term-enforce]
  post-pull: [update-stats]

# Flow-specific settings
flows:
  pseudo:
    target_locale: qps
    method: extended
```

**Field descriptions:**

- **`project.name`** -- Human-readable project name
- **`project.source_locale`** -- BCP-47 tag for source locale (e.g., `en-US`)
- **`project.target_locales`** -- Array of BCP-47 tags for target locales
- **`server.url`** -- (Optional) Bowrain Server URL for pull/push
- **`server.project_id`** -- (Optional) Remote project ID
- **`server.workspace`** -- (Optional) Workspace slug for workspace-scoped API routes
- **`mappings`** -- Array of local-to-remote path mappings (see below)
- **`hooks`** -- Flow names to run on events (`pre-push`, `post-pull`)
- **`flows`** -- Per-flow configuration overrides

## File Mappings

Mappings define the relationship between local files and remote project items:

```yaml
mappings:
  - local: src/locales/**/*.json
    remote: ui/strings/{path}
    format: json
```

**Fields:**
- **`local`** -- Glob pattern for local files (relative to project root)
- **`remote`** -- Remote item path template (can use `\{path\}`, `\{filename\}`, `\{basename\}`)
- **`format`** -- Format ID (from FormatRegistry: `json`, `html`, `markdown`, etc.)

**Path templates:**
- `\{path\}` -- Full path relative to glob root
- `\{filename\}` -- Filename with extension
- `\{basename\}` -- Filename without extension

Example resolution:
```
local: src/locales/auth/login.json
remote template: ui/strings/{path}
-> remote: ui/strings/auth/login
```

## Sync Cache (`.sync-cache`) JSON Format

```json
{
  "server_url": "https://bowrain.example.com",
  "project_id": "abc123",
  "sync_cursor": 4821,
  "last_sync": "2026-02-15T10:30:00Z",
  "files": {
    "_blocks": {
      "mtime": "0001-01-01T00:00:00Z",
      "size": 0,
      "blocks": {
        "greeting": "a1b2c3d4...",
        "farewell": "e5f6a7b8..."
      }
    }
  }
}
```

**Key fields:**
- **`sync_cursor`** -- Monotonic sequence number from the server's change log. Used by `pull` to request only changes since the last sync (`WHERE seq > cursor`). This follows the Contentful sync token / CouchDB sequence ID pattern.
- **`last_sync`** -- Timestamp of the last successful push or pull.
- **`files._blocks`** -- Map of block ID -> content hash (SHA-256). Used by `push` to diff local blocks against the last known server state and send only changed blocks.

**Design principles:**
- **Cache, not state**: The sync cache can be deleted and regenerated. Deleting it forces a full re-scan on the next push (expensive but correct). The server is the source of truth.
- **Block-level granularity**: Tracks individual block hashes, not file-level hashes. When one string changes in a 100-string file, only that block is pushed.
- **Gitignored**: Contains local-only data. Each developer's cache tracks their own sync position.

## Push Algorithm (Cursor-Based)

```
1. Scan local files -> extract blocks -> compute hashes
2. Diff block hashes against .sync-cache -> identify changed blocks
3. Send changed blocks to server: POST /projects/:id/sync/push (batched)
4. Server appends to change log, returns new cursor
5. Update .sync-cache with new hashes + cursor
```

## Pull Algorithm (Cursor-Based)

```
1. Read sync_cursor from .sync-cache
2. Query server: GET /projects/:id/sync/pull?cursor=X&locales=fr-FR
3. Server returns only changes since cursor (O(changes), not O(total))
4. Update .sync-cache with new cursor
```

The append-only change log with sync cursors follows the industry-standard pattern used by Contentful (sync tokens), CouchDB (sequence IDs), and Firebase (timestamp-based feeds).

## Server API Endpoints

When `server.url` is configured, Kapi uses the Bowrain Server REST API ([AD-013](/docs/ad/013-cli-and-server)):

**Sync API endpoints:**
```
POST /api/v1/projects/:id/sync/push   # Push source blocks to server
GET  /api/v1/projects/:id/sync/pull   # Pull changes since cursor
GET  /api/v1/projects/:id/changes     # Raw change log query
```

**Push workflow:**
```
1. Read local files via FormatRegistry -> extract blocks
2. Compute block hashes (BlockIdentity SHA-256)
3. Compare with .sync-cache -> identify changed blocks
4. Run pre-push hooks (if configured)
5. POST /api/v1/projects/:id/sync/push
   -> Request body: { blocks: [{id, text, name, type}] }
   -> Response: { stored: N, new_cursor: X }
   -> Batched at 1000 blocks per request (MaxBlocksPerRequest)
6. Update .sync-cache with new hashes + cursor
```

**Pull workflow:**
```
1. Read sync_cursor from .sync-cache
2. GET /api/v1/projects/:id/sync/pull?cursor=X&locales=fr-FR,de-DE
   -> Response: { changes: [...], new_cursor: Y, has_more: bool }
   -> Paginated: follow has_more until all changes consumed
3. Update .sync-cache with new cursor
```

**Server-side change log:**

The server maintains an append-only change log (`change_log` table) that records every mutation to a project's blocks. Each entry has a monotonic sequence number (`seq`). Sync queries are O(changes) via indexed cursor lookup -- the server never needs to diff entire version snapshots.

Authentication uses the token from `kapi auth login` stored at `~/.config/kapi/auth.json` ([AD-015](/docs/ad/015-auth-and-workspaces)).
