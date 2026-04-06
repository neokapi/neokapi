---
id: 040-rest-api-redesign
sidebar_position: 40
title: "AD-040: REST API Redesign — Slug-Based Hierarchy, Ref-Scoped, Clean Paths"
---

# AD-040: REST API Redesign — Slug-Based Hierarchy, Ref-Scoped, Clean Paths

## Context

The Bowrain server API (~170 endpoints in `platform/server/server.go`) has evolved incrementally during development. Routes were added as features landed, leading to inconsistent naming, duplicated paths, and accidental patterns that would be painful to maintain long-term. Since we are **not live yet**, we can make full breaking changes across all apps and clients.

### Problems in the Current API

| Problem                      | Example                                                                                |
| ---------------------------- | -------------------------------------------------------------------------------------- |
| Dual flat/workspace routes   | `/api/v1/projects/:id` AND `/api/v1/workspaces/:ws/projects/:id` for the same handlers |
| Artificial `editor/` prefix  | `/editor/projects/:pid/blocks/:bid` — these are just project resources                 |
| Inconsistent param names     | `:id` vs `:pid` for projects across different route groups                             |
| Wildcard file paths          | `file-blocks/*`, `file-export/*` suffix patterns — fragile, framework-specific         |
| Stream route duplication     | Every sync/asset route registered twice (with and without `/streams/:stream/` infix)   |
| Verb-in-URL                  | `file-pseudo`, `file-ai-translate`, `file-tm-translate` — not RESTful                  |
| Inconsistent naming          | `tm` (abbreviation) vs `terms` (full word), mixed plural/singular                      |
| Broken scoping               | Connectors, fetch, publish are public when they should be workspace-scoped             |
| `automation-runs` as sibling | Should be nested under `automations/`                                                  |
| `/my/tasks` special route    | Should be a filter on the collection endpoint                                          |
| `files` too specific         | Content can come from CMS, git, or filesystem — not always "files"                     |
| Verbose paths                | `/workspaces/:ws/projects/:pid` with `/w/` and `/p/` prefixes                          |
| Scattered reference data     | `/formats`, `/tools`, `/locales` as separate endpoints                                 |

## Decision

Redesign the REST API in-place (stay on `/api/v1`) with the following principles:

### Design Principles

1. **Slug-based hierarchy**: Workspace and project slugs appear as bare path segments — `/:ws/:proj/...` — mirroring GitHub's `/:owner/:repo/` pattern. No `/w/` or `/p/` prefixes.
2. **Reserved name blocklist**: Fixed route segments (`auth`, `admin`, `health`, `info`, `pulse`, etc.) are reserved and rejected at workspace/project creation time. Routers match static segments before parameterized ones.
3. **Resource-first ref scoping**: Content routes place the resource keyword first, then the ref — `/:ws/:proj/blocks/:ref` — following GitHub's `/:owner/:repo/tree/:ref` pattern. The resource keyword (`blocks`, `items`, `sync`, etc.) disambiguates from non-ref project routes (`members`, `settings`, `automations`). No `/s/` or `/r/` prefix needed.
4. **Refs are streams or tags**: `:ref` resolves to either a stream (read-write) or a tag (read-only snapshot), like GitHub's branch/tag interchangeability. Default ref is `main`. Stream CRUD at `/streams/:stream`, tag CRUD at `/tags/:tag` — both are peers in the ref namespace.
5. **Item paths as query param** (`?item=path/to/file.json`) instead of wildcard `/*` suffixes.
6. **RPC actions use `POST .../actions/:ref/<verb>`** (Google Cloud / Azure pattern) for non-CRUD operations.
7. **Consistent naming**: plural nouns, kebab-case, human-readable slugs everywhere (`:ws`, `:proj`, `:ref`).
8. **Standalone mode**: Virtual workspace `_` (`/api/v1/_/...`), same routes, no-op auth middleware. No route duplication between modes.
9. **Items not files**: Generic term for content from any source (CMS, git, filesystem).
10. **All identifiers are slugs**: Workspaces, projects, streams, and tags use human-readable slugs in URLs — not UUIDs. Slug-to-ID resolution happens once in middleware.

### Key Renames

| Current                                 | New                                                | Reason                                       |
| --------------------------------------- | -------------------------------------------------- | -------------------------------------------- |
| `/w/:ws/p/:pid`                         | `/:ws/:proj`                                       | GitHub-style bare slugs, no prefixes         |
| `editor/projects/:pid/...`              | `/:proj/...`                                       | Remove artificial prefix                     |
| `file-blocks/*`, `file-export/*`        | `blocks?item=X`, `actions/export`                  | Standard query param + RPC pattern           |
| `/s/:ref/blocks`, `/s/:ref/items`       | `/blocks/:ref`, `/items/:ref`                      | GitHub-style resource-first, no prefix       |
| `/streams/:stream/sync/...` duplication | `/sync/:ref/...` (mandatory)                       | Zero duplication, refs as first-class        |
| `streams/:stream/tags` (nested)         | `/tags/:tag` (peer to `/streams/:stream`)          | Tags and streams are interchangeable refs    |
| `file-pseudo`, `file-ai-translate`      | `actions/pseudo-translate`, `actions/ai-translate` | No verbs in URLs                             |
| `tm`                                    | `translation-memory`                               | Self-documenting                             |
| `files`                                 | `items`                                            | CMS/git/filesystem agnostic                  |
| `/formats`, `/tools`, `/locales`        | `/info`                                            | Single endpoint returning all reference data |
| `/my/tasks`                             | `/tasks?assignee_id=me`                            | Filter, not special route                    |
| `/fetch`, `/publish` (root)             | `/connectors/:id/fetch`, `/connectors/:id/publish` | Scope to connector                           |
| `automation-runs` (sibling)             | `automations/runs` (nested)                        | Proper hierarchy                             |

### Reserved Names

Workspace slugs cannot match top-level API route segments. Project slugs cannot match workspace-level route segments. These blocklists are checked at creation time.

**Reserved workspace slugs** (top-level API segments):
`auth`, `admin`, `health`, `ready`, `info`, `pulse`, `badges`, `join`, `webhooks`, `workspaces`, `projects`, `connectors`, `_`

**Reserved project slugs** (workspace-level segments):
`members`, `invites`, `roles`, `tokens`, `billing`, `audit-log`, `providers`, `terms`, `translation-memory`, `connectors`, `tasks`, `jobs`, `ai-usage`, `bravo`, `graph`, `activities`, `notifications`, `notification-preferences`, `digest-settings`, `brand-profiles`, `archived-projects`, `projects`, `streams`, `tags`, `refs`, `blocks`, `items`, `sync`, `actions`, `assets`, `collections`, `preview`, `word-count`, `review-queue`, `brand-voice`, `collab`, `dashboard`, `automations`, `settings`

## Route Structure

### Public (no auth)

```
GET    /api/v1/health
GET    /api/v1/ready
GET    /api/v1/info                    # { mode, formats, tools, locales, connector_types, version, ... }
GET    /api/v1/badges/:proj
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

Collection routes use the full noun `/workspaces`. Single-resource routes use the bare slug.

```
GET    /api/v1/workspaces
POST   /api/v1/workspaces
GET    /api/v1/:ws
PUT    /api/v1/:ws
DELETE /api/v1/:ws

# Members
GET    /api/v1/:ws/members
POST   /api/v1/:ws/members
PUT    /api/v1/:ws/members/:uid/role
DELETE /api/v1/:ws/members/:uid

# Invites
GET    /api/v1/:ws/invites
POST   /api/v1/:ws/invites
DELETE /api/v1/:ws/invites/:id
POST   /api/v1/join/:code

# Roles
GET    /api/v1/:ws/roles
POST   /api/v1/:ws/roles
PUT    /api/v1/:ws/roles/:rid
DELETE /api/v1/:ws/roles/:rid

# Tokens
GET    /api/v1/:ws/tokens
POST   /api/v1/:ws/tokens
DELETE /api/v1/:ws/tokens/:id

# Billing
GET    /api/v1/:ws/billing
GET    /api/v1/:ws/billing/usage
POST   /api/v1/:ws/billing/checkout
POST   /api/v1/:ws/billing/portal
GET    /api/v1/:ws/billing/invoices
POST   /api/v1/:ws/billing/buy-credits

# Audit log
GET    /api/v1/:ws/audit-log
```

### Projects

Collection routes use the full noun `/projects`. Single-resource routes use the bare slug.

```
GET    /api/v1/:ws/projects
POST   /api/v1/:ws/projects
GET    /api/v1/:ws/:proj
PUT    /api/v1/:ws/:proj
DELETE /api/v1/:ws/:proj
POST   /api/v1/:ws/:proj/restore
DELETE /api/v1/:ws/:proj/permanent
GET    /api/v1/:ws/archived-projects

# Members
GET    /api/v1/:ws/:proj/members
POST   /api/v1/:ws/:proj/members
PUT    /api/v1/:ws/:proj/members/:uid
DELETE /api/v1/:ws/:proj/members/:uid

# Settings
GET    /api/v1/:ws/:proj/settings/extraction
PUT    /api/v1/:ws/:proj/settings/extraction

# Audit log (project-level)
GET    /api/v1/:ws/:proj/audit-log

# Claim (no workspace yet)
POST   /api/v1/projects/claim
POST   /api/v1/projects/anonymous
```

### Items and Blocks

Resource keyword first, then ref. Item paths use `?item=` query parameter.

```
# Items (generic content units — files, CMS docs, etc.)
GET    /api/v1/:ws/:proj/items/:ref
POST   /api/v1/:ws/:proj/items/:ref                    (multipart upload)
DELETE /api/v1/:ws/:proj/items/:ref                     ?item=path/to/file.json

# Blocks (translatable segments within an item)
GET    /api/v1/:ws/:proj/blocks/:ref                    ?item=path/to/file.json
PUT    /api/v1/:ws/:proj/blocks/:ref/:bid
PUT    /api/v1/:ws/:proj/blocks/:ref/:bid/coded

# Block sub-resources
GET    /api/v1/:ws/:proj/blocks/:ref/:bid/history       ?locale=fr
GET    /api/v1/:ws/:proj/blocks/:ref/:bid/notes
POST   /api/v1/:ws/:proj/blocks/:ref/:bid/notes
DELETE /api/v1/:ws/:proj/blocks/:ref/:bid/notes/:nid
GET    /api/v1/:ws/:proj/blocks/:ref/:bid/tm-matches    ?locale=fr
GET    /api/v1/:ws/:proj/blocks/:ref/:bid/term-matches  ?locale=fr
GET    /api/v1/:ws/:proj/blocks/:ref/:bid/html          ?locale=fr

# Entities (on blocks)
POST   /api/v1/:ws/:proj/blocks/:ref/:bid/entities
PUT    /api/v1/:ws/:proj/blocks/:ref/:bid/entities/:eid
DELETE /api/v1/:ws/:proj/blocks/:ref/:bid/entities/:eid
POST   /api/v1/:ws/:proj/blocks/:ref/:bid/entities/:eid/promote
```

### Actions (RPC-style, ref-scoped)

Non-CRUD operations use `actions/` prefix, making them explicitly distinct from resource endpoints.

```
POST   /api/v1/:ws/:proj/actions/:ref/pseudo-translate   { item, target_locale }
POST   /api/v1/:ws/:proj/actions/:ref/ai-translate       { item, target_locale, ... }
POST   /api/v1/:ws/:proj/actions/:ref/tm-translate       { item, target_locale }
POST   /api/v1/:ws/:proj/actions/:ref/export             { item, target_locale }
POST   /api/v1/:ws/:proj/actions/:ref/qa-check           { item, target_locale }
POST   /api/v1/:ws/:proj/actions/:ref/qa-check-block     { block_id, locale }
```

### Preview and Word Count (ref-scoped)

```
GET    /api/v1/:ws/:proj/preview/:ref                    ?item=X&locale=fr
GET    /api/v1/:ws/:proj/word-count/:ref                 ?item=X
```

### Dashboard (ref-scoped)

```
GET    /api/v1/:ws/:proj/dashboard/:ref
```

### Sync (CLI/CI, ref-scoped)

```
GET    /api/v1/:ws/:proj/sync/:ref/pull
GET    /api/v1/:ws/:proj/sync/:ref/blocks
GET    /api/v1/:ws/:proj/sync/:ref/status
POST   /api/v1/:ws/:proj/sync/:ref/push/init
POST   /api/v1/:ws/:proj/sync/:ref/push/diff
POST   /api/v1/:ws/:proj/sync/:ref/push/commit
PUT    /api/v1/:ws/:proj/sync/:ref/push/chunks/:uploadId/:chunkIndex
POST   /api/v1/:ws/:proj/sync/:ref/translate

# Flat routes for unclaimed projects only (claim-token auth):
GET    /api/v1/projects/:proj/sync/:ref/pull
POST   /api/v1/projects/:proj/sync/:ref/push/init
POST   /api/v1/projects/:proj/sync/:ref/push/diff
POST   /api/v1/projects/:proj/sync/:ref/push/commit
PUT    /api/v1/projects/:proj/sync/:ref/push/chunks/:uploadId/:chunkIndex
```

### Refs (unified listing)

Streams and tags share a ref namespace — like git branches and tags.

```
GET    /api/v1/:ws/:proj/refs                           ?kind=stream|tag
```

### Streams (CRUD management)

```
GET    /api/v1/:ws/:proj/streams
POST   /api/v1/:ws/:proj/streams
GET    /api/v1/:ws/:proj/streams/:stream
PATCH  /api/v1/:ws/:proj/streams/:stream
DELETE /api/v1/:ws/:proj/streams/:stream
POST   /api/v1/:ws/:proj/streams/:stream/restore
POST   /api/v1/:ws/:proj/streams/:stream/merge        ?dry_run=true
GET    /api/v1/:ws/:proj/streams/:stream/diff
POST   /api/v1/:ws/:proj/streams/:stream/lock
POST   /api/v1/:ws/:proj/streams/:stream/unlock
```

### Tags (CRUD management)

Tags are named snapshots created from a stream. They are peers to streams in the ref namespace — content routes work identically for both (`/blocks/main` and `/blocks/v1.2.0`). Write operations on a tag ref return 409 (frozen).

```
GET    /api/v1/:ws/:proj/tags
POST   /api/v1/:ws/:proj/tags                           { name: "v1.2.0", stream: "main" }
GET    /api/v1/:ws/:proj/tags/:tag
DELETE /api/v1/:ws/:proj/tags/:tag
```

### Collections (ref-scoped)

```
GET    /api/v1/:ws/:proj/collections/:ref
POST   /api/v1/:ws/:proj/collections/:ref
GET    /api/v1/:ws/:proj/collections/:ref/:cid
PUT    /api/v1/:ws/:proj/collections/:ref/:cid
DELETE /api/v1/:ws/:proj/collections/:ref/:cid
POST   /api/v1/:ws/:proj/collections/:ref/:cid/items   (multipart upload)
```

### Assets (ref-scoped)

```
POST   /api/v1/:ws/:proj/assets/:ref/upload-url
GET    /api/v1/:ws/:proj/assets/:ref
POST   /api/v1/:ws/:proj/assets/:ref
GET    /api/v1/:ws/:proj/assets/:ref/:aid
DELETE /api/v1/:ws/:proj/assets/:ref/:aid
POST   /api/v1/:ws/:proj/assets/:ref/:aid/variants/upload-url
GET    /api/v1/:ws/:proj/assets/:ref/:aid/variants
POST   /api/v1/:ws/:proj/assets/:ref/:aid/variants
```

### Translation Memory (workspace-level, renamed from `tm`)

```
GET    /api/v1/:ws/translation-memory                   ?q=X&source=en&target=fr&cursor=X&limit=N
POST   /api/v1/:ws/translation-memory
GET    /api/v1/:ws/translation-memory/count
PUT    /api/v1/:ws/translation-memory/:eid
DELETE /api/v1/:ws/translation-memory/:eid
```

### Terms (workspace-level)

```
GET    /api/v1/:ws/terms                                ?q=X&source=en&target=fr&cursor=X&limit=N
POST   /api/v1/:ws/terms
GET    /api/v1/:ws/terms/count
PUT    /api/v1/:ws/terms/:cid
DELETE /api/v1/:ws/terms/:cid
POST   /api/v1/:ws/terms/import                         { format: "csv"|"json", ... }
GET    /api/v1/:ws/terms/export                         ?format=json&name=X
```

### Providers (workspace-level)

```
GET    /api/v1/:ws/providers
POST   /api/v1/:ws/providers
DELETE /api/v1/:ws/providers/:id
POST   /api/v1/:ws/providers/test
```

### Brand Profiles (workspace-level, auth required)

```
GET    /api/v1/:ws/brand-profiles
POST   /api/v1/:ws/brand-profiles
POST   /api/v1/:ws/brand-profiles/from-starter
GET    /api/v1/:ws/brand-profiles/:id
PUT    /api/v1/:ws/brand-profiles/:id
DELETE /api/v1/:ws/brand-profiles/:id
POST   /api/v1/:ws/brand-profiles/:id/check
GET    /api/v1/:ws/brand-profiles/suggested-rules
GET    /api/v1/:ws/brand-profiles/starter-packs
```

### Brand Voice (ref-scoped)

```
GET    /api/v1/:ws/:proj/brand-voice/:ref/scores        ?locale=X
GET    /api/v1/:ws/:proj/brand-voice/:ref/trends
POST   /api/v1/:ws/:proj/brand-voice/:ref/corrections
```

### Connectors (workspace-scoped, moved from public)

```
GET    /api/v1/:ws/connectors
POST   /api/v1/:ws/connectors
GET    /api/v1/:ws/connectors/:id
DELETE /api/v1/:ws/connectors/:id
GET    /api/v1/:ws/connectors/:id/status
POST   /api/v1/:ws/connectors/:id/fetch
POST   /api/v1/:ws/connectors/:id/publish
```

### Automations (project-scoped, runs nested)

```
GET    /api/v1/:ws/:proj/automations
POST   /api/v1/:ws/:proj/automations
GET    /api/v1/:ws/:proj/automations/events
PUT    /api/v1/:ws/:proj/automations/:rid
DELETE /api/v1/:ws/:proj/automations/:rid
PATCH  /api/v1/:ws/:proj/automations/:rid/toggle
GET    /api/v1/:ws/:proj/automations/history

# Runs (nested under automations)
GET    /api/v1/:ws/:proj/automations/runs              ?status=X&limit=N
GET    /api/v1/:ws/:proj/automations/runs/:runId
GET    /api/v1/:ws/:proj/automations/runs/:runId/steps
GET    /api/v1/:ws/:proj/automations/runs/:runId/steps/:stepId/logs
POST   /api/v1/:ws/:proj/automations/runs/:runId/cancel
GET    /api/v1/:ws/:proj/automations/runs/:runId/events   (SSE)
```

### Review Queue (ref-scoped)

```
GET    /api/v1/:ws/:proj/review-queue/:ref
GET    /api/v1/:ws/:proj/review-queue/:ref/:itemId
POST   /api/v1/:ws/:proj/review-queue/:ref/:itemId/decide
POST   /api/v1/:ws/:proj/review-queue/:ref/:itemId/assign
POST   /api/v1/:ws/:proj/review-queue/:ref/:itemId/split
POST   /api/v1/:ws/:proj/review-queue/:ref/batch-decide
POST   /api/v1/:ws/:proj/review-queue/:ref/sync
```

### Notifications

```
GET    /api/v1/:ws/notifications                        ?unread_only=true&limit=N
POST   /api/v1/:ws/notifications/:nid/read
POST   /api/v1/:ws/notifications/read-all
DELETE /api/v1/:ws/notifications/:nid
GET    /api/v1/:ws/notifications/ws                     (WebSocket)
GET    /api/v1/:ws/notification-preferences
PUT    /api/v1/:ws/notification-preferences
GET    /api/v1/:ws/digest-settings
PUT    /api/v1/:ws/digest-settings
```

### Activities

```
GET    /api/v1/:ws/activities                           ?project_id=X&stream=Y&cursor=C
POST   /api/v1/:ws/activities/seen
```

### Tasks

No more `/my/tasks` — use `?assignee_id=me` filter.

```
GET    /api/v1/:ws/tasks                                ?assignee_id=me&project_id=X&status=Z
POST   /api/v1/:ws/tasks
GET    /api/v1/:ws/tasks/:tid
PATCH  /api/v1/:ws/tasks/:tid
DELETE /api/v1/:ws/tasks/:tid
POST   /api/v1/:ws/tasks/:tid/assign
POST   /api/v1/:ws/tasks/:tid/complete
POST   /api/v1/:ws/tasks/:tid/cancel
```

### Jobs and AI Usage

```
GET    /api/v1/:ws/jobs
POST   /api/v1/:ws/jobs/translate
GET    /api/v1/:ws/jobs/:id
DELETE /api/v1/:ws/jobs/:id
GET    /api/v1/:ws/ai-usage
```

### Bravo Agent

```
GET    /api/v1/:ws/bravo/config
PUT    /api/v1/:ws/bravo/config
GET    /api/v1/:ws/bravo/tools
GET    /api/v1/:ws/bravo/usage                          ?from=X&to=Y
GET    /api/v1/:ws/bravo/conversations
POST   /api/v1/:ws/bravo/conversations
GET    /api/v1/:ws/bravo/conversations/:cid
DELETE /api/v1/:ws/bravo/conversations/:cid
POST   /api/v1/:ws/bravo/conversations/:cid/cancel
PATCH  /api/v1/:ws/bravo/conversations/:cid/mode
GET    /api/v1/:ws/bravo/conversations/:cid/messages
POST   /api/v1/:ws/bravo/conversations/:cid/messages
POST   /api/v1/:ws/bravo/conversations/:cid/tool-calls/:tcid/approve
POST   /api/v1/:ws/bravo/conversations/:cid/tool-calls/:tcid/deny
```

### Knowledge Graph

```
GET    /api/v1/:ws/graph/concepts
GET    /api/v1/:ws/graph/nodes/:nodeId/neighbors
GET    /api/v1/:ws/graph/nodes/:nodeId/edges
GET    /api/v1/:ws/graph/shortest-path                  ?from=X&to=Y
```

### Collab (WebSocket, ref-scoped)

```
GET    /api/v1/:ws/:proj/collab/:ref                    ?item=X
```

### Pulse (public dashboard)

```
GET    /api/v1/pulse
GET    /api/v1/pulse/:workspace
GET    /api/v1/pulse/:workspace/projects
GET    /api/v1/pulse/:workspace/projects/:proj
GET    /api/v1/pulse/:workspace/projects/:proj/locales/:locale
GET    /api/v1/pulse/:workspace/activity
GET    /api/v1/pulse/:workspace/activity/heatmap
GET    /api/v1/pulse/:workspace/leaderboard
GET    /api/v1/pulse/:workspace/terms
GET    /api/v1/pulse/:workspace/terms/:cid
```

### Badges

```
GET    /api/v1/badges/:proj
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
- Client uses `_` as the workspace slug: `/api/v1/_/my-project/blocks/main`
- `AuthMiddleware` is replaced with a no-op that injects a synthetic user context
- `WorkspaceAccessMiddleware` is replaced with a no-op
- No route duplication between modes

## Implementation Notes

### Slug Resolution Middleware

Workspace and project slugs are resolved to internal IDs once in middleware, early in the request chain. Downstream handlers receive IDs from the request context, never from URL params directly. This keeps slug-to-ID mapping in one place and allows slugs to be renamed without breaking internal references.

### Ref Resolution

Content routes use `resource/:ref/` where `:ref` is resolved by middleware:

1. Exact stream name match → use stream head (read-write)
2. Exact tag name match → use tag snapshot (read-only)
3. 404

Streams and tags share a single namespace — a tag cannot have the same name as a stream (enforced at creation time, like git). Write operations (PUT block, POST items, sync push) through a tag ref return **409 Conflict**.

Three URL patterns involve refs:

- **`/resource/:ref/`** — content scoping (blocks, items, sync, actions, assets, etc.). `:ref` can be a stream or tag. The resource keyword comes first, like GitHub's `/tree/:ref/`, `/blob/:ref/`.
- **`/streams/:stream`** — stream management CRUD (create, merge, diff, lock/unlock).
- **`/tags/:tag`** — tag management CRUD (create, delete).

### Resource-First Ref Pattern

GitHub avoids a scope prefix for refs by placing a resource keyword before the ref parameter:

```
GitHub:   /:owner/:repo/tree/:ref/*path
Bowrain:  /:ws/:proj/blocks/:ref?item=X
```

The resource keyword (`blocks`, `items`, `sync`, `actions`, `assets`, `collections`, `preview`, `word-count`, `review-queue`, `brand-voice`, `collab`, `dashboard`) is a fixed path segment at the project level. The router matches it before any parameterized segment like `:proj`, so there is no ambiguity. This eliminates the need for an `/s/` or `/r/` prefix while keeping all three slug levels (workspace, project, ref) unambiguous.

The trade-off is a larger reserved project slug list — every ref-scoped resource keyword must be reserved. This is acceptable because these are stable, well-defined resource names that rarely change, and the blocklist is checked only at project creation time.

### Item Path Encoding

File/item paths are passed as `?item=path/to/file.json` using standard URL encoding (`encodeURIComponent`). This replaces the wildcard `/*` suffix pattern which was fragile and framework-specific.

### Actions Convention

Non-CRUD operations live under `actions/`:

- `POST /api/v1/:ws/:proj/actions/:ref/ai-translate`
- `POST /api/v1/:ws/:proj/actions/:ref/export`

This follows the Google Cloud API pattern and makes it unambiguous whether an endpoint is a resource (GET/PUT/DELETE) or an operation (POST to `actions/`).

### Workspace and Project Creation

When creating a workspace or project, the API validates the slug against:

1. The reserved name blocklist (see above)
2. Slug format rules: lowercase alphanumeric + hyphens, 2-64 chars, no leading/trailing hyphens, no consecutive hyphens
3. Uniqueness within the parent scope (workspace slugs globally unique, project slugs unique within workspace)

Slugs can be changed (renamed) — the internal ID remains stable. Clients that cache URLs should handle 301 redirects from old slugs.

## Affected Files

| File                                           | Changes                                                  |
| ---------------------------------------------- | -------------------------------------------------------- |
| `platform/server/server.go`                    | Route registration rewrite                               |
| `platform/server/editor.go`                    | `streamParam()` → `c.Param("ref")`, item path extraction |
| `platform/server/handlers.go`                  | Consolidate `/info` endpoint                             |
| `platform/server/handlers_*.go`                | Param name standardization, slug resolution              |
| `platform/server/handlers_connector.go`        | Scope to workspace, connector ID for fetch/publish       |
| `platform/server/middleware_auth.go`           | Virtual workspace `_` support                            |
| `platform/server/middleware_slug.go`           | New: slug-to-ID resolution middleware                    |
| `platform/server/middleware_ref.go`            | New: ref resolution (stream or tag) middleware           |
| `platform/packages/ui/src/api/rest-adapter.ts` | All URL constructions                                    |
| `platform/packages/ui/src/api/adapter.ts`      | Rename file→item in method signatures                    |
| `platform/core/client/client.go`               | URL prefix and ref handling                              |
| `platform/apps/web/src/api.ts`                 | Login redirect URL                                       |
