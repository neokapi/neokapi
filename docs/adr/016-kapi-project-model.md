---
id: 016-kapi-project-model
sidebar_position: 16
title: "ADR-016: Kapi Project Model"
---
# ADR-016: Kapi Project Model

## Context

Kapi is the command-line swiss army knife for file-based localization workflows. It must operate effectively both standalone (local file processing) and connected (syncing with Bowrain Server). Traditional localization tools treat each file operation as isolated — there is no persistent project context, no history of what was synced, and no declarative configuration.

Modern developer tools solve this with **project directories**: `.git` for version control, `.terraform` for infrastructure, `package.json` for dependencies. The same pattern applies to localization — a `.kapi/` directory that captures project identity, file mappings, sync state, and workflow definitions.

This ADR establishes the `.kapi/` project model as the foundation of Kapi's architecture. Everything in Kapi operates within a project context. There are no standalone file operations — all commands require a `.kapi/` directory.

## Decision

### `.kapi/` Directory Structure

Every Kapi project is a directory containing a `.kapi/` subdirectory:

```
my-app/
├── .kapi/
│   ├── config.yaml      # Project configuration
│   ├── flows/           # Flow definitions (YAML)
│   │   ├── extract.yaml
│   │   ├── pseudo.yaml
│   │   └── qa.yaml
│   └── .sync-cache      # Sync cache (gitignored)
├── src/
│   └── locales/
│       ├── en-US.json
│       ├── fr-FR.json
│       └── de-DE.json
└── content/
    └── docs/
```

The `.kapi/` directory is created by `kapi init` and contains:

1. **`config.yaml`** — Project metadata, server connection, file mappings, and hooks
2. **`flows/`** — YAML definitions of file processing flows
3. **`.sync-cache`** — Sync cache tracking last known server state (block hashes, sync cursor) — gitignored

### `config.yaml` Schema

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
  # Auth token comes from: kapi auth login --server URL

# File mappings: local paths ↔ remote items
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

- **`project.name`** — Human-readable project name
- **`project.source_locale`** — BCP-47 tag for source locale (e.g., `en-US`)
- **`project.target_locales`** — Array of BCP-47 tags for target locales
- **`server.url`** — (Optional) Bowrain Server URL for pull/push
- **`server.project_id`** — (Optional) Remote project ID
- **`mappings`** — Array of local ↔ remote path mappings (see below)
- **`hooks`** — Flow names to run on events (`pre-push`, `post-pull`)
- **`flows`** — Per-flow configuration overrides

### File Mappings

Mappings define the relationship between local files and remote project items:

```yaml
mappings:
  - local: src/locales/**/*.json
    remote: ui/strings/{path}
    format: json
```

**Fields:**
- **`local`** — Glob pattern for local files (relative to project root)
- **`remote`** — Remote item path template (can use `{path}`, `{filename}`, `{basename}`)
- **`format`** — Format ID (from FormatRegistry: `json`, `html`, `markdown`, etc.)

**Path templates:**
- `{path}` — Full path relative to glob root
- `{filename}` — Filename with extension
- `{basename}` — Filename without extension

Example resolution:
```
local: src/locales/auth/login.json
remote template: ui/strings/{path}
→ remote: ui/strings/auth/login
```

### Sync Cache (`.sync-cache`)

The `.sync-cache` file is a lightweight JSON cache of the last known server state. It enables incremental sync without re-reading all files or re-querying the server on every operation.

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
- **`sync_cursor`** — Monotonic sequence number from the server's change log. Used by `pull` to request only changes since the last sync (`WHERE seq > cursor`). This follows the Contentful sync token / CouchDB sequence ID pattern.
- **`last_sync`** — Timestamp of the last successful push or pull.
- **`files._blocks`** — Map of block ID → content hash (SHA-256). Used by `push` to diff local blocks against the last known server state and send only changed blocks.

**Design principles:**
- **Cache, not state**: The sync cache can be deleted and regenerated. Deleting it forces a full re-scan on the next push (expensive but correct). The server is the source of truth.
- **Block-level granularity**: Tracks individual block hashes, not file-level hashes. When one string changes in a 100-string file, only that block is pushed.
- **Gitignored**: Contains local-only data. Each developer's cache tracks their own sync position.

**This file is gitignored** — it contains local cache data, not project configuration.

### Flow Definitions

Flows are file processing chains defined in `.kapi/flows/*.yaml`:

```yaml
# .kapi/flows/extract.yaml
name: extract
description: Extract translatable content to XLIFF

steps:
  - tool: extract
    input: "src/**/*.html"
    output: "locales/source.xliff"
    config:
      format: html
      preserve_whitespace: true

  - tool: segmentation
    input: "locales/source.xliff"
    output: "locales/source.xliff"
```

```yaml
# .kapi/flows/pseudo.yaml
name: pseudo
description: Generate pseudo-translations for testing

steps:
  - tool: pseudo-translate
    input: "locales/en-US.json"
    output: "locales/qps.json"
    config:
      method: extended
      expansion_rate: 1.3
```

**Flow execution:**
```bash
kapi flow run extract
kapi flow run pseudo
```

Flows use the same tool system as Bowrain ([ADR-006](./006-tool-system.md)) but operate on local files instead of streaming through the server pipeline.

### Project Initialization

The `kapi init` command creates a new project:

```bash
# Standalone project (no server)
cd my-app/
kapi init

# Connected project
kapi init --server https://bowrain.example.com --project my-app-l10n
```

**`kapi init` workflow:**
1. Check if `.kapi/` already exists (error if so)
2. Create `.kapi/config.yaml` with defaults
3. If `--server` provided:
   - Verify auth token exists (`kapi auth status`)
   - Fetch project metadata from server
   - Populate `server.url` and `server.project_id`
4. Create `.kapi/flows/` directory with example flows
5. Create `.gitignore` entry for `.kapi/.sync-cache`

**Example generated `config.yaml`:**
```yaml
project:
  name: my-app
  source_locale: en-US
  target_locales: []

# Uncomment to connect to Bowrain Server:
# server:
#   url: https://bowrain.example.com
#   project_id: your-project-id

mappings: []

hooks: {}
```

### Project Commands

All Kapi commands operate within a project context:

```bash
cd my-app/

# Show sync status
kapi status
# → Modified: src/locales/en-US.json
# → Remote changes: 2 blocks updated
# → Conflicts: 0

# Pull from Bowrain Server
kapi pull
# → Fetching remote changes...
# → Updated: src/locales/fr-FR.json (5 blocks)
# → Updated: src/locales/de-DE.json (3 blocks)

# Push to Bowrain Server
kapi push --message "Update source strings"
# → Running pre-push hooks: qa-check, term-enforce
# → Pushing 2 changed files...
# → Pushed: src/locales/en-US.json (12 blocks)

# Run a flow
kapi flow run pseudo

# Start local dashboard
kapi serve
# → Dashboard running at http://localhost:3000
```

Commands automatically discover the `.kapi/` directory by searching upward from the current directory (like git).

### Content Hashing for Sync

Kapi uses the same content-addressable hashing system as the ContentStore ([ADR-003](./003-content-store.md)):

1. **Read local file** via FormatRegistry
2. **Extract blocks** (streaming Parts → Blocks)
3. **Compute `BlockIdentity`** hash (from [ADR-002](./002-content-model.md))
4. **Compare hashes** with `.sync-cache`
5. **Sync only changed blocks** (batched at 1000 blocks per request)

This is identical to how the ContentStore works — Kapi and Bowrain Server share the same hashing algorithm, enabling efficient delta sync without transferring unchanged content.

**Push algorithm (cursor-based):**

```
1. Scan local files → extract blocks → compute hashes
2. Diff block hashes against .sync-cache → identify changed blocks
3. Send changed blocks to server: POST /projects/:id/sync/push (batched)
4. Server appends to change log, returns new cursor
5. Update .sync-cache with new hashes + cursor
```

**Pull algorithm (cursor-based):**

```
1. Read sync_cursor from .sync-cache
2. Query server: GET /projects/:id/sync/pull?cursor=X&locales=fr-FR
3. Server returns only changes since cursor (O(changes), not O(total))
4. Update .sync-cache with new cursor
```

The append-only change log with sync cursors follows the industry-standard pattern used by Contentful (sync tokens), CouchDB (sequence IDs), and Firebase (timestamp-based feeds).

### Server API Integration

When `server.url` is configured, Kapi uses the Bowrain Server REST API ([ADR-013](./013-cli-and-server.md)):

**Sync API endpoints:**
```
POST /api/v1/projects/:id/sync/push   # Push source blocks to server
GET  /api/v1/projects/:id/sync/pull   # Pull changes since cursor
GET  /api/v1/projects/:id/changes     # Raw change log query
```

**Push workflow:**
```
1. Read local files via FormatRegistry → extract blocks
2. Compute block hashes (BlockIdentity SHA-256)
3. Compare with .sync-cache → identify changed blocks
4. Run pre-push hooks (if configured)
5. POST /api/v1/projects/:id/sync/push
   → Request body: { blocks: [{id, text, name, type}] }
   → Response: { stored: N, new_cursor: X }
   → Batched at 1000 blocks per request (MaxBlocksPerRequest)
6. Update .sync-cache with new hashes + cursor
```

**Pull workflow:**
```
1. Read sync_cursor from .sync-cache
2. GET /api/v1/projects/:id/sync/pull?cursor=X&locales=fr-FR,de-DE
   → Response: { changes: [...], new_cursor: Y, has_more: bool }
   → Paginated: follow has_more until all changes consumed
3. Update .sync-cache with new cursor
```

**Server-side change log:**

The server maintains an append-only change log (`change_log` table) that records every mutation to a project's blocks. Each entry has a monotonic sequence number (`seq`). Sync queries are O(changes) via indexed cursor lookup — the server never needs to diff entire version snapshots.

Authentication uses the token from `kapi auth login` stored at `~/.config/gokapi/auth.json` ([ADR-015](./015-auth-and-workspaces.md)).

## Alternatives Considered

- **Global config file** (`~/.kapi/config.yaml`): Does not support multiple projects with different settings. Modern dev tools use per-project directories (`.git`, `.terraform`) for good reason.

- **Single `gokapi.yaml` file** (like `package.json`): Works for simple cases but becomes cluttered when adding flow definitions, mappings, and hooks. Separate `flows/` directory keeps configuration organized.

- **KAZ files as the project format**: KAZ archives ([ADR-003](./003-content-store.md)) are server-side snapshots, not working directories. They lack version control integration, live editing, and incremental sync. `.kapi/` directories integrate naturally with git.

- **Store-based local projects**: Using SQLite ContentStore for local work adds unnecessary complexity. Files are the native format — direct file editing is faster and more transparent than store operations.

- **No project requirement** (standalone file commands): Leads to inconsistent state, no sync history, and manual path management. Requiring projects enforces clean workflows and enables powerful features (status, diff, incremental sync).

## Consequences

- All Kapi operations require a `.kapi/` project directory — no standalone file commands. This enforces clean project structure and enables stateful operations.

- `kapi init` is the entry point for new projects, analogous to `git init` or `npm init`.

- `.kapi/config.yaml` is the single source of truth for project configuration. Checked into git, enabling team collaboration.

- `.kapi/.sync-cache` tracks the last known server state locally (block hashes + sync cursor). Gitignored, regenerable from the server.

- File mappings connect local paths to remote items, enabling bidirectional sync with Bowrain Server.

- Flows are defined in `.kapi/flows/*.yaml` — project-specific processing chains that use the same tool system as Bowrain but operate on files.

- Content hashing enables efficient incremental sync — only changed blocks are transferred, even for large projects.

- `kapi pull/push` commands mirror git's mental model (fetch/push remote changes), making localization workflows familiar to developers.

- `kapi serve` becomes a project dashboard showing local + remote state, not just a generic web UI.

- The `.kapi/` directory structure is version-control friendly — configuration is checked in, state is gitignored.

- Projects can exist without server connection (pure local file processing) or with connection (bidirectional sync with Bowrain).

- The project model positions Kapi as **the file connector** for Bowrain Server — it handles files, the server handles integrations with CMS, design tools, etc. ([ADR-005](./005-connector-system.md)).
