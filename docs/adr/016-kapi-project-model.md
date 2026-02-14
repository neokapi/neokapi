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
│   └── .state.json      # Sync state (gitignored)
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
3. **`.state.json`** — Sync state tracking (content hashes, timestamps) — gitignored

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

### Sync State (`.state.json`)

The `.state.json` file tracks sync state between local files and Bowrain Server:

```json
{
  "last_pull": "2026-02-15T10:30:00Z",
  "last_push": "2026-02-15T09:00:00Z",
  "files": {
    "src/locales/en-US.json": {
      "content_hash": "abc123...",
      "modified": "2026-02-15T09:00:00Z",
      "remote_hash": "abc123...",
      "remote_modified": "2026-02-14T16:00:00Z"
    }
  },
  "remote_items": {
    "ui/strings/auth/login": {
      "content_hash": "def456...",
      "modified": "2026-02-15T10:00:00Z",
      "local_path": "src/locales/auth/login.json"
    }
  }
}
```

**Purpose:**
- Detect local changes (compare file mtime + hash vs `.state.json`)
- Detect remote changes (compare server hashes vs `.state.json`)
- Enable `kapi status` (show modified/new/deleted files)
- Support incremental sync (only transfer changed content)

**This file is gitignored** — it contains local state, not project configuration.

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
5. Create `.gitignore` entry for `.kapi/.state.json`

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
4. **Compare hashes** with `.state.json`
5. **Sync only changed blocks**

This is identical to how the ContentStore works — Kapi and Bowrain Server share the same hashing algorithm, enabling efficient delta sync without transferring unchanged content.

**3-way sync algorithm:**

```
Base (last sync):  .state.json
Local (current):   file mtime + content hash
Remote (server):   GET /api/v1/projects/:id/sync-state

Changes:
- Local modified + remote unchanged → push candidate
- Remote modified + local unchanged → pull candidate
- Both modified → conflict (requires resolution)
```

### Server API Integration

When `server.url` is configured, Kapi uses the Bowrain Server REST API ([ADR-013](./013-cli-and-server.md)):

**Pull workflow:**
```
1. GET /api/v1/workspaces/:ws/projects/:id/sync-state
   → Returns: map of remote item ID → content hash

2. Compare with .state.json → identify changed items

3. GET /api/v1/workspaces/:ws/projects/:id/blocks?items=<ids>
   → Returns: only changed blocks

4. Write blocks to local files via FormatRegistry

5. Update .state.json with new hashes
```

**Push workflow:**
```
1. Read local files via FormatRegistry
2. Compute block hashes
3. Compare with .state.json → identify changed blocks
4. Run pre-push hooks (if configured)
5. POST /api/v1/workspaces/:ws/projects/:id/push
   → Request body: changed blocks + item mappings
6. Update .state.json
```

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

- `.kapi/.state.json` tracks sync state locally. Gitignored, analogous to `.git/index`.

- File mappings connect local paths to remote items, enabling bidirectional sync with Bowrain Server.

- Flows are defined in `.kapi/flows/*.yaml` — project-specific processing chains that use the same tool system as Bowrain but operate on files.

- Content hashing enables efficient incremental sync — only changed blocks are transferred, even for large projects.

- `kapi pull/push` commands mirror git's mental model (fetch/push remote changes), making localization workflows familiar to developers.

- `kapi serve` becomes a project dashboard showing local + remote state, not just a generic web UI.

- The `.kapi/` directory structure is version-control friendly — configuration is checked in, state is gitignored.

- Projects can exist without server connection (pure local file processing) or with connection (bidirectional sync with Bowrain).

- The project model positions Kapi as **the file connector** for Bowrain Server — it handles files, the server handles integrations with CMS, design tools, etc. ([ADR-005](./005-connector-system.md)).
