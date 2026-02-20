---
id: 016-kapi-project-model
sidebar_position: 16
title: "AD-016: Kapi Project Model"
---
# AD-016: Kapi Project Model

## Context

Kapi is the command-line swiss army knife for file-based localization workflows. It must operate effectively both standalone (local file processing) and connected (syncing with Bowrain Server). Traditional localization tools treat each file operation as isolated — there is no persistent project context, no history of what was synced, and no declarative configuration.

Modern developer tools solve this with **project directories**: `.git` for version control, `.terraform` for infrastructure, `package.json` for dependencies. The same pattern applies to localization — a `.kapi/` directory that captures project identity, file mappings, sync state, and workflow definitions.

This AD establishes the `.kapi/` project model as the foundation of Kapi's architecture. Everything in Kapi operates within a project context. There are no standalone file operations — all commands require a `.kapi/` directory.

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

The config file defines project identity (name, source locale, target locales), optional server connection (URL, project ID, workspace), file mappings (local glob patterns to remote item paths with format IDs), hooks (pre-push, post-pull), and per-flow configuration overrides.

### File Mappings

Mappings connect local files to remote project items using glob patterns, path templates (`{path}`, `{filename}`, `{basename}`), and format IDs from the FormatRegistry.

### Sync Cache (`.sync-cache`)

A lightweight JSON cache of the last known server state, enabling incremental sync via a monotonic sync cursor and per-item block hash maps. Gitignored, deletable, and regenerable from the server.

See [Kapi Sync Protocol](/docs/notes/kapi-sync-protocol) for the full config.yaml schema, sync-cache JSON format, and file mapping details.

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

Flows use the same tool system as Bowrain ([AD-006](./006-tool-system.md)) but operate on local files instead of streaming through the server pipeline.

### Project Initialization

The `kapi init` command creates a new project:

```bash
# Interactive mode (recommended)
kapi init

# Non-interactive: local project
kapi init --name "My App" --source en-US --targets fr-FR,de-DE

# Non-interactive: anonymous project
kapi init --anonymous --name "My App" --source en

# Non-interactive: connect to existing project
kapi init --server https://bowrain.example.com --project my-app-l10n
```

**Interactive `kapi init` workflow:**
1. Check if `.kapi/` already exists (error if so)
2. If already authenticated:
   - Prompt for workspace selection (or create a new workspace)
   - Prompt for project name and source locale (BCP-47 selector)
   - Create project on server in the selected workspace
3. If not authenticated, offer four paths:
   - **Sign in**: OAuth device flow → workspace selection → project details
   - **Email claim**: project details → email → anonymous project with email claim
   - **Anonymous**: project details → anonymous project (prints claim URL)
   - **Local only**: project details → local-only project
4. Create `.kapi/config.yaml` with project settings (including `server.workspace` if applicable)
5. Create `.kapi/flows/` directory with example flows
6. Create `.kapi/.gitignore` to exclude `.sync-cache`

**Non-interactive `kapi init` workflow:**
1. Check if `.kapi/` already exists (error if so)
2. Create `.kapi/config.yaml` from flag values
3. If `--anonymous` or `--email`: create anonymous project on server
4. If `--project`: verify auth and connect to existing project
5. If authenticated with no flags: create project in personal workspace
6. Create `.kapi/flows/` directory with example flows
7. Create `.kapi/.gitignore`

All paths support `--json` output for CI/CD integration.

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

### Content Hashing and Sync

Kapi uses the same content-addressable hashing system as the ContentStore ([AD-003](./003-content-store.md)). Block IDs are scoped to items (files), enabling per-file incremental sync. Push and pull use cursor-based algorithms with batching at 1000 blocks per request. Pull supports multi-locale fetching.

### Server API Integration

When `server.url` is configured, Kapi uses workspace-scoped sync endpoints on the Bowrain Server REST API ([AD-013](./013-cli-and-server.md)). The server maintains an append-only change log with monotonic sequence numbers for O(changes) cursor-based queries. Sync routes accept JWT authentication or `ClaimToken` for anonymous projects.

See [Kapi Sync Protocol](/docs/notes/kapi-sync-protocol) for push/pull algorithms, server API endpoints, and change log details.

## Alternatives Considered

- **Global config file** (`~/.kapi/config.yaml`): Does not support multiple projects with different settings. Modern dev tools use per-project directories (`.git`, `.terraform`) for good reason.

- **Single `kapi.yaml` file** (like `package.json`): Works for simple cases but becomes cluttered when adding flow definitions, mappings, and hooks. Separate `flows/` directory keeps configuration organized.

- **Archive files as the project format**: Snapshot archives lack version control integration, live editing, and incremental sync. `.kapi/` directories integrate naturally with git.

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

- The project model positions Kapi as **the file connector** for Bowrain Server — it handles files, the server handles integrations with CMS, design tools, etc. ([AD-005](./005-connector-system.md)).
