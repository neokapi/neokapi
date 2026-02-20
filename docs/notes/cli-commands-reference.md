---
sidebar_position: 7
title: "CLI Commands Reference"
---
# CLI Commands Reference

This note provides implementation details for [AD-013](/docs/ad/013-cli-and-server).

## Kapi Command Tree

```
kapi
+-- init             # Initialize a new .kapi/ project
|   +-- --name, --source, --targets, --server, --project, --anonymous, --email
+-- config           # View or set configuration values
|   +-- --global (use ~/.config/kapi/kapi.yaml instead of .kapi/config.yaml)
+-- ls               # List tracked files with optional stats
|   +-- --stats, --dirty, [paths...]
+-- add              # Add file patterns to track
|   +-- --format, <pattern> [pattern...]
+-- rm               # Stop tracking files (remove mapping or add exclude)
|   +-- <pattern> [pattern...]
+-- status           # Show sync state (local vs remote)
+-- diff             # Show changes between local and remote
+-- pull             # Pull from Bowrain Server -> update local files
|   +-- --force, --dry-run, --locale, [paths...]
+-- push             # Push local files -> update Bowrain Server
|   +-- --force, --dry-run, --message MSG, [paths...]
+-- flow             # Flow management
|   +-- run FLOW     # Execute a flow from .kapi/flows/
|   |   +-- --format, --encoding, --source-lang, --target-lang
|   +-- list         # List available flows
+-- serve            # Start local dashboard (web UI)
|   +-- --port 3000
+-- auth             # Authentication with Bowrain Server
|   +-- login        # OAuth device flow login
|   +-- logout       # Remove stored token
|   +-- status       # Show current user, server URL
+-- termbase         # Terminology management
|   +-- import       # Import CSV/TBX/JSON
|   +-- export       # Export
|   +-- lookup       # Query terms
|   +-- search       # Search concepts
|   +-- stats        # Statistics
+-- formats          # Format listing
|   +-- list         # List available formats (built-in + plugin)
+-- tools            # Tool listing
|   +-- list         # List available tools
+-- plugins          # Plugin management
    +-- install      # Install a plugin
    +-- list         # List installed plugins
    +-- update       # Update plugins
    +-- search       # Search plugin registry
```

## `kapi init` Workflows

### Interactive Mode (default when stdin is a terminal)

- If already signed in: select workspace -> enter project name -> select source locale
- If not signed in: choose from Sign in / Email claim / Anonymous / Local only
- Workspace selector allows choosing existing workspaces or creating new ones
- Source locale uses a BCP-47 selector with type-ahead filtering

### Non-interactive Workflow

1. Check if `.kapi/` already exists (error if so)
2. Create `.kapi/config.yaml` with defaults or provided flags
3. If `--anonymous` or `--email`: create anonymous project on server
4. If `--project` provided: verify auth and connect to existing project
5. If authenticated with no flags: create project in personal workspace
6. Create `.kapi/flows/` directory with example flows
7. Create `.gitignore` entry for `.kapi/.sync-cache`

All paths support `--json` output for CI/CD integration.

## `kapi pull` Algorithm

1. Read `.kapi/config.yaml` (server URL, project ID, mappings)
2. Verify auth token
3. Call `POST /api/v1/workspaces/:ws/projects/:id/pull`
   - Request body: local state (hashes, timestamps)
   - Response: only changed blocks
4. Write blocks to local files via FormatRegistry
5. Run `post-pull` hooks (if configured)
6. Update `.kapi/.sync-cache`

**Conflict handling:**
- By default, pull fails if local files have uncommitted changes
- `--force` overwrites local changes
- Future: `--merge` attempts 3-way merge

## `kapi push` Algorithm

1. Read `.kapi/config.yaml` (mappings)
2. Run `pre-push` hooks (qa-check, term-enforce, etc.)
   - If any hook fails, abort push
3. Read local files via FormatRegistry
4. Compute block hashes
5. Compare with `.kapi/.sync-cache` -> identify changed blocks
6. Verify auth token
7. Call `POST /api/v1/workspaces/:ws/projects/:id/push`
   - Request body: changed blocks + item mappings + message
   - Server may reject if quality gates fail
8. Update `.kapi/.sync-cache`

## REST API Routes

### Public Routes

```
GET    /api/v1/health
GET    /api/v1/config                               # Server mode (local/server)
```

### Auth Routes (multi-user mode only)

```
POST   /api/v1/auth/device/start                    # Start device auth flow
POST   /api/v1/auth/device/poll                     # Poll for token
GET    /api/v1/auth/callback                        # OIDC redirect callback
GET    /api/v1/auth/me                              # Get current user
POST   /api/v1/auth/logout                          # Revoke token
```

### Workspace Routes (require auth)

```
POST   /api/v1/workspaces
GET    /api/v1/workspaces
GET    /api/v1/workspaces/:ws
PUT    /api/v1/workspaces/:ws
DELETE /api/v1/workspaces/:ws
GET    /api/v1/workspaces/:ws/members
POST   /api/v1/workspaces/:ws/members
PUT    /api/v1/workspaces/:ws/members/:uid/role
DELETE /api/v1/workspaces/:ws/members/:uid
```

### Project Routes (scoped to workspace)

```
POST   /api/v1/workspaces/:ws/projects
GET    /api/v1/workspaces/:ws/projects
GET    /api/v1/workspaces/:ws/projects/:id
PUT    /api/v1/workspaces/:ws/projects/:id
DELETE /api/v1/workspaces/:ws/projects/:id
```

### Content Routes

```
GET    /api/v1/workspaces/:ws/projects/:id/blocks
PUT    /api/v1/workspaces/:ws/projects/:id/blocks/:hash/translation
```

### Sync Routes (for Kapi pull/push)

```
GET    /api/v1/workspaces/:ws/projects/:id/sync-state
POST   /api/v1/workspaces/:ws/projects/:id/pull
POST   /api/v1/workspaces/:ws/projects/:id/push
```

### KAZ Export/Import

```
GET    /api/v1/workspaces/:ws/projects/:id/export
POST   /api/v1/workspaces/:ws/projects/import
```

### Connector Management (server-side only)

```
GET    /api/v1/workspaces/:ws/connectors
POST   /api/v1/workspaces/:ws/connectors
GET    /api/v1/workspaces/:ws/connectors/:id/sync-status
POST   /api/v1/workspaces/:ws/connectors/:id/pull
POST   /api/v1/workspaces/:ws/connectors/:id/push
```

### Flow Execution (server-side)

```
POST   /api/v1/workspaces/:ws/projects/:id/flows/run
GET    /api/v1/workspaces/:ws/flows
```

## gRPC Service Definition

```protobuf
service GokapiService {
    rpc CreateProject(CreateProjectRequest) returns (Project);
    rpc StreamBlocks(StreamBlocksRequest) returns (stream Block);
    rpc UpdateTranslation(UpdateTranslationRequest) returns (Block);
    rpc ExecuteFlow(ExecuteFlowRequest) returns (stream FlowProgress);
    rpc Subscribe(SubscribeRequest) returns (stream Event);
}
```

gRPC streaming enables real-time flow progress, block updates, and event subscriptions for the Bowrain desktop app ([AD-012](/docs/ad/012-bowrain)).

## CI/CD Integration

```yaml
# GitHub Actions example
- name: Authenticate with Bowrain Server
  run: kapi auth login --server $\{{ secrets.BOWRAIN_SERVER }}
  env:
    KAPI_AUTH_TOKEN: $\{{ secrets.BOWRAIN_TOKEN }}

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
