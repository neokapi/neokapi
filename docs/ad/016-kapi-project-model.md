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

1. **`config.yaml`** — Project defaults, server connection (compound URL), content entries, hooks, and automations
2. **`flows/`** — YAML definitions of file processing flows
3. **`.sync-cache`** — JSON cache tracking last known server state (block hashes, sync cursor, server metadata) — gitignored
4. **`.gitignore`** — Excludes `.sync-cache` from version control

The project model types live in `platform/project/` — shared infrastructure with no CLI or bowrain module dependency.

### `config.yaml` Schema

The config file uses flat YAML with a top-level `version: v1` field. Because `config.yaml` is a well-known file at a well-known path (`.bowrain/config.yaml`), it does not need a k8s-style envelope — the `kind` is always implicit. The version is part of the spec, not a wrapper around it.

```yaml
version: v1

url: https://bowrain.example.com/my-team/abc123

# Default: auto-detect based on git branch, CI environment, or other heuristics
# stream: $auto

defaults:
  source_language: en-US
  target_languages:
    - fr-FR
    - de-DE
  collection: ui/strings

content:
  - path: "src/**/*.json"
    format: json
    dest: "locales/{lang}/*.json"

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

The `Config` struct defines: schema version (`version`), compound server URL (`url`), stream name (`stream`), project defaults (`source_language`, `target_languages`, `collection`), content entries (`path`, `dest`, `format`, `base`, `collection`, `language`, `target_languages`), plugin list, registries, framework preset, format presets, exclude patterns, hooks, per-flow configuration overrides, and automation rules.

#### Compound URL

The `url` field encodes the server, workspace, and project ID in a single URL:

- `https://bowrain.example.com/my-team/abc123` — workspace project
- `https://bowrain.example.com/projects/abc123` — direct project (no workspace)

Accessor methods (`ServerURL()`, `ProjectID()`, `Workspace()`, `HasServer()`) parse the URL on demand.

Claim tokens for anonymous projects are stored in `.sync-cache` (gitignored), not in `config.yaml`, to avoid accidentally committing credentials to version control. The `bowrain auth claim` command reads the token from `.sync-cache` or accepts it as a CLI argument.

#### Multi-Source-Language Projects

Content entries can override the project's default source language with the `language` field:

```yaml
defaults:
  source_language: en

content:
  - path: src/en/**/*.json
    format: json
    # Uses default: en

  - path: src/es/**/*.json
    format: json
    language: es    # This content is in Spanish
```

The `EffectiveLanguage()` method resolves per-entry language, falling back to the project default. All `\{lang\}` placeholder expansion uses this method.

#### Dynamic Target Languages

When `defaults.target_languages` is empty, the CLI fetches target locales from the server during sync. The resolution order is:

1. CLI flags (`--locales fr,de`)
2. `defaults.target_languages` in config
3. Server-side target locales (cached in `.sync-cache`)

Server metadata is fetched via `GET /projects/:id` and cached locally to avoid redundant API calls.

#### Collections

Collections organize content on the server. Resolved per content entry with fallback to `defaults.collection`. Sent with each block during push.

### Content Entries

Content entries connect local files to remote project items using glob patterns with `\{lang\}` placeholders, format IDs from the FormatRegistry, optional dest patterns for target file layout, base paths for prefix stripping, and per-entry language and collection overrides.

### Sync Cache (`.sync-cache`)

A lightweight JSON cache of the last known server state, enabling incremental sync via a monotonic sync cursor, per-item block hash maps, and cached server metadata (target locales). Gitignored, deletable, and regenerable from the server.

See [Bowrain Sync Protocol](/docs/notes/kapi-sync-protocol) for the full config.yaml schema, sync-cache JSON format, and content entry details.

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
bowrain run pseudo
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
4. Create `.bowrain/config.yaml` with project settings (URL encodes server/workspace/project)
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

# Run a tool directly
bowrain pseudo-translate

# Run a composed flow
bowrain run pseudo

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

Bowrain CLI uses the same content-addressable hashing system as the ContentStore ([AD-003](./003-content-store.md)). Block IDs are scoped to items (files), enabling per-file incremental sync. Push and pull use cursor-based algorithms with batching at 1000 blocks per request. Pull supports multi-locale fetching with dynamic locale resolution from server metadata.

### Server API Integration

When `url` is configured, Bowrain CLI uses workspace-scoped sync endpoints on the Bowrain Server REST API ([AD-013](./013-cli-and-server.md)). The server maintains an append-only change log with monotonic sequence numbers for O(changes) cursor-based queries. Sync routes accept JWT authentication or `ClaimToken` for anonymous projects.

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

- **Separate server/workspace/project fields**: The compound URL (`url: https://server.com/workspace/project`) is more ergonomic than three separate fields. Users can copy-paste URLs from their browser. The URL is parsed on demand by accessor methods.

- **Separate endpoint for fetching available languages**: Reusing `GET /projects/:id` for metadata avoids endpoint proliferation. The response already includes `target_locales`. Caching in `.sync-cache` avoids redundant API calls.

## Consequences

- All Bowrain CLI operations require a `.bowrain/` project directory. This enforces clean project structure and enables stateful operations.

- `bowrain init` is the entry point for new projects, analogous to `git init`.

- `.bowrain/config.yaml` is the single source of truth for project configuration. Checked into git, enabling team collaboration. Uses flat YAML with `version: v1`.

- The compound `url` field encodes server, workspace, and project ID. Accessor methods parse on demand.

- `defaults` section provides project-wide language and collection settings. Content entries can override these per-entry.

- Content entries with `language` override enable multi-source-language projects.

- Dynamic target languages: when `defaults.target_languages` is empty, the CLI fetches and caches server-side target locales.

- Collections are sent with each block during push, resolved per content entry with fallback to `defaults.collection`.

- `.bowrain/.sync-cache` tracks the last known server state locally (block hashes + sync cursor + cached server metadata). Gitignored, regenerable from the server.

- Content entries connect local paths to remote items, enabling bidirectional sync with Bowrain Server.

- Flows are defined in `.bowrain/flows/*.yaml` — project-specific processing chains that use the same tool system as the server pipeline but operate on files.

- Content hashing enables efficient incremental sync — only changed blocks are transferred, even for large projects.

- `bowrain pull/push` commands mirror git's mental model (fetch/push remote changes), making localization workflows familiar to developers.

- `bowrain serve` becomes a project dashboard showing local + remote state.

- The `.bowrain/` directory structure is version-control friendly — configuration is checked in, state is gitignored.

- Projects can exist without server connection (pure local file processing) or with connection (bidirectional sync with Bowrain).

- The project model positions Bowrain CLI as **the file connector** for Bowrain Server — it handles files, the server handles integrations with CMS, design tools, etc. ([AD-005](./005-connector-system.md)).

- Kapi remains a standalone tool without a project requirement, while Bowrain CLI owns the project-based workflow.
