---
id: 013-cli-and-server
sidebar_position: 13
title: "ADR-013: Kapi CLI and Bowrain Server"
---
# ADR-013: Kapi CLI and Bowrain Server

## Context

gokapi exposes its functionality through two application entry points:
- **Kapi CLI** — Command-line tool for file-based localization workflows
- **Bowrain Server** — Multi-user platform for team collaboration and integrations

The CLI must reflect the **project-based architecture** ([ADR-016](./016-kapi-project-model.md)). All commands operate within a `.kapi/` project directory. The server provides both REST (for external integrations and webhooks) and gRPC (for Bowrain desktop app streaming) APIs.

**This ADR establishes the role separation:**
- **Kapi** = local file tool, git-like project model, can sync with Bowrain Server
- **Bowrain Server** = multi-user platform, integration connectors, automation, ContentStore

## Decision

### Kapi CLI: Project-Based Commands

The CLI uses [Cobra](https://github.com/spf13/cobra) for hierarchical subcommands. **All commands require a `.kapi/` project directory** (discovered by searching upward from the current directory, like git).

```
kapi
├── init             # Initialize a new .kapi/ project
│   └── --server URL --project ID (optional server connection)
├── status           # Show sync state (local vs remote)
├── diff             # Show changes between local and remote
├── pull             # Pull from Bowrain Server → update local files
│   └── --force, --dry-run, [paths...]
├── push             # Push local files → update Bowrain Server
│   └── --force, --dry-run, --message MSG, [paths...]
├── flow             # Flow management
│   ├── run FLOW     # Execute a flow from .kapi/flows/
│   └── list         # List available flows
├── serve            # Start local dashboard (web UI)
│   └── --port 3000
├── auth             # Authentication with Bowrain Server
│   ├── login        # OAuth device flow login
│   ├── logout       # Remove stored token
│   └── status       # Show current user, server URL
├── termbase         # Terminology management
│   ├── import       # Import CSV/TBX/JSON
│   ├── export       # Export
│   ├── lookup       # Query terms
│   ├── search       # Search concepts
│   └── stats        # Statistics
├── formats          # Format listing
│   └── list         # List available formats (built-in + plugin)
├── tools            # Tool listing
│   └── list         # List available tools
└── plugins          # Plugin management
    ├── install      # Install a plugin
    ├── list         # List installed plugins
    ├── update       # Update plugins
    └── search       # Search plugin registry
```

### Removed Commands

The following commands are **removed** as they do not fit the new architecture:

**❌ `kapi store`** — ContentStore is server-side ([ADR-003](./003-content-store.md))
- `store version` → server-side versioning
- `store projects` → `kapi status` shows current project
- `store export/import` → KAZ files are server-side

**❌ `kapi connect`** — Connector management is server-side ([ADR-005](./005-connector-system.md))
- `connect add` → Bowrain Server admin UI
- `connect list` → server-side connectors

**❌ Standalone file operations** — All operations require `.kapi/` project
- No more `kapi convert input.html output.xliff` without project context

### Core Commands

#### **`kapi init` — Initialize Project**

Creates a new `.kapi/` project directory:

```bash
# Standalone project (no server)
cd my-app/
kapi init

# Connected to Bowrain Server
kapi init --server https://bowrain.example.com --project my-app-l10n
```

**Workflow:**
1. Check if `.kapi/` already exists (error if so)
2. Create `.kapi/config.yaml` with defaults
3. If `--server` provided:
   - Verify auth token exists (`kapi auth status`)
   - Fetch project metadata from server
   - Populate `server.url` and `server.project_id`
4. Create `.kapi/flows/` directory with example flows
5. Create `.gitignore` entry for `.kapi/.state.json`

#### **`kapi status` — Show Sync State**

Displays the sync state between local files and Bowrain Server (if configured):

```bash
kapi status
# → Modified: src/locales/en-US.json
# → Remote changes: 2 blocks updated
# → Conflicts: 0
# → Last pull: 2026-02-15 10:30:00
# → Last push: 2026-02-15 09:00:00
```

**Algorithm:**
1. Read `.kapi/.state.json` (last sync state)
2. Scan local files (compute content hashes)
3. Compare with `.state.json` → identify local changes
4. If server configured:
   - Call `GET /api/v1/workspaces/:ws/projects/:id/sync-state`
   - Compare with `.state.json` → identify remote changes
5. Detect conflicts (both local and remote changed)

#### **`kapi diff` — Show Changes**

Shows differences between local files and remote project:

```bash
kapi diff
# → src/locales/en-US.json:
# →   Block abc123 (local): "Hello, world!"
# →   Block abc123 (remote): "Hello, World!"
# →
# → src/locales/fr-FR.json:
# →   Block def456 (remote only): "Bonjour"
```

#### **`kapi pull` — Fetch from Server**

Pulls changes from Bowrain Server and updates local files:

```bash
kapi pull                           # Pull all changes
kapi pull src/locales/*.json        # Pull specific paths
kapi pull --force                   # Overwrite local changes
kapi pull --dry-run                 # Show what would be pulled
```

**Workflow:**
1. Read `.kapi/config.yaml` (server URL, project ID, mappings)
2. Verify auth token
3. Call `POST /api/v1/workspaces/:ws/projects/:id/pull`
   - Request body: local state (hashes, timestamps)
   - Response: only changed blocks
4. Write blocks to local files via FormatRegistry
5. Run `post-pull` hooks (if configured)
6. Update `.kapi/.state.json`

**Conflict handling:**
- By default, pull fails if local files have uncommitted changes
- `--force` overwrites local changes
- Future: `--merge` attempts 3-way merge

#### **`kapi push` — Send to Server**

Pushes local file changes to Bowrain Server:

```bash
kapi push                            # Push all changes
kapi push src/locales/*.json         # Push specific paths
kapi push --message "Update strings" # Add commit message
kapi push --force                    # Bypass quality gates
kapi push --dry-run                  # Show what would be pushed
kapi push --no-hooks                 # Skip pre-push hooks
```

**Workflow:**
1. Read `.kapi/config.yaml` (mappings)
2. Run `pre-push` hooks (qa-check, term-enforce, etc.)
   - If any hook fails, abort push
3. Read local files via FormatRegistry
4. Compute block hashes
5. Compare with `.kapi/.state.json` → identify changed blocks
6. Verify auth token
7. Call `POST /api/v1/workspaces/:ws/projects/:id/push`
   - Request body: changed blocks + item mappings + message
   - Server may reject if quality gates fail
8. Update `.kapi/.state.json`

#### **`kapi flow` — Run Flows**

Executes flows defined in `.kapi/flows/*.yaml`:

```bash
kapi flow run extract               # Run extract flow
kapi flow run pseudo                # Run pseudo-translate flow
kapi flow list                      # List available flows
```

**Flow execution:**
1. Load flow definition from `.kapi/flows/<flow-name>.yaml`
2. Execute each step sequentially
3. Each step runs a tool on local files
4. Output is written to files or piped to next step

**Example flow definition:**

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

#### **`kapi serve` — Local Dashboard**

Starts a web UI dashboard for the current project:

```bash
kapi serve                   # Start on port 3000
kapi serve --port 4000       # Custom port
kapi serve --no-open         # Don't auto-open browser
```

**Dashboard features:**
- **Project overview**: File count, locale coverage, last modified
- **Sync status**: Local vs remote diff, conflicts, pending changes
- **Flow execution**: Trigger flows from UI, view execution logs
- **Remote project stats** (if connected): Server project state, translation progress
- **Config editor**: Edit `.kapi/config.yaml` through UI

**Implementation:**
- Uses the same React web UI as Bowrain desktop app
- Embedded in Kapi binary (via embed.FS)
- Runs local HTTP server on `localhost` only
- No authentication (local-only tool)
- Auto-opens browser on start

### Authentication

The `kapi auth` command group enables CLI users to authenticate against a Bowrain Server instance using OAuth 2.0 Device Authorization Grant (RFC 8628) via Dex ([ADR-015](./015-auth-and-workspaces.md)):

```bash
# Login to Bowrain Server
kapi auth login --server https://bowrain.example.com
# → Open https://bowrain.example.com/auth/device and enter code: ABCD-1234
# → Polling for authorization...
# → Logged in as user@example.com

# Check auth status
kapi auth status
# → Server: https://bowrain.example.com
# → User: user@example.com (ID: abc123)
# → Token expires: 2026-02-16 14:30:00

# Logout
kapi auth logout
# → Logged out from https://bowrain.example.com
```

The token is stored at `~/.config/gokapi/auth.json` and automatically attached to API requests made by `kapi pull`, `kapi push`, etc.

### Bowrain Server

The server binary (`cmd/bowrain-server/`) provides remote access via two protocols. Server logic lives in `internal/server/` and supports two deployment modes:

**Deployment modes:**
1. **Multi-user mode** (bowrain-server) — Full authentication, workspaces, binds to `0.0.0.0`
2. **Local mode** (kapi serve) — No authentication, single project, binds to `localhost`

**REST API** (Echo v4) serves external integrations, webhooks, and HTTP clients:

```
# Public routes
GET    /api/v1/health
GET    /api/v1/config                               # Server mode (local/server)

# Auth routes (multi-user mode only)
POST   /api/v1/auth/device/start                    # Start device auth flow
POST   /api/v1/auth/device/poll                     # Poll for token
GET    /api/v1/auth/callback                        # OIDC redirect callback
GET    /api/v1/auth/me                              # Get current user
POST   /api/v1/auth/logout                          # Revoke token

# Workspace routes (require auth)
POST   /api/v1/workspaces
GET    /api/v1/workspaces
GET    /api/v1/workspaces/:ws
PUT    /api/v1/workspaces/:ws
DELETE /api/v1/workspaces/:ws
GET    /api/v1/workspaces/:ws/members
POST   /api/v1/workspaces/:ws/members
PUT    /api/v1/workspaces/:ws/members/:uid/role
DELETE /api/v1/workspaces/:ws/members/:uid

# Project routes (scoped to workspace)
POST   /api/v1/workspaces/:ws/projects
GET    /api/v1/workspaces/:ws/projects
GET    /api/v1/workspaces/:ws/projects/:id
PUT    /api/v1/workspaces/:ws/projects/:id
DELETE /api/v1/workspaces/:ws/projects/:id

# Content routes
GET    /api/v1/workspaces/:ws/projects/:id/blocks
PUT    /api/v1/workspaces/:ws/projects/:id/blocks/:hash/translation

# Sync routes (for Kapi pull/push)
GET    /api/v1/workspaces/:ws/projects/:id/sync-state
POST   /api/v1/workspaces/:ws/projects/:id/pull
POST   /api/v1/workspaces/:ws/projects/:id/push

# KAZ export/import
GET    /api/v1/workspaces/:ws/projects/:id/export
POST   /api/v1/workspaces/:ws/projects/import

# Connector management (server-side only)
GET    /api/v1/workspaces/:ws/connectors
POST   /api/v1/workspaces/:ws/connectors
GET    /api/v1/workspaces/:ws/connectors/:id/sync-status
POST   /api/v1/workspaces/:ws/connectors/:id/pull
POST   /api/v1/workspaces/:ws/connectors/:id/push

# Flow execution (server-side)
POST   /api/v1/workspaces/:ws/projects/:id/flows/run
GET    /api/v1/workspaces/:ws/flows
```

**gRPC** serves Bowrain desktop app streaming communication:

```protobuf
service GokapiService {
    rpc CreateProject(CreateProjectRequest) returns (Project);
    rpc StreamBlocks(StreamBlocksRequest) returns (stream Block);
    rpc UpdateTranslation(UpdateTranslationRequest) returns (Block);
    rpc ExecuteFlow(ExecuteFlowRequest) returns (stream FlowProgress);
    rpc Subscribe(SubscribeRequest) returns (stream Event);
}
```

gRPC streaming enables real-time flow progress, block updates, and event subscriptions for the Bowrain desktop app ([ADR-012](./012-bowrain.md)).

### CI/CD Integration

The CLI is designed for non-interactive use in CI/CD pipelines:

```yaml
# GitHub Actions example
- name: Authenticate with Bowrain Server
  run: kapi auth login --server ${{ secrets.BOWRAIN_SERVER }}
  env:
    KAPI_AUTH_TOKEN: ${{ secrets.BOWRAIN_TOKEN }}

- name: Pull latest translations
  run: kapi pull

- name: Run pseudo-translation flow
  run: kapi flow run pseudo

- name: Push changes if tests pass
  run: kapi push --message "CI: Update translations"
```

**CI-friendly features:**
- `--json` flag for machine-readable output
- Exit codes: 0 (success), 1 (error), 2 (conflict)
- `KAPI_AUTH_TOKEN` environment variable (bypass device flow)
- `--no-hooks` flag to skip interactive hooks

## Alternatives Considered

- **REST only**: Simpler but no streaming for real-time progress and events. Bowrain would need polling, degrading the editing experience.

- **gRPC only**: Efficient and type-safe but harder for simple integrations, webhooks, and `curl`-based debugging.

- **GraphQL**: Flexible queries but over-engineered for this use case where the API surface is well-defined and resource-oriented.

- **Store-based CLI** (`kapi store`): Mixes server concerns into the CLI. The ContentStore is a server-side persistence layer; Kapi should only interact via API.

- **Global config** (no project directories): Makes collaboration harder. The `.kapi/` directory model enables team workflows (check config into git, .state.json gitignored) and git-like mental model.

## Consequences

- **Kapi is project-based** — all commands require a `.kapi/` directory, enforcing clean project structure and enabling stateful operations.

- **`kapi init`** is the entry point, analogous to `git init` or `npm init`.

- **`kapi pull/push`** mirror git's fetch/push mental model, making localization workflows familiar to developers.

- **`kapi status/diff`** show sync state without modifying files, enabling safe inspection before pull/push.

- **Flow system** is simplified — flows run on local files, defined in `.kapi/flows/*.yaml`, no server interaction.

- **`kapi serve`** becomes a project dashboard showing local + remote state, not just a generic web UI.

- **Removed commands** (`store`, `connect`) eliminate confusion between CLI and server responsibilities.

- **REST + gRPC dual protocol** serves both external integrations (REST) and Bowrain desktop app real-time requirements (gRPC).

- **OAuth device flow authentication** (`kapi auth`) makes CI/CD integration seamless with stored tokens.

- **Workspace-scoped API routes** enforce multi-tenancy at the HTTP layer, complementing the domain-level workspace model ([ADR-015](./015-auth-and-workspaces.md)).

- **Clear role separation**: Kapi = local file tool, Bowrain Server = platform for integrations and collaboration.

- The CLI command structure reflects the new architecture: project-centric, sync-focused, file-based.

- Server logic is shared between `bowrain-server` (multi-user) and `kapi serve` (local) via `ServerConfig.LocalMode` flag.

- Kapi positions itself as **the file connector** for Bowrain Server — it handles files, the server handles integrations ([ADR-005](./005-connector-system.md)).
