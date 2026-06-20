---
sidebar_position: 8
title: "Bowrain Sync Protocol"
---

# Bowrain Sync Protocol

This note provides implementation details for [AD-009](/architecture-decisions/009-sync-protocol) and [AD-010](/architecture-decisions/010-bowrain-cli-and-project-model).

## Recipe (`<dir-name>.kapi`) Full Schema

```yaml
version: v1
name: my-app

# Project-wide defaults for language and organization.
defaults:
  source_language: en-US
  target_languages: [fr-FR, de-DE, ja-JP]
  collection: ui/strings

# Content collections: which files to track.
content:
  - path: src/locales/**/*.json
    format: json

  - path: content/docs/**/*.md
    format: markdown
    target: i18n/{lang}/docs/{path}/{filename}

  - path: src/es/**/*.json
    format: json
    source_language: es     # Override source language for this entry
    collection: spanish-ui  # Override collection for this entry

# Plugins: map of name -> version constraint.
plugins:
  okapi-bridge: "^1.47.0"

# Bowrain server connection. Optional — its presence enables push/pull/sync.
server:
  # Compound URL encoding server, workspace, and project ID.
  # Formats:
  #   https://bowrain.example.com/my-team/abc123     (workspace project)
  #   https://bowrain.example.com/projects/abc123     (direct project, no workspace)
  url: https://bowrain.example.com/my-team/abc123
  # Stream determines which content stream to sync with.
  # Default: $auto (detect from git branch / CI environment)
  # Explicit: "main", "v2.0", "feature/new-ui"
  stream: $auto

# Hooks: flows that run automatically at lifecycle points.
hooks:
  pre-push: [qa, term-enforce]
  post-pull: [update-stats]

# Flow definitions (inline; or in .kapi/flows/<name>.yaml).
flows:
  pseudo:
    steps:
      - tool: pseudo-translate
        config: { method: extended }
```

**Field descriptions:**

- **`server.url`** -- Compound URL encoding server, workspace, and project ID. Parsed on demand via `ParseProjectURL()`. Accessor methods: `ServerURL()`, `ProjectID()`, `Workspace()`, `HasServer()`. Claim tokens for anonymous projects are stored in `.kapi/cache/sync-cache.json` (gitignored), not in the URL.
- **`server.stream`** -- Content stream name. Defaults to `$auto` (auto-detect from git branch or CI environment variables). Set to a specific name like `v2.0` to pin the stream. See [AD-005](/architecture-decisions/005-streams) for full stream design.
- **`defaults.source_language`** -- BCP-47 tag for the project's default source language (e.g., `en-US`)
- **`defaults.target_languages`** -- Array of BCP-47 tags for target languages. When empty, the CLI falls back to server-side target locales (cached in the sync cache).
- **`defaults.collection`** -- Default collection for organizing content on the server
- **`content`** -- Array of content collections defining which files to track (see below)
- **`plugins`** -- Map of plugin name to semver constraint (e.g., `okapi-bridge: "^1.47.0"`)
- **`hooks`** -- Flow names to run on events (`pre-push`, `post-pull`)
- **`flows`** -- Inline flow definitions (file-per-flow definitions also supported under `.kapi/flows/`)

## Content Collections

Content collections define which files to track and how they map to the server:

```yaml
content:
  - path: src/locales/{lang}/*.json
    format: json
    target: src/locales/{lang}/*.json
    base: src/locales/
    collection: ui
    source_language: en
    target_languages: [fr, de]
```

**Fields:**

- **`path`** -- Glob pattern for source files (relative to project root). May contain `\{lang\}` placeholder expanded with the source language.
- **`target`** -- (Optional) Output path pattern for translated files. May contain `\{lang\}` for target locale, or `\{locale\}`, `\{path\}`, `\{filename\}` for legacy-style templates.
- **`format`** -- Format ID (from FormatRegistry: `json`, `html`, `markdown`, etc.) — string or object form (`{ name, config, preset }`). Use `$auto` or omit for auto-detection by file extension.
- **`base`** -- (Optional) Path prefix to strip when reporting files to the server (e.g., `src/locales/` so the server sees `en/messages.json` instead of `src/locales/en/messages.json`).
- **`collection`** -- (Optional) Override the default collection for this content entry. Sent with each block during push.
- **`source_language`** -- (Optional) Override the project's default source language for this entry. Enables projects with multiple source languages.
- **`target_languages`** -- (Optional) Override the default target languages for this entry.

### Per-Entry Language Override

Projects with content in multiple source languages use the `language` field:

```yaml
defaults:
  source_language: en

content:
  - path: src/en/**/*.json
    format: json
    # Uses defaults.source_language (en)

  - path: src/es/**/*.json
    format: json
    source_language: es # This content is in Spanish, not English
```

The `EffectiveLanguage()` method on `ContentEntry` resolves the per-entry language, falling back to the project default. All code paths that expand `\{lang\}` placeholders use this method.

### Dynamic Target Languages

When `defaults.target_languages` is empty, the CLI automatically fetches target locales from the server during pull. The resolution order is:

1. CLI flags (`--locales fr,de`)
2. `defaults.target_languages` in config
3. Server-side target locales (cached in `.kapi/cache/sync-cache.json` as `server_meta`)

Server metadata is fetched via `GET /api/v1/projects/:id` and cached locally so subsequent operations don't require a network round-trip.

### Collections

Collections organize content on the server. They are resolved per content entry:

1. `content[].collection` (per-entry override)
2. `defaults.collection` (project-wide default)

Collections are sent with each block during push via the `collection` field in `BlockInput`.

## Sync Cache (`.kapi/cache/sync-cache.json`) Format

```json
{
  "server_url": "https://bowrain.example.com",
  "project_id": "abc123",
  "sync_cursor": 4821,
  "last_sync": "2026-02-15T10:30:00Z",
  "claim_token": "clm_abc123",
  "files": {
    "src/locales/en-US.json": {
      "mtime": "2026-02-15T10:25:00Z",
      "size": 4096,
      "blocks": {
        "greeting": "a1b2c3d4...",
        "farewell": "e5f6a7b8..."
      }
    }
  },
  "server_meta": {
    "target_locales": ["fr-FR", "de-DE", "ja-JP"],
    "fetched_at": "2026-02-15T10:30:00Z"
  }
}
```

**Key fields:**

- **`sync_cursor`** -- Monotonic sequence number from the server's change log. Used by `pull` to request only changes since the last sync (`WHERE seq > cursor`). This follows the Contentful sync token / CouchDB sequence ID pattern.
- **`last_sync`** -- Timestamp of the last successful push or pull.
- **`claim_token`** -- Claim token for anonymous projects. Stored here (gitignored) rather than in the recipe to avoid accidentally committing credentials to version control. Cleared after `kapi auth claim` transfers ownership.
- **`files`** -- Per-file entries keyed by relative path. Each entry tracks the file's mtime, size, and a map of block ID → content hash (SHA-256). Used by `push` to diff local blocks against the last known server state and send only changed blocks.
- **`server_meta`** -- Cached project metadata from the server, including target locales. Updated on each push/pull. Used to resolve dynamic target languages when `defaults.target_languages` is empty.

**Design principles:**

- **Cache, not state**: The sync cache can be deleted and regenerated. Deleting it forces a full re-scan on the next push (expensive but correct). The server is the source of truth.
- **Block-level granularity**: Tracks individual block hashes, not file-level hashes. When one string changes in a 100-string file, only that block is pushed.
- **Gitignored**: Contains local-only data. Each developer's cache tracks their own sync position.
- **Server metadata caching**: Target locales and other project metadata are cached locally to avoid redundant API calls. The cache is refreshed on each sync operation.

## Push Algorithm (Cursor-Based)

```
0. Resolve stream: --stream flag > BOWRAIN_STREAM env > server.stream > $auto > "main"
1. Scan local files -> extract blocks -> compute hashes
   - Each content entry uses its effective language for {lang} expansion
   - Collections resolved per-entry (entry override > default)
2. Diff block hashes against .kapi/cache/sync-cache.json -> identify changed blocks
3. Send changed blocks to server: POST /projects/:id/sync/push (batched)
   - Each block includes: id, text, name, type, item_name, collection
   - X-Bowrain-Stream header sent for non-main streams
4. Server appends to change log (scoped to stream), returns new cursor
5. Fetch project metadata from server (best-effort) -> cache in .kapi/cache/sync-cache.json
6. Update .kapi/cache/sync-cache.json with new hashes + cursor + server metadata
```

## Pull Algorithm (Cursor-Based)

```
0. Resolve stream: --stream flag > BOWRAIN_STREAM env > server.stream > $auto > "main"
1. Fetch project metadata from server -> cache target locales
2. Resolve target locales: CLI flags > config > server cache
3. Read sync_cursor from .kapi/cache/sync-cache.json
4. Query server: GET /projects/:id/sync/pull?cursor=X&locales=fr-FR
   - X-Bowrain-Stream header sent for non-main streams
5. Server returns only changes since cursor (O(changes), not O(total))
6. For each changed item, fetch blocks and write translated files
7. Update .kapi/cache/sync-cache.json with new cursor + server metadata
```

The append-only change log with sync cursors follows the industry-standard pattern used by Contentful (sync tokens), CouchDB (sequence IDs), and Firebase (timestamp-based feeds).

## Server API Endpoints

When `url` is configured, kapi uses the Bowrain Server REST API ([AD-011](/architecture-decisions/011-rest-api)):

**Sync API endpoints:**

```
POST /api/v1/projects/:id/sync/push       # Push source blocks to server
GET  /api/v1/projects/:id/sync/pull        # Pull changes since cursor
GET  /api/v1/projects/:id/sync/blocks      # Get blocks for an item
GET  /api/v1/projects/:id/sync/status      # Push status (translation job tracking)
GET  /api/v1/projects/:id                  # Project metadata (languages, name)
POST /api/v1/projects/:id/sync/translate   # Create translation job for pushed content
GET  /api/v1/projects/:id/changes          # Raw change log query
```

**Stream API endpoints** ([AD-005](/architecture-decisions/005-streams)):

```
GET    /api/v1/projects/:id/streams                    # List streams
POST   /api/v1/projects/:id/streams                    # Create stream
GET    /api/v1/projects/:id/streams/:name              # Get stream info
DELETE /api/v1/projects/:id/streams/:name              # Archive stream
POST   /api/v1/projects/:id/streams/:name/merge        # Merge into parent
GET    /api/v1/projects/:id/streams/:name/diff          # Diff against parent
```

Push and pull endpoints accept the `X-Bowrain-Stream` header to target a specific stream. When absent, operations target the `main` stream. The server auto-creates a stream on first push if it doesn't exist.

Workspace-scoped equivalents are also available at `/api/v1/workspaces/:ws/projects/:id/sync/...`.

**Push workflow:**

```
1. Read local files via FormatRegistry -> extract blocks
2. Compute block hashes (BlockIdentity SHA-256)
3. Compare with .kapi/cache/sync-cache.json -> identify changed blocks
4. Resolve collections per content entry
5. Run pre-push automations (if configured; recipe `hooks:` are validated but not yet executed — see /cli/flows/hooks)
6. POST /api/v1/projects/:id/sync/push
   -> Request body: { blocks: [{id, text, name, type, item_name, collection}] }
   -> Response: { stored: N, new_cursor: X, push_id: "..." }
   -> Batched at 1000 blocks per request (MaxBlocksPerRequest)
7. GET /api/v1/projects/:id -> cache server metadata (target_locales)
8. Update .kapi/cache/sync-cache.json with new hashes + cursor + server metadata
```

**Pull workflow:**

```
1. GET /api/v1/projects/:id -> cache server metadata (target_locales)
2. Resolve target locales: CLI flags > config > server cache
3. Read sync_cursor from .kapi/cache/sync-cache.json
4. GET /api/v1/projects/:id/sync/pull?cursor=X&locales=fr-FR,de-DE
   -> Response: { changes: [...], new_cursor: Y, has_more: bool }
   -> Paginated: follow has_more until all changes consumed
5. For each item with changes:
   -> GET /api/v1/projects/:id/sync/blocks?item_name=...
   -> Write translated file for each target locale
6. Update .kapi/cache/sync-cache.json with new cursor + server metadata
```

**Server-side change log:**

The server maintains an append-only change log (`change_log` table) that records every mutation to a project's blocks. Each entry has a monotonic sequence number (`seq`). Sync queries are O(changes) via indexed cursor lookup -- the server never needs to diff entire version snapshots.

Authentication uses the token from `kapi auth login` stored at `~/.config/bowrain/auth.json` ([AD-002](/architecture-decisions/002-authentication-and-workspaces)).
