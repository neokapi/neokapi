---
id: 016-kapi-project-model
sidebar_position: 16
title: "AD-016: Bowrain Project Model"
---
# AD-016: Bowrain Project Model

## Context

The Bowrain CLI is the project sync companion for Bowrain Server. It manages bidirectional sync between local files and the server, runs flows within a project context, and tracks sync state. Modern developer tools use **project directories** — `.git` for version control, `.terraform` for infrastructure — to provide persistent project context, history, and declarative configuration.

The `.bowrain/` directory captures project identity, file mappings, sync state, and workflow definitions. All Bowrain CLI commands require a `.bowrain/` project directory. Kapi, in contrast, is a standalone file-processing tool that operates on files directly without a project directory.

## Decision

### `.bowrain/` Directory Structure

Every Bowrain project is a directory containing a `.bowrain/` subdirectory:

```
my-app/
├── .bowrain/
│   ├── config.yaml      # Project configuration
│   ├── flows/           # Flow definitions (YAML)
│   │   └── pseudo.yaml
│   ├── .sync-cache      # Sync cache (gitignored)
│   └── .gitignore       # Excludes .sync-cache
├── src/
│   └── locales/
│       ├── en-US.json
│       ├── fr-FR.json
│       └── de-DE.json
```

The `.bowrain/` directory is created by `bowrain init` and contains:

1. **`config.yaml`** — Project metadata, server connection, file mappings, hooks, and automations
2. **`flows/`** — YAML definitions of file processing flows
3. **`.sync-cache`** — JSON cache tracking last known server state (block hashes, sync cursor) — gitignored
4. **`.gitignore`** — Excludes `.sync-cache` from version control

The project model types live in `platform/project/` — shared infrastructure with no CLI or bowrain module dependency.

### `config.yaml` Schema

The config file supports two formats: an **envelope format** (with `apiVersion` and `kind` fields, used by default) and **bare YAML** (backward compatible, transparently migrated on load).

**Envelope format:**
```yaml
apiVersion: v1
kind: ProjectConfig
metadata:
  name: my-app
spec:
  project:
    name: My App
    source_locale: en-US
    target_locales:
      - fr-FR
      - de-DE
  server:
    url: https://bowrain.example.com
    project_id: abc123
    workspace: default
  mappings:
    - local: "src/**/*.json"
      remote: "app/{path}"
      format: json
      target_path: "locales/{locale}.json"
  hooks:
    pre-push:
      - qa-check
  automations:
    - name: "qa-before-push"
      trigger: "pre-push"
      actions:
        - type: "run_flow"
          config:
            flow: "qa-check"
```

The `Config` struct defines: project identity (`name`, `source_locale`, `target_locales`), optional server connection (`url`, `project_id`, `workspace`, `claim_token`), plugin configuration and registries, framework preset, format presets, file mappings, exclude patterns, hooks, per-flow configuration overrides, and automation rules.

### File Mappings

Mappings connect local files to remote project items using glob patterns, path templates (`{path}`, `{filename}`, `{basename}`), format IDs from the FormatRegistry, and optional target path templates with `{locale}` expansion.

### Sync Cache (`.sync-cache`)

A lightweight JSON cache of the last known server state, enabling incremental sync via a monotonic sync cursor and per-item block hash maps. Gitignored, deletable, and regenerable from the server.

See [Bowrain Sync Protocol](/docs/notes/kapi-sync-protocol) for the full config.yaml schema, sync-cache JSON format, and file mapping details.

### Flow Definitions

Flows are file processing chains defined in `.bowrain/flows/*.yaml`:

```yaml
# .bowrain/flows/pseudo.yaml
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
bowrain flow run pseudo
```

Flows use the same tool system as the server pipeline ([AD-006](./006-tool-system.md)) but operate on local files.

### Project Initialization

The `bowrain init` command creates a new project:

```bash
# Interactive mode (recommended)
bowrain init

# Non-interactive: local project
bowrain init --name "My App" --source en-US --targets fr-FR,de-DE

# Non-interactive: anonymous project
bowrain init --anonymous --name "My App" --source en

# Non-interactive: connect to existing project
bowrain init --server https://bowrain.example.com --project my-app-l10n
```

**Interactive `bowrain init` workflow:**
1. Check if `.bowrain/` already exists (error if so)
2. If already authenticated:
   - Prompt for workspace selection (or create a new workspace)
   - Prompt for project name and source locale (BCP-47 selector)
   - Create project on server in the selected workspace
3. If not authenticated, offer four paths:
   - **Sign in**: OAuth device flow → workspace selection → project details
   - **Email claim**: project details → email → anonymous project with email claim
   - **Anonymous**: project details → anonymous project (prints claim URL)
   - **Local only**: project details → local-only project
4. Create `.bowrain/config.yaml` with project settings (including `server.workspace` if applicable)
5. Create `.bowrain/flows/` directory with example flows
6. Create `.bowrain/.gitignore` to exclude `.sync-cache`

All paths support `--json` output for CI/CD integration.

### Project Commands

All Bowrain CLI commands operate within a project context:

```bash
cd my-app/

# Show sync status
bowrain status
# → Modified: src/locales/en-US.json
# → Remote changes: 2 blocks updated
# → Conflicts: 0

# Pull from Bowrain Server
bowrain pull
# → Fetching remote changes...
# → Updated: src/locales/fr-FR.json (5 blocks)
# → Updated: src/locales/de-DE.json (3 blocks)

# Push to Bowrain Server
bowrain push --message "Update source strings"
# → Running pre-push hooks: qa-check, term-enforce
# → Pushing 2 changed files...
# → Pushed: src/locales/en-US.json (12 blocks)

# Run a flow
bowrain flow run pseudo

# List project contents
bowrain ls

# Add / remove files
bowrain add src/locales/*.json
bowrain rm src/locales/old.json

# Manage config
bowrain config

# Start local dashboard
bowrain serve
# → Dashboard running at http://localhost:3000
```

Commands automatically discover the `.bowrain/` directory by searching upward from the current directory (like git). The `FindProject` function in `platform/project/` implements this traversal.

### Content Hashing and Sync

Bowrain CLI uses the same content-addressable hashing system as the ContentStore ([AD-003](./003-content-store.md)). Block IDs are scoped to items (files), enabling per-file incremental sync. Push and pull use cursor-based algorithms with batching at 1000 blocks per request. Pull supports multi-locale fetching.

### Server API Integration

When `server.url` is configured, Bowrain CLI uses workspace-scoped sync endpoints on the Bowrain Server REST API ([AD-013](./013-cli-and-server.md)). The server maintains an append-only change log with monotonic sequence numbers for O(changes) cursor-based queries. Sync routes accept JWT authentication or `ClaimToken` for anonymous projects.

See [Bowrain Sync Protocol](/docs/notes/kapi-sync-protocol) for push/pull algorithms, server API endpoints, and change log details.

### Automations

Project-level automation rules can be defined in `config.yaml`:

```yaml
automations:
  - name: "qa-before-push"
    trigger: "pre-push"
    enabled: true
    actions:
      - type: "run_flow"
        config:
          flow: "qa-check"
      - type: "run_flow"
        config:
          flow: "term-enforce"
```

Triggers include `pre-push`, `post-push`, `pre-pull`, `post-pull`, `pre-flow`, and `post-flow`. This is distinct from server-side automation ([AD-011](./011-automation.md)), which orchestrates multi-step workflows across connectors and quality gates.

## Alternatives Considered

- **Global config file** (`~/.bowrain/config.yaml`): Does not support multiple projects with different settings. Modern dev tools use per-project directories (`.git`, `.terraform`) for good reason.

- **Single config file** (like `package.json`): Works for simple cases but becomes cluttered when adding flow definitions, mappings, and hooks. Separate `flows/` directory keeps configuration organized.

- **Archive files as the project format**: Snapshot archives lack version control integration, live editing, and incremental sync. `.bowrain/` directories integrate naturally with git.

- **Store-based local projects**: Using SQLite ContentStore for local work adds unnecessary complexity. Files are the native format — direct file editing is faster and more transparent than store operations.

- **No project requirement** (standalone file commands): Leads to inconsistent state, no sync history, and manual path management. Requiring projects enforces clean workflows and enables powerful features (status, diff, incremental sync). Standalone file processing is Kapi's role.

## Consequences

- All Bowrain CLI operations require a `.bowrain/` project directory. This enforces clean project structure and enables stateful operations.

- `bowrain init` is the entry point for new projects, analogous to `git init`.

- `.bowrain/config.yaml` is the single source of truth for project configuration. Checked into git, enabling team collaboration. Supports envelope format with migrations.

- `.bowrain/.sync-cache` tracks the last known server state locally (block hashes + sync cursor). Gitignored, regenerable from the server.

- File mappings connect local paths to remote items, enabling bidirectional sync with Bowrain Server.

- Flows are defined in `.bowrain/flows/*.yaml` — project-specific processing chains that use the same tool system as the server pipeline but operate on files.

- Content hashing enables efficient incremental sync — only changed blocks are transferred, even for large projects.

- `bowrain pull/push` commands mirror git's mental model (fetch/push remote changes), making localization workflows familiar to developers.

- `bowrain serve` becomes a project dashboard showing local + remote state.

- The `.bowrain/` directory structure is version-control friendly — configuration is checked in, state is gitignored.

- Projects can exist without server connection (pure local file processing) or with connection (bidirectional sync with Bowrain).

- The project model positions Bowrain CLI as **the file connector** for Bowrain Server — it handles files, the server handles integrations with CMS, design tools, etc. ([AD-005](./005-connector-system.md)).

- Kapi remains a standalone tool without a project requirement, while Bowrain CLI owns the project-based workflow.
