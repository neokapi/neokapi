---
id: 011-rest-api
sidebar_position: 11
title: "AD-011: REST API"
---

# AD-011: REST API

## Summary

The bowrain-server REST API is a slug-based, resource-first hierarchy
rooted at `/api/v1/:workspace/:project/`. Refs (streams or tags) are
scoped inline after the resource keyword (`blocks/:ref`, `sync/:ref`,
`actions/:ref/…`). gRPC services share the same port via h2c protocol
detection. Sync endpoints use protobuf; everything else uses JSON.

## Context

The server exposes roughly 100 endpoints covering identity, content
management, sync, automation, tasks, notifications, billing, admin
operations, and a public dashboard. At that scale, consistency and
predictability are the only defensible design axes — every new endpoint
has to "fit" without requiring a lookup. The API design mirrors the
conventions already used by GitHub (`/:owner/:repo/tree/:ref`) and
Google Cloud (RPC-style actions under `:verb`), so integrators
recognize the shape before reading the docs.

## Decision

### Design principles

1. **Slug-based hierarchy.** Workspaces and projects appear as bare
   path segments: `/api/v1/:ws/:proj/…`. No `/w/` or `/p/` prefix.
2. **Reserved name blocklist.** Fixed route segments are reserved at
   workspace and project creation time so user slugs never collide with
   the router.
3. **Resource-first refs.** Content routes place the resource keyword
   before the ref: `/:ws/:proj/blocks/:ref`, `/:ws/:proj/sync/:ref/push`.
   No `/s/` or `/r/` prefix is needed — the resource keyword
   disambiguates from non-ref project routes (`members`, `settings`).
4. **RPC actions under `/actions/`.** Non-CRUD verbs live under
   `POST /:ws/:proj/actions/:ref/<verb>`.
5. **Item paths as query params.** File paths pass as
   `?item=path/to/file.json` using standard URL encoding instead of
   wildcard `/*` suffixes.
6. **All identifiers are slugs.** Workspaces, projects, streams, and
   tags use human-readable slugs in URLs. Slug-to-ID resolution happens
   once in middleware.
7. **Standalone mode uses `_`.** In self-hosted single-user deployments,
   clients address the virtual workspace `_`. Route registration stays
   identical between modes.

### Reserved names

Workspace slugs cannot match top-level API segments. Project slugs
cannot match workspace-level segments. The blocklist is validated at
creation time alongside slug format rules (lowercase alphanumeric plus
hyphens, 2–64 characters).

**Reserved workspace slugs:** `auth`, `admin`, `health`, `ready`,
`info`, `pulse`, `badges`, `join`, `webhooks`, `workspaces`, `projects`,
`connectors`, `_`.

**Reserved project slugs:** `members`, `invites`, `roles`, `tokens`,
`billing`, `audit-log`, `providers`, `terms`, `translation-memory`,
`connectors`, `tasks`, `jobs`, `ai-usage`, `bravo`, `graph`,
`activities`, `notifications`, `notification-preferences`,
`digest-settings`, `brand-profiles`, `archived-projects`, `projects`,
`streams`, `tags`, `refs`, `blocks`, `items`, `sync`, `actions`,
`assets`, `collections`, `preview`, `word-count`, `review-queue`,
`brand-voice`, `collab`, `dashboard`, `automations`, `settings`.

### Ref resolution

Content routes use `/resource/:ref/…` where `:ref` is resolved in
middleware:

1. Exact stream name match → stream head (read-write).
2. Exact tag name match → tag snapshot (read-only).
3. Otherwise, 404.

Streams and tags share one namespace, so a tag cannot reuse a stream's
name (enforced at creation). Writes through a tag ref return `409 Conflict`.

Three URL patterns involve refs:

- **Stream refs** — `/blocks/main`, `/sync/main/…`, `/actions/main/…`.
- **Tag refs** — identical syntax; `/blocks/v1.2.0` returns the
  snapshot.
- **Management CRUD** — `/streams/:stream` and `/tags/:tag` are peers
  in the ref namespace.

### Route categories

The server exposes approximately 100 endpoints across the following
categories. Each item below is a category, not an exhaustive list.

| Category          | Scope            | Examples                                                      |
| ----------------- | ---------------- | ------------------------------------------------------------- |
| Health / info     | public           | `/health`, `/ready`, `/info`, `/badges/:proj`                |
| Auth              | mixed            | `/auth/device/*`, `/auth/callback`, `/auth/desktop/*`        |
| Workspace         | `/:ws/…`         | `/workspaces`, `/:ws/members`, `/:ws/roles`, `/:ws/tokens`   |
| Project           | `/:ws/:proj`     | CRUD, settings, members, audit log                            |
| Content           | `/:ws/:proj`     | `items/:ref`, `blocks/:ref`, `blocks/:ref/:bid/*`            |
| Sync              | `/:ws/:proj`     | `sync/:ref/pull`, `sync/:ref/push/{init,diff,commit,chunks}` |
| Actions (RPC)     | `/:ws/:proj`     | `actions/:ref/{ai-translate,pseudo-translate,export,qa-check}` |
| Streams / tags    | `/:ws/:proj`     | `streams/:stream/{merge,diff,lock}`, `tags/:tag`             |
| Collections       | `/:ws/:proj`     | `collections/:ref`, `collections/:ref/:cid/items`            |
| Assets            | `/:ws/:proj`     | `assets/:ref`, `assets/:ref/:aid/variants`                   |
| Review queue      | `/:ws/:proj`     | `review-queue/:ref/:itemId/{decide,assign,split}`            |
| Preview           | `/:ws/:proj`     | `preview/:ref`, `word-count/:ref`                            |
| Dashboard         | `/:ws/:proj`     | `dashboard/:ref`                                              |
| Brand voice       | `/:ws/:proj`     | `brand-voice/:ref/{scores,trends,corrections}`               |
| Collab            | `/:ws/:proj`     | `collab/:ref` (WebSocket)                                    |
| Connectors        | `/:ws`           | `connectors/:id/{fetch,publish,status}`                      |
| Automations       | `/:ws/:proj`     | Rules + `/automations/runs/:runId/*`                          |
| TM                | `/:ws`           | `translation-memory`, `translation-memory/:eid`              |
| Terms             | `/:ws`           | `terms`, `terms/import`, `terms/export`                      |
| Providers         | `/:ws`           | `providers`, `providers/test`                                 |
| Brand profiles    | `/:ws`           | `brand-profiles`, `brand-profiles/:id/check`                 |
| Notifications     | `/:ws`           | `notifications`, `notification-preferences`, `digest-settings` |
| Activities        | `/:ws`           | `activities`, `activities/seen`                              |
| Tasks             | `/:ws`           | `tasks`, `tasks/:tid/{assign,complete,cancel}`               |
| Jobs / AI usage   | `/:ws`           | `jobs`, `jobs/translate`, `ai-usage`                         |
| Bravo             | `/:ws`           | `bravo/{config,conversations,tools,usage}`                   |
| Graph             | `/:ws`           | `graph/{concepts,nodes,shortest-path}`                       |
| Pulse             | public           | `/pulse/:ws/{projects,activity,leaderboard,terms}`           |
| Admin             | `/api/admin/…`   | `workspaces`, `users`, `metrics`, `events`, `overrides`, `upsells` |
| Webhooks          | `/api/webhooks/…`| `/api/webhooks/stripe`                                        |

### Items and blocks

Items are the generic unit of tracked content — a file, a CMS document,
a connector object. Blocks are the translatable segments within an
item.

```
GET    /:ws/:proj/items/:ref
POST   /:ws/:proj/items/:ref                         (multipart upload)
DELETE /:ws/:proj/items/:ref                         ?item=path/to/file.json

GET    /:ws/:proj/blocks/:ref                        ?item=path/to/file.json
PUT    /:ws/:proj/blocks/:ref/:bid
PUT    /:ws/:proj/blocks/:ref/:bid/coded

GET    /:ws/:proj/blocks/:ref/:bid/history           ?locale=fr
GET    /:ws/:proj/blocks/:ref/:bid/notes
POST   /:ws/:proj/blocks/:ref/:bid/notes
GET    /:ws/:proj/blocks/:ref/:bid/tm-matches        ?locale=fr
GET    /:ws/:proj/blocks/:ref/:bid/term-matches      ?locale=fr
GET    /:ws/:proj/blocks/:ref/:bid/html              ?locale=fr
```

### Sync endpoints

The bowrain CLI ([AD-010](010-bowrain-cli-and-project-model.md)) drives
sync via a small set of ref-scoped endpoints:

```
GET    /:ws/:proj/sync/:ref/pull
GET    /:ws/:proj/sync/:ref/blocks
GET    /:ws/:proj/sync/:ref/status
POST   /:ws/:proj/sync/:ref/push/init
POST   /:ws/:proj/sync/:ref/push/diff
POST   /:ws/:proj/sync/:ref/push/commit
PUT    /:ws/:proj/sync/:ref/push/chunks/:uploadId/:chunkIndex
POST   /:ws/:proj/sync/:ref/translate
```

Sync uses protobuf-encoded payloads for the hot path (push chunks, pull
deltas) per [AD-009](009-sync-protocol.md). Everything else —
administration, task management, automation configuration — uses JSON.

Flat routes at `/api/v1/projects/:proj/sync/:ref/…` serve anonymous
projects that have not been claimed into a workspace; they accept claim
tokens instead of JWTs.

### Actions

Non-CRUD operations use the `actions/` prefix, following Google Cloud
API conventions:

```
POST /:ws/:proj/actions/:ref/pseudo-translate  { item, target_locale }
POST /:ws/:proj/actions/:ref/ai-translate      { item, target_locale, ... }
POST /:ws/:proj/actions/:ref/tm-translate      { item, target_locale }
POST /:ws/:proj/actions/:ref/export            { item, target_locale }
POST /:ws/:proj/actions/:ref/qa-check          { item, target_locale }
POST /:ws/:proj/actions/:ref/qa-check-block    { block_id, locale }
```

This pattern makes it unambiguous at a glance whether an endpoint is a
resource (`GET`/`PUT`/`DELETE` on a path) or an operation (`POST` under
`actions/`).

### Slug resolution middleware

Slugs resolve to internal IDs once, early in the request chain, and
downstream handlers receive IDs from request context — never from URL
params directly. This keeps slug-to-ID mapping in one place and allows
slugs to be renamed without breaking internal references. Cached URLs
handle `301` redirects from old slugs.

### gRPC multiplexing

The Bowrain desktop app ([AD-017](017-bowrain-apps.md)) communicates via
a dedicated `EditorService` gRPC API. gRPC and HTTP share the same port
using h2c (cleartext HTTP/2) protocol detection:

- Requests with `Content-Type: application/grpc` route to the gRPC
  server.
- All other requests route to the Echo HTTP router.

Both protocols use the same JWT authentication — `authorization: Bearer <token>`
on REST requests, the same header in gRPC metadata.

### Standalone mode

Self-hosted single-user deployments (`kapi serve`, or bowrain-server
with no `JWTSecret`) use the virtual workspace slug `_`:

- `/api/v1/_/my-project/blocks/main` is a valid standalone request.
- `AuthMiddleware` becomes a no-op that injects a synthetic user.
- `WorkspaceAccessMiddleware` becomes a no-op.
- Route registration is unchanged — no duplication between modes.

### Item path encoding

File and item paths pass as `?item=path/to/file.json` using standard URL
encoding (`encodeURIComponent`). This avoids wildcard `/*` suffixes,
which are fragile across HTTP frameworks and interact poorly with
ref-scoped routing.

## Consequences

- New endpoints fit into the existing shape without design debate — the
  resource-first, ref-scoped pattern answers the question before it is
  asked.
- Clients can construct URLs mechanically from `(workspace, project,
  ref, resource)` tuples.
- Standalone and multi-tenant deployments share the same route table.
- Router match order (static segments before parameterized ones) makes
  the reserved-slug lists trivial to enforce.
- gRPC and REST share a single port, which simplifies TLS termination,
  load balancers, and firewall rules.

## Related

- [AD-009: Sync Protocol](009-sync-protocol.md) — protobuf wire format for sync
- [AD-010: Bowrain CLI and Project Model](010-bowrain-cli-and-project-model.md) — primary consumer of sync endpoints
- [AD-017: Bowrain Apps](017-bowrain-apps.md) — gRPC EditorService consumer
- [AD-014: Translator Workflow](014-translator-workflow.md) — tasks, activities, notifications endpoints
- [AD-018: Billing and Plans](018-billing-and-plans.md) — admin and billing route families
