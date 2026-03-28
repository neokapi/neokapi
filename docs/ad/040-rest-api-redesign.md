---
id: 040-rest-api-redesign
sidebar_position: 40
title: "AD-040: REST API Redesign — Workspace-First, Stream-Scoped, Short Paths"
---
# AD-040: REST API Redesign — Workspace-First, Stream-Scoped, Short Paths

## Context

The Bowrain server API (~170 endpoints in `platform/server/server.go`) has evolved incrementally during development. Routes were added as features landed, leading to inconsistent naming, duplicated paths, and accidental patterns that would be painful to maintain long-term. Since we are **not live yet**, we can make full breaking changes across all apps and clients.

### Problems in the Current API

| Problem | Example |
|---------|---------|
| Dual flat/workspace routes | `/api/v1/projects/:id` AND `/api/v1/workspaces/:ws/projects/:id` for the same handlers |
| Artificial `editor/` prefix | `/editor/projects/:pid/blocks/:bid` — these are just project resources |
| Inconsistent param names | `:id` vs `:pid` for projects across different route groups |
| Wildcard file paths | `file-blocks/*`, `file-export/*` suffix patterns — fragile, framework-specific |
| Stream route duplication | Every sync/asset route registered twice (with and without `/streams/:stream/` infix) |
| Verb-in-URL | `file-pseudo`, `file-ai-translate`, `file-tm-translate` — not RESTful |
| Inconsistent naming | `tm` (abbreviation) vs `terms` (full word), mixed plural/singular |
| Broken scoping | Connectors, fetch, publish are public when they should be workspace-scoped |
| `automation-runs` as sibling | Should be nested under `automations/` |
| `/my/tasks` special route | Should be a filter on the collection endpoint |
| `files` too specific | Content can come from CMS, git, or filesystem — not always "files" |
| Verbose paths | `/workspaces/:ws/projects/:pid` repeated in every URL |
| Scattered reference data | `/formats`, `/tools`, `/locales` as separate endpoints |

## Decision

Redesign the REST API in-place (stay on `/api/v1`) with the following principles:

### Design Principles

1. **Workspace-first**: All authenticated routes under `/api/v1/w/:ws/...`. Only unclaimed project sync uses flat routes.
2. **Max 3 nesting levels** after workspace: `w/:ws/p/:pid/blocks/:bid`.
3. **Streams mandatory in path** (`/s/:sid/`) for all content-scoped routes. Default stream is `main`. Stream management (CRUD, merge, diff) stays at `/streams/:stream`.
4. **Item paths as query param** (`?item=path/to/file.json`) instead of wildcard `/*` suffixes.
5. **RPC actions use `POST .../actions/<verb>`** (Google Cloud / Azure pattern) for non-CRUD operations.
6. **Consistent naming**: plural nouns, kebab-case, consistent params (`:ws`, `:pid`, `:sid`, `:bid`).
7. **Short prefixes**: `/w/` for workspaces, `/p/` for projects, `/s/` for streams.
8. **Standalone mode**: Virtual workspace `_` (`/api/v1/w/_/...`), same routes, no-op auth middleware. No route duplication between modes.
9. **Items not files**: Generic term for content from any source (CMS, git, filesystem).

### Key Renames

| Current | New | Reason |
|---------|-----|--------|
| `/workspaces/:ws/projects/:pid` | `/w/:ws/p/:pid` | Short, readable, saves ~20 chars |
| `editor/projects/:pid/...` | `p/:pid/...` | Remove artificial prefix |
| `file-blocks/*`, `file-export/*` | `blocks?item=X`, `actions/export` | Standard query param + RPC pattern |
| `/streams/:stream/sync/...` duplication | `/s/:sid/sync/...` (mandatory) | Zero duplication, streams as first-class |
| `file-pseudo`, `file-ai-translate` | `actions/pseudo-translate`, `actions/ai-translate` | No verbs in URLs |
| `tm` | `translation-memory` | Self-documenting |
| `files` | `items` | CMS/git/filesystem agnostic |
| `/formats`, `/tools`, `/locales` | `/info` | Single endpoint returning all reference data |
| `/my/tasks` | `/tasks?assignee_id=me` | Filter, not special route |
| `/fetch`, `/publish` (root) | `/connectors/:id/fetch`, `/connectors/:id/publish` | Scope to connector |
| `automation-runs` (sibling) | `automations/runs` (nested) | Proper hierarchy |

## Route Structure

### Public (no auth)

```
GET    /api/v1/health
GET    /api/v1/ready
GET    /api/v1/config
GET    /api/v1/info                    # { formats, tools, locales, version, ... }
GET    /api/v1/connectors/types        # public reference data only
GET    /api/v1/badges/p/:pid
```

### Auth

```
POST   /api/v1/auth/device/start
POST   /api/v1/auth/device/poll
POST   /api/v1/auth/refresh
GET    /api/v1/auth/login
GET    /api/v1/auth/callback
POST   /api/v1/auth/callback
GET    /api/v1/auth/desktop/login
GET    /api/v1/auth/desktop/callback
GET    /api/v1/auth/device/verify
GET    /api/v1/auth/device/callback
POST   /api/v1/auth/backchannel-logout

# Protected
GET    /api/v1/auth/me
POST   /api/v1/auth/logout
POST   /api/v1/auth/token/exchange
```

### Workspaces

```
GET    /api/v1/w
POST   /api/v1/w
GET    /api/v1/w/:ws
PUT    /api/v1/w/:ws
DELETE /api/v1/w/:ws

# Members
GET    /api/v1/w/:ws/members
POST   /api/v1/w/:ws/members
PUT    /api/v1/w/:ws/members/:uid/role
DELETE /api/v1/w/:ws/members/:uid

# Invites
GET    /api/v1/w/:ws/invites
POST   /api/v1/w/:ws/invites
DELETE /api/v1/w/:ws/invites/:id
POST   /api/v1/join/:code

# Roles
GET    /api/v1/w/:ws/roles
POST   /api/v1/w/:ws/roles
PUT    /api/v1/w/:ws/roles/:rid
DELETE /api/v1/w/:ws/roles/:rid

# Tokens
GET    /api/v1/w/:ws/tokens
POST   /api/v1/w/:ws/tokens
DELETE /api/v1/w/:ws/tokens/:id

# Billing
GET    /api/v1/w/:ws/billing
GET    /api/v1/w/:ws/billing/usage
POST   /api/v1/w/:ws/billing/checkout
POST   /api/v1/w/:ws/billing/portal
GET    /api/v1/w/:ws/billing/invoices
POST   /api/v1/w/:ws/billing/buy-credits

# Audit log
GET    /api/v1/w/:ws/audit-log
```

### Projects

```
GET    /api/v1/w/:ws/p
POST   /api/v1/w/:ws/p
GET    /api/v1/w/:ws/p/:pid
PUT    /api/v1/w/:ws/p/:pid
DELETE /api/v1/w/:ws/p/:pid
POST   /api/v1/w/:ws/p/:pid/restore
DELETE /api/v1/w/:ws/p/:pid/permanent
GET    /api/v1/w/:ws/archived-projects

# Members
GET    /api/v1/w/:ws/p/:pid/members
POST   /api/v1/w/:ws/p/:pid/members
PUT    /api/v1/w/:ws/p/:pid/members/:uid
DELETE /api/v1/w/:ws/p/:pid/members/:uid

# Dashboard (stream-scoped)
GET    /api/v1/w/:ws/p/:pid/s/:sid/dashboard

# Settings
GET    /api/v1/w/:ws/p/:pid/settings/extraction
PUT    /api/v1/w/:ws/p/:pid/settings/extraction

# Audit log (project-level)
GET    /api/v1/w/:ws/p/:pid/audit-log

# Claim (no workspace yet)
POST   /api/v1/p/claim
POST   /api/v1/p/anonymous
```

### Items and Blocks

Stream is always in path as `/s/:sid/`. Item paths use `?item=` query parameter.

```
# Items (generic content units — files, CMS docs, etc.)
GET    /api/v1/w/:ws/p/:pid/s/:sid/items
POST   /api/v1/w/:ws/p/:pid/s/:sid/items                 (multipart upload)
DELETE /api/v1/w/:ws/p/:pid/s/:sid/items                  ?item=path/to/file.json

# Blocks (translatable segments within an item)
GET    /api/v1/w/:ws/p/:pid/s/:sid/blocks                 ?item=path/to/file.json
PUT    /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid
PUT    /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/coded

# Block sub-resources
GET    /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/history       ?locale=fr
GET    /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/notes
POST   /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/notes
DELETE /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/notes/:nid
GET    /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/tm-matches    ?locale=fr
GET    /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/term-matches  ?locale=fr
GET    /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/html          ?locale=fr

# Entities (on blocks)
POST   /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/entities
PUT    /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/entities/:eid
DELETE /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/entities/:eid
POST   /api/v1/w/:ws/p/:pid/s/:sid/blocks/:bid/entities/:eid/promote
```

### Actions (RPC-style, stream-scoped)

Non-CRUD operations use `actions/` prefix, making them explicitly distinct from resource endpoints.

```
POST   /api/v1/w/:ws/p/:pid/s/:sid/actions/pseudo-translate   { item, target_locale }
POST   /api/v1/w/:ws/p/:pid/s/:sid/actions/ai-translate       { item, target_locale, ... }
POST   /api/v1/w/:ws/p/:pid/s/:sid/actions/tm-translate       { item, target_locale }
POST   /api/v1/w/:ws/p/:pid/s/:sid/actions/export             { item, target_locale }
POST   /api/v1/w/:ws/p/:pid/s/:sid/actions/qa-check           { item, target_locale }
POST   /api/v1/w/:ws/p/:pid/s/:sid/actions/qa-check-block     { block_id, locale }
GET    /api/v1/w/:ws/p/:pid/s/:sid/preview                    ?item=X&locale=fr
GET    /api/v1/w/:ws/p/:pid/s/:sid/word-count                 ?item=X
```

### Sync (CLI/CI, stream-scoped)

```
GET    /api/v1/w/:ws/p/:pid/s/:sid/sync/pull
GET    /api/v1/w/:ws/p/:pid/s/:sid/sync/blocks
GET    /api/v1/w/:ws/p/:pid/s/:sid/sync/status
POST   /api/v1/w/:ws/p/:pid/s/:sid/sync/push/init
POST   /api/v1/w/:ws/p/:pid/s/:sid/sync/push/diff
POST   /api/v1/w/:ws/p/:pid/s/:sid/sync/push/commit
PUT    /api/v1/w/:ws/p/:pid/s/:sid/sync/push/chunks/:uploadId/:chunkIndex
POST   /api/v1/w/:ws/p/:pid/s/:sid/sync/translate

# Flat routes for unclaimed projects only (claim-token auth):
GET    /api/v1/p/:pid/s/:sid/sync/pull
POST   /api/v1/p/:pid/s/:sid/sync/push/init
POST   /api/v1/p/:pid/s/:sid/sync/push/diff
POST   /api/v1/p/:pid/s/:sid/sync/push/commit
PUT    /api/v1/p/:pid/s/:sid/sync/push/chunks/:uploadId/:chunkIndex
```

### Streams (CRUD management)

Stream management uses full `streams/` name (CRUD on the stream entity itself). Content scoping uses `/s/:sid/` (short form).

```
GET    /api/v1/w/:ws/p/:pid/streams
POST   /api/v1/w/:ws/p/:pid/streams
GET    /api/v1/w/:ws/p/:pid/streams/:stream
PATCH  /api/v1/w/:ws/p/:pid/streams/:stream
DELETE /api/v1/w/:ws/p/:pid/streams/:stream
POST   /api/v1/w/:ws/p/:pid/streams/:stream/restore
POST   /api/v1/w/:ws/p/:pid/streams/:stream/merge        ?dry_run=true
GET    /api/v1/w/:ws/p/:pid/streams/:stream/diff
POST   /api/v1/w/:ws/p/:pid/streams/:stream/lock
POST   /api/v1/w/:ws/p/:pid/streams/:stream/unlock

# Tags
GET    /api/v1/w/:ws/p/:pid/streams/:stream/tags
POST   /api/v1/w/:ws/p/:pid/streams/:stream/tags
GET    /api/v1/w/:ws/p/:pid/streams/:stream/tags/:tag
DELETE /api/v1/w/:ws/p/:pid/streams/:stream/tags/:tag
GET    /api/v1/w/:ws/p/:pid/tags                          ?kind=release
```

### Collections (stream-scoped)

```
GET    /api/v1/w/:ws/p/:pid/s/:sid/collections
POST   /api/v1/w/:ws/p/:pid/s/:sid/collections
GET    /api/v1/w/:ws/p/:pid/s/:sid/collections/:cid
PUT    /api/v1/w/:ws/p/:pid/s/:sid/collections/:cid
DELETE /api/v1/w/:ws/p/:pid/s/:sid/collections/:cid
POST   /api/v1/w/:ws/p/:pid/s/:sid/collections/:cid/items   (multipart upload)
```

### Assets (stream-scoped)

```
POST   /api/v1/w/:ws/p/:pid/s/:sid/assets/upload-url
GET    /api/v1/w/:ws/p/:pid/s/:sid/assets
POST   /api/v1/w/:ws/p/:pid/s/:sid/assets
GET    /api/v1/w/:ws/p/:pid/s/:sid/assets/:aid
DELETE /api/v1/w/:ws/p/:pid/s/:sid/assets/:aid
POST   /api/v1/w/:ws/p/:pid/s/:sid/assets/:aid/variants/upload-url
GET    /api/v1/w/:ws/p/:pid/s/:sid/assets/:aid/variants
POST   /api/v1/w/:ws/p/:pid/s/:sid/assets/:aid/variants
```

### Versions (stream-scoped)

```
POST   /api/v1/w/:ws/p/:pid/s/:sid/versions
GET    /api/v1/w/:ws/p/:pid/s/:sid/versions
GET    /api/v1/w/:ws/p/:pid/s/:sid/changes
```

### Translation Memory (workspace-level, renamed from `tm`)

```
GET    /api/v1/w/:ws/translation-memory                   ?q=X&source=en&target=fr&cursor=X&limit=N
POST   /api/v1/w/:ws/translation-memory
GET    /api/v1/w/:ws/translation-memory/count
PUT    /api/v1/w/:ws/translation-memory/:eid
DELETE /api/v1/w/:ws/translation-memory/:eid
```

### Terms (workspace-level)

```
GET    /api/v1/w/:ws/terms                                ?q=X&source=en&target=fr&cursor=X&limit=N
POST   /api/v1/w/:ws/terms
GET    /api/v1/w/:ws/terms/count
PUT    /api/v1/w/:ws/terms/:cid
DELETE /api/v1/w/:ws/terms/:cid
POST   /api/v1/w/:ws/terms/import                         { format: "csv"|"json", ... }
GET    /api/v1/w/:ws/terms/export                         ?format=json&name=X
```

### Providers (workspace-level)

```
GET    /api/v1/w/:ws/providers
POST   /api/v1/w/:ws/providers
DELETE /api/v1/w/:ws/providers/:id
POST   /api/v1/w/:ws/providers/test
```

### Brand Profiles (workspace-level, auth required)

```
GET    /api/v1/w/:ws/brand-profiles
POST   /api/v1/w/:ws/brand-profiles
POST   /api/v1/w/:ws/brand-profiles/from-starter
GET    /api/v1/w/:ws/brand-profiles/:id
PUT    /api/v1/w/:ws/brand-profiles/:id
DELETE /api/v1/w/:ws/brand-profiles/:id
POST   /api/v1/w/:ws/brand-profiles/:id/check
GET    /api/v1/w/:ws/brand-profiles/suggested-rules
GET    /api/v1/w/:ws/brand-profiles/starter-packs
```

### Brand Voice (project+stream-scoped)

```
GET    /api/v1/w/:ws/p/:pid/s/:sid/brand-voice/scores     ?locale=X
GET    /api/v1/w/:ws/p/:pid/s/:sid/brand-voice/trends
POST   /api/v1/w/:ws/p/:pid/s/:sid/brand-voice/corrections
```

### Connectors (workspace-scoped, moved from public)

```
GET    /api/v1/w/:ws/connectors
POST   /api/v1/w/:ws/connectors
GET    /api/v1/w/:ws/connectors/:id
DELETE /api/v1/w/:ws/connectors/:id
GET    /api/v1/w/:ws/connectors/:id/status
POST   /api/v1/w/:ws/connectors/:id/fetch
POST   /api/v1/w/:ws/connectors/:id/publish
```

### Automations (project-scoped, runs nested)

```
GET    /api/v1/w/:ws/p/:pid/automations
POST   /api/v1/w/:ws/p/:pid/automations
GET    /api/v1/w/:ws/p/:pid/automations/events
PUT    /api/v1/w/:ws/p/:pid/automations/:rid
DELETE /api/v1/w/:ws/p/:pid/automations/:rid
PATCH  /api/v1/w/:ws/p/:pid/automations/:rid/toggle
GET    /api/v1/w/:ws/p/:pid/automations/history

# Runs (nested under automations)
GET    /api/v1/w/:ws/p/:pid/automations/runs              ?status=X&limit=N
GET    /api/v1/w/:ws/p/:pid/automations/runs/:runId
GET    /api/v1/w/:ws/p/:pid/automations/runs/:runId/steps
GET    /api/v1/w/:ws/p/:pid/automations/runs/:runId/steps/:stepId/logs
POST   /api/v1/w/:ws/p/:pid/automations/runs/:runId/cancel
GET    /api/v1/w/:ws/p/:pid/automations/runs/:runId/events   (SSE)
```

### Review Queue (stream-scoped)

```
GET    /api/v1/w/:ws/p/:pid/s/:sid/review-queue
GET    /api/v1/w/:ws/p/:pid/s/:sid/review-queue/:itemId
POST   /api/v1/w/:ws/p/:pid/s/:sid/review-queue/:itemId/decide
POST   /api/v1/w/:ws/p/:pid/s/:sid/review-queue/:itemId/assign
POST   /api/v1/w/:ws/p/:pid/s/:sid/review-queue/:itemId/split
POST   /api/v1/w/:ws/p/:pid/s/:sid/review-queue/batch-decide
POST   /api/v1/w/:ws/p/:pid/s/:sid/review-queue/sync
```

### Notifications

```
GET    /api/v1/w/:ws/notifications                        ?unread_only=true&limit=N
POST   /api/v1/w/:ws/notifications/:nid/read
POST   /api/v1/w/:ws/notifications/read-all
DELETE /api/v1/w/:ws/notifications/:nid
GET    /api/v1/w/:ws/notifications/ws                     (WebSocket)
GET    /api/v1/w/:ws/notification-preferences
PUT    /api/v1/w/:ws/notification-preferences
GET    /api/v1/w/:ws/digest-settings
PUT    /api/v1/w/:ws/digest-settings
```

### Activities

```
GET    /api/v1/w/:ws/activities                           ?project_id=X&stream=Y&cursor=C
POST   /api/v1/w/:ws/activities/seen
```

### Tasks

No more `/my/tasks` — use `?assignee_id=me` filter.

```
GET    /api/v1/w/:ws/tasks                                ?assignee_id=me&project_id=X&status=Z
POST   /api/v1/w/:ws/tasks
GET    /api/v1/w/:ws/tasks/:tid
PATCH  /api/v1/w/:ws/tasks/:tid
DELETE /api/v1/w/:ws/tasks/:tid
POST   /api/v1/w/:ws/tasks/:tid/assign
POST   /api/v1/w/:ws/tasks/:tid/complete
POST   /api/v1/w/:ws/tasks/:tid/cancel
```

### Jobs and AI Usage

```
GET    /api/v1/w/:ws/jobs
POST   /api/v1/w/:ws/jobs/translate
GET    /api/v1/w/:ws/jobs/:id
DELETE /api/v1/w/:ws/jobs/:id
GET    /api/v1/w/:ws/ai-usage
```

### Bravo Agent

```
GET    /api/v1/w/:ws/bravo/config
PUT    /api/v1/w/:ws/bravo/config
GET    /api/v1/w/:ws/bravo/tools
GET    /api/v1/w/:ws/bravo/usage                          ?from=X&to=Y
GET    /api/v1/w/:ws/bravo/conversations
POST   /api/v1/w/:ws/bravo/conversations
GET    /api/v1/w/:ws/bravo/conversations/:cid
DELETE /api/v1/w/:ws/bravo/conversations/:cid
POST   /api/v1/w/:ws/bravo/conversations/:cid/cancel
PATCH  /api/v1/w/:ws/bravo/conversations/:cid/mode
GET    /api/v1/w/:ws/bravo/conversations/:cid/messages
POST   /api/v1/w/:ws/bravo/conversations/:cid/messages
POST   /api/v1/w/:ws/bravo/conversations/:cid/tool-calls/:tcid/approve
POST   /api/v1/w/:ws/bravo/conversations/:cid/tool-calls/:tcid/deny
```

### Knowledge Graph

```
GET    /api/v1/w/:ws/graph/concepts
GET    /api/v1/w/:ws/graph/nodes/:nodeId/neighbors
GET    /api/v1/w/:ws/graph/nodes/:nodeId/edges
GET    /api/v1/w/:ws/graph/shortest-path                  ?from=X&to=Y
```

### Collab (WebSocket, stream-scoped)

```
GET    /api/v1/w/:ws/p/:pid/s/:sid/collab                 ?item=X
```

### Pulse (public dashboard)

```
GET    /api/v1/pulse
GET    /api/v1/pulse/:workspace
GET    /api/v1/pulse/:workspace/projects
GET    /api/v1/pulse/:workspace/projects/:pid
GET    /api/v1/pulse/:workspace/projects/:pid/locales/:locale
GET    /api/v1/pulse/:workspace/activity
GET    /api/v1/pulse/:workspace/activity/heatmap
GET    /api/v1/pulse/:workspace/leaderboard
GET    /api/v1/pulse/:workspace/terms
GET    /api/v1/pulse/:workspace/terms/:cid
```

### Badges

```
GET    /api/v1/badges/p/:pid
```

### Admin (unchanged)

```
GET    /api/admin/workspaces
GET    /api/admin/workspaces/:id
PUT    /api/admin/workspaces/:id/plan
POST   /api/admin/workspaces/:id/credits
GET    /api/admin/workspaces/:id/feature-overrides
PUT    /api/admin/workspaces/:id/feature-overrides
GET    /api/admin/workspaces/:id/notes
POST   /api/admin/workspaces/:id/notes
GET    /api/admin/workspaces/:id/ledger
POST   /api/admin/workspaces/:id/impersonate
POST   /api/admin/workspaces/:id/members
GET    /api/admin/users
GET    /api/admin/users/:id
GET    /api/admin/metrics
GET    /api/admin/events
GET    /api/admin/upsells
GET    /api/admin/overrides
```

### Webhooks (unchanged)

```
POST   /api/webhooks/stripe
```

## Standalone Mode

In standalone mode (`bowrain serve`, no JWTSecret):

- All workspace-scoped routes are registered identically
- Client uses `_` as the workspace slug: `/api/v1/w/_/p/:pid/s/main/blocks`
- `AuthMiddleware` is replaced with a no-op that injects a synthetic user context
- `WorkspaceAccessMiddleware` is replaced with a no-op
- No route duplication between modes

## Implementation Notes

### Stream Duality

Streams appear in two URL patterns:
- **`/s/:sid/`** — short form for content scoping (blocks, items, sync, actions, assets, etc.). Always mandatory, defaults to `main`.
- **`/streams/:stream`** — full form for stream management (CRUD, merge, diff, lock/unlock, tags). The stream IS the resource here.

This cleanly separates "operating within a stream" from "operating on a stream".

### Item Path Encoding

File/item paths are passed as `?item=path/to/file.json` using standard URL encoding (`encodeURIComponent`). This replaces the wildcard `/*` suffix pattern which was fragile and framework-specific.

### Actions Convention

Non-CRUD operations live under `actions/`:
- `POST /api/v1/w/:ws/p/:pid/s/:sid/actions/ai-translate`
- `POST /api/v1/w/:ws/p/:pid/s/:sid/actions/export`

This follows the Google Cloud API pattern and makes it unambiguous whether an endpoint is a resource (GET/PUT/DELETE) or an operation (POST to `actions/`).

## Affected Files

| File | Changes |
|------|---------|
| `platform/server/server.go` | Route registration rewrite |
| `platform/server/editor.go` | `streamParam()` → `c.Param("sid")`, item path extraction |
| `platform/server/handlers.go` | Consolidate `/info` endpoint |
| `platform/server/handlers_*.go` | Param name standardization (`:id`→`:pid`, `:stream`→`:sid`) |
| `platform/server/handlers_connector.go` | Scope to workspace, connector ID for fetch/publish |
| `platform/server/middleware_auth.go` | Virtual workspace `_` support |
| `platform/packages/ui/src/api/rest-adapter.ts` | All URL constructions |
| `platform/packages/ui/src/api/adapter.ts` | Rename file→item in method signatures |
| `platform/core/client/client.go` | URL prefix and stream handling |
| `platform/apps/web/src/api.ts` | Login redirect URL |
