---
sidebar_position: 7
title: "CLI Commands Reference"
---

# CLI Commands Reference

This note provides implementation details for [AD-010](/architecture-decisions/010-bowrain-cli-and-project-model) and [AD-011](/architecture-decisions/011-rest-api).

## kapi (with bowrain plugin) command tree

The standalone `bowrain` binary is retired; once the `kapi-bowrain` plugin is
installed, every command below runs as `kapi <command>`.

```
kapi
+-- init             # Initialize a new .kapi project (recipe + state dir)
|   +-- --name, --source, --targets, --server, --project, --anonymous, --email, --preset
+-- config           # View or set configuration values
|   +-- --global (use ~/.config/kapi/kapi.yaml instead of <dir-name>.kapi)
+-- ls               # List tracked files with optional stats
|   +-- --stats/-s, --dirty/-d, [paths...]
+-- add              # Add file patterns to track
|   +-- --format/-f, <pattern> [pattern...]
+-- rm               # Stop tracking files (remove mapping or add exclude)
|   +-- <pattern> [pattern...]
+-- status           # Show sync state (local vs remote)
+-- diff             # Show changes between local and remote
+-- pull             # Pull from Bowrain Server -> update local files
|   +-- --force, --dry-run, --locale (repeatable)
+-- push             # Push local files -> update Bowrain Server
|   +-- --force, --dry-run, [paths...]
+-- serve            # Start local dashboard (web UI)
|   +-- --port 3000, --no-open
+-- auth             # Authentication with Bowrain Server
|   +-- login        # OAuth device flow login (--server)
|   +-- logout       # Remove stored token
|   +-- status       # Show current user, server URL
|   +-- claim        # Claim anonymous project
+-- run FLOW         # Execute a flow (inline on recipe, .kapi/flows/, or built-in)
+-- <tool>           # Run a tool directly (pseudo-translate, translate, qa, etc.)
+-- flows            # List available flows
+-- tools            # List available tools
+-- sync             # Sync operations (push + translate + pull)
+-- ui               # Open project in Bowrain web UI
+-- termbase         # Terminology management
|   +-- list         # List terminology entries
+-- formats          # Format listing
|   +-- list         # List available formats (built-in + plugin)
+-- plugins          # Plugin management
|   +-- list         # List installed plugins
+-- presets          # Preset management
|   +-- list         # List presets
|   +-- validate     # Validate project preset references
+-- version          # Show version info
+-- mcp              # Start MCP server for AI agent integration
```

## kapi command tree

```
kapi
+-- <tool>           # Run a tool directly (pseudo-translate, translate, qa, etc.)
+-- run FLOW         # Execute a composed multi-tool flow (translate-qa, etc.)
+-- tools            # List available tools
+-- flows            # List available flows
+-- formats          # Format listing
|   +-- list         # List available formats (built-in + plugin)
+-- plugins          # Plugin management
|   +-- list         # List installed plugins
+-- presets          # Preset management
|   +-- list         # List presets
+-- termbase         # Terminology management
|   +-- list         # List terminology entries
+-- version          # Show version info
+-- mcp              # Start MCP server for AI agent integration
```

## `kapi init` Workflows

### Interactive Mode (default when stdin is a terminal)

- If already signed in: select workspace -> enter project name -> select source locale
- If not signed in: choose from Sign in / Email claim / Anonymous / Local only
- Workspace selector allows choosing existing workspaces or creating new ones
- Source locale uses a BCP-47 selector with type-ahead filtering

### Non-interactive Workflow

1. Check if `<dir-name>.kapi` or `.kapi/` already exists (error if so)
2. Write `<dir-name>.kapi` recipe with defaults or provided flags
3. If `--anonymous` or `--email`: create anonymous project on server, write `server:` block
4. If `--project` provided: verify auth and connect to existing project, write `server:` block
5. If authenticated with no flags: create project in personal workspace, write `server:` block
6. Create `.kapi/flows/` directory with example flows
7. Add `.kapi/` to `.gitignore`

All paths support `--json` output for CI/CD integration.

## `kapi pull` Algorithm

1. Read the recipe (`server.url`, content collections)
2. Verify auth token (keychain `bowrain-auth:<server-url>` or `BOWRAIN_AUTH_TOKEN`)
3. Call `GET /api/v1/projects/:id/sync/pull?cursor=X&locales=...`
   - Response: changes since cursor, new cursor, has_more
   - Paginated: follow has_more until all changes consumed
4. Write blocks to local files via FormatRegistry
5. If the project is workspace-claimed, snapshot governed terminology: paginate
   `GET /api/v1/:ws/concepts`, fetch each concept's relations via
   `GET /api/v1/:ws/concepts/:cid/relations`, and write both into the project
   termbase (`.kapi/termbase.db`) through `AddConcept`/`AddRelation`. Record a
   `ConceptBaseline` in the sync cache so a later `kapi push` can diff local
   terminology edits against it.
6. Run `post-pull` hooks (if configured)
7. Update `.kapi/cache/sync-cache.json`

**Conflict handling:**

- By default, pull fails if local files have uncommitted changes
- `--force` overwrites local changes

## `kapi push` Algorithm

1. Read the recipe (content collections)
2. Run `pre-push` hooks (qa, term-enforce, etc.)
   - If any hook fails, abort push
3. Read local files via FormatRegistry
4. Compute block hashes
5. Compare with `.kapi/cache/sync-cache.json` -> identify changed blocks
6. Verify auth token (keychain `bowrain-auth:<server-url>` or `BOWRAIN_AUTH_TOKEN`)
7. Call `POST /api/v1/projects/:id/sync/push`
   - Request body: `{ blocks: [{id, text, name, type, item_name}] }`
   - Response: `{ stored: N, new_cursor: X, push_id: "..." }`
   - Batched at 1000 blocks per request (MaxBlocksPerRequest)
8. Update `.kapi/cache/sync-cache.json`

## REST API Routes

### Public Routes

```
GET    /api/v1/health
GET    /api/v1/config                               # Server configuration
GET    /api/v1/info                                  # Server information
GET    /api/v1/formats                               # List supported formats
GET    /api/v1/tools                                 # List available tools
GET    /api/v1/locales                               # List known locales (BCP-47)
POST   /api/v1/projects/anonymous                    # Create anonymous project
```

### Auth Routes

```
POST   /api/v1/auth/device/start                    # Start device auth flow
POST   /api/v1/auth/device/poll                     # Poll for token
POST   /api/v1/auth/refresh                         # Token refresh
GET    /api/v1/auth/login                           # OAuth/OIDC login redirect
GET    /api/v1/auth/callback                        # OIDC redirect callback
GET    /api/v1/auth/me                              # Get current user (JWT)
POST   /api/v1/auth/logout                          # Invalidate token (JWT)
```

### Project Routes (JWT-protected)

```
POST   /api/v1/projects
GET    /api/v1/projects
GET    /api/v1/projects/:id
PUT    /api/v1/projects/:id
DELETE /api/v1/projects/:id
POST   /api/v1/projects/claim                       # Claim anonymous project
POST   /api/v1/projects/:id/blocks                  # Store blocks
GET    /api/v1/projects/:id/blocks                  # Retrieve blocks
POST   /api/v1/projects/:id/versions                # Create version snapshot
GET    /api/v1/projects/:id/versions                # List versions
GET    /api/v1/projects/:id/changes                 # Get block changes
```

### Sync Routes (JWT or ClaimToken)

```
POST   /api/v1/projects/:id/sync/push               # Push source blocks
GET    /api/v1/projects/:id/sync/pull                # Pull changes since cursor
GET    /api/v1/projects/:id/sync/blocks              # Get blocks for an item
GET    /api/v1/projects/:id/sync/status              # Push status (job tracking)
POST   /api/v1/projects/:id/sync/translate           # Create translation job
```

Workspace-scoped equivalents at `/api/v1/workspaces/:ws/projects/:id/sync/...`.

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

### Connector Management

```
GET    /api/v1/connectors/types                      # List connector types
GET    /api/v1/connectors                            # List active connectors
POST   /api/v1/connectors                            # Add connector
DELETE /api/v1/connectors/:id                        # Remove connector
GET    /api/v1/connectors/:id/status                 # Check status
POST   /api/v1/fetch                                 # Fetch content via connector
POST   /api/v1/publish                               # Publish content via connector
```

## gRPC Service Definitions

Two gRPC services are defined in `bowrain/proto/v1/`:

```protobuf
service NeokapiService {
    // Project management
    rpc CreateProject(CreateProjectRequest) returns (ProjectResponse);
    rpc GetProject(GetProjectRequest) returns (ProjectResponse);
    rpc ListProjects(ListProjectsRequest) returns (ListProjectsResponse);
    rpc UpdateProject(UpdateProjectRequest) returns (ProjectResponse);
    rpc DeleteProject(DeleteProjectRequest) returns (google.protobuf.Empty);
    rpc CreateAnonymousProject(CreateProjectRequest) returns (ProjectResponse);
    rpc ClaimProject(ClaimProjectRequest) returns (ProjectResponse);
    rpc GetProjectChanges(GetChangesRequest) returns (GetChangesResponse);

    // Block operations
    rpc StoreBlocks(StoreBlocksRequest) returns (StoreBlocksResponse);
    rpc GetBlocks(GetBlocksRequest) returns (GetBlocksResponse);
    rpc StreamBlocks(StreamBlocksRequest) returns (stream BlockResponse);

    // Version management
    rpc CreateVersion(CreateVersionRequest) returns (VersionResponse);
    rpc ListVersions(ListVersionsRequest) returns (ListVersionsResponse);

    // Connector operations
    rpc PullContent(PullContentRequest) returns (PullContentResponse);
    rpc PushContent(PushContentRequest) returns (PushContentResponse);

    // Flow execution
    rpc ExecuteFlow(ExecuteFlowRequest) returns (stream FlowProgressResponse);

    // Event subscription
    rpc Subscribe(SubscribeRequest) returns (stream EventResponse);
}

service EditorService {
    // Authentication & workspace
    rpc GetCurrentUser(GetCurrentUserRequest) returns (UserResponse);
    rpc ListWorkspaces(ListWorkspacesRequest) returns (ListWorkspacesResponse);

    // Editor projects
    rpc ListEditorProjects(ListEditorProjectsRequest) returns (ListEditorProjectsResponse);
    rpc GetEditorProject(GetEditorProjectRequest) returns (EditorProjectResponse);

    // Block operations
    rpc GetBlocks(GetBlocksRequest) returns (GetBlocksResponse);
    rpc UpdateBlockTarget(UpdateBlockTargetRequest) returns (google.protobuf.Empty);
    rpc ReviewBlock(ReviewBlockRequest) returns (google.protobuf.Empty);

    // Context lookups
    rpc LookupTMForBlock(TMLookupRequest) returns (TMLookupResponse);
    rpc LookupTermsForBlock(TermLookupRequest) returns (TermLookupResponse);

    // TM and terminology CRUD
    rpc GetTMEntries(TMEntriesRequest) returns (TMEntriesResponse);
    rpc AddTMEntry(AddTMEntryRequest) returns (TMEntryResponse);
    rpc UpdateTMEntry(UpdateTMEntryRequest) returns (google.protobuf.Empty);
    rpc DeleteTMEntry(DeleteTMEntryRequest) returns (google.protobuf.Empty);
    rpc GetTerms(TermsRequest) returns (TermsResponse);
    rpc AddConcept(AddConceptRequest) returns (ConceptResponse);
    rpc UpdateConcept(UpdateConceptRequest) returns (google.protobuf.Empty);
    rpc DeleteConcept(DeleteConceptRequest) returns (google.protobuf.Empty);

    // Presence & collaboration
    rpc UpdatePresence(UpdatePresenceRequest) returns (google.protobuf.Empty);
    rpc WatchProject(WatchProjectRequest) returns (stream ProjectEvent);
}
```

gRPC and REST are multiplexed on the same port via h2c (HTTP/2 cleartext). gRPC streaming enables real-time flow progress, block updates, presence tracking, and event subscriptions for the Bowrain desktop app ([AD-017](/architecture-decisions/017-bowrain-apps)).

## CI/CD Integration

```yaml
# GitHub Actions example
- name: Pull latest translations
  run: kapi pull
  env:
    BOWRAIN_AUTH_TOKEN: $\{{ secrets.BOWRAIN_TOKEN }}
    BOWRAIN_SERVER_URL: $\{{ secrets.BOWRAIN_SERVER }}

- name: Run pseudo-translation
  run: kapi pseudo-translate

- name: Push changes if tests pass
  run: kapi push
  env:
    BOWRAIN_AUTH_TOKEN: $\{{ secrets.BOWRAIN_TOKEN }}
    BOWRAIN_SERVER_URL: $\{{ secrets.BOWRAIN_SERVER }}
```

**CI-friendly features:**

- `--json` flag for machine-readable output
- Exit codes: 0 (success), 1 (error), 2 (conflict)
- `BOWRAIN_AUTH_TOKEN` environment variable (bypasses device flow login)
