---
id: 017-bowrain-apps
sidebar_position: 17
title: "AD-017: Bowrain Apps"
---

# AD-017: Bowrain Apps

## Summary

The bowrain platform ships four first-party applications: a Wails v3
**Desktop app** for translators and project managers; a
**Collaborative Editor** gRPC surface that powers real-time editing;
**Pulse**, a public activity dashboard; and the **Admin Control
Plane** (`ctrl.bowrain.cloud`) for platform operators. All four reuse
the shared `@neokapi/ui-primitives` component library and communicate
with bowrain-server through the same REST and gRPC APIs.

## Context

Different audiences need different surfaces onto the same data model.
Translators want a native editing experience with presence and offline
support. Public communities want a shareable progress page. Operators
need an internal tool for customer support and platform health. Each
app has its own deployment, authentication flow, and feature set, but
sharing components keeps them visually and behaviorally coherent.

## Decision

### 1. Bowrain Desktop (`bowrain/apps/bowrain/`)

A native app built with Wails v3, a React 19 + Vite + TailwindCSS +
shadcn/ui frontend, and a Go backend that proxies to bowrain-server.

**Connector-driven workflow.** The primary flow is Browse CMS → Select
items → Pull → Translate → Push. Connectors
([AD-008: Connector System](008-connector-system.md)) provide the integration; the desktop app
renders the same UI regardless of whether content came from a CMS, a
design tool, a git repository, or the local filesystem.

**Two editor views.**

| View       | Layout                                                                |
| ---------- | --------------------------------------------------------------------- |
| **Visual** | An inline editing card over a formatted live document preview         |
| **Table**  | Source + target columns, many blocks visible at once                  |

The three per-file surfaces — Translate, Review, and Pre-process — share these
views. The context panel shows TM matches, terminology highlights, display
hints, and ContentRef links alongside the current block.

**Flow editor.** A drag-and-drop visual editor built on `@xyflow/react`
and packaged as the `@neokapi/flow-editor` shared library. Color-coded
node types, validation, and template support.

**Terminology management.** Faceted search, concept editing, TBX /
CSV / JSON import and export, analytics, editor integration.

**TM explorer.** Entry browsing with fuzzy match visualization, entity
mapping display, and TMX import/export.

**Slack-like workspace rail.** A 60 px workspace rail plus a 220 px
main sidebar for navigation within the active workspace.

**Component library.** Core UI primitives live in `packages/ui/` as
`@neokapi/ui-primitives` — shadcn primitives (`Button`, `Card`,
`Badge`, `Label`, `Input`, `Tabs`, `ScrollArea`, …) and layout
primitives (`PageHeader`, `EmptyState`, `SkeletonCard`, `PanelHeader`,
`LoadingSpinner`). The web app, desktop app, and Pulse all consume
this library via npm workspace.

**Offline-first.** Local SQLite cache plus an exponential-backoff
reconnection loop (2 s → 60 s). All mutations that fail due to network
errors queue in a `pending_changes` SQLite table and replay in FIFO
order on reconnection.

**Embedded translation UI.** For design tools and CMS platforms, a
lightweight Bowrain panel can embed within a host application as a
WebView. The host's connector provides a bidirectional message
channel, so the translator sees live source and can edit translations
in context.

### 2. Collaborative Editor (gRPC `EditorService`)

The desktop app talks to bowrain-server through a dedicated gRPC
service organized into seven categories that cover the full editing
surface.

| Category             | RPCs                                                                                                               |
| -------------------- | ------------------------------------------------------------------------------------------------------------------ |
| **Auth & workspace** | `GetCurrentUser`, `ListWorkspaces`                                                                                 |
| **Projects**         | `ListEditorProjects`, `GetEditorProject`                                                                           |
| **Blocks**           | `GetBlocks`, `UpdateBlockTarget`, `ReviewBlock`                                                                    |
| **Context**          | `LookupTMForBlock`, `LookupTermsForBlock`                                                                           |
| **TM CRUD**          | `GetTMEntries`, `GetTMCount`, `AddTMEntry`, `UpdateTMEntry`, `DeleteTMEntry`                                       |
| **Terminology**      | `GetTerms`, `GetTermCount`, `AddConcept`, `UpdateConcept`, `DeleteConcept`, `ImportTermsCSV`, `ImportTermsJSON`, `ExportTermsJSON` |
| **Real-time**        | `WatchProject` (server-streaming), `UpdatePresence`                                                                |

**Real-time collaboration.**

- `WatchProject` is a server-streaming RPC that opens when the user
  navigates to a project. It delivers `BlockChangeEvent` (created /
  updated / deleted with the editor's name) and `PresenceChangeEvent`
  (user joined / moved to block / left), plus the broader change events
  the server's [change relay](/server/collaboration) fans out —
  `ProjectChangeEvent`, `ConnectorSyncEvent`, `FlowEventEvent`,
  `MembershipChangeEvent`, and more — so no desktop view goes stale when
  content changes from outside it. The frontend's `useBackendEvents` bus
  translates these into targeted refetches, and re-runs every refreshable
  listener after an offline gap on reconnect.
- `UpdatePresence` is fire-and-forget unary — the client sends it when
  focus moves between blocks; the server broadcasts to all watchers of
  the same project.

**Connection state machine.**

```
disconnected ──StartLogin──→ connecting ──ConnectToServer──→ connected
                                                                │
                                                         (connection lost)
                                                                ▼
                                                             offline
                                                       (reconnect loop)
                                                                ▼
                                                            connected
```

**Offline queue.** When writes fail, the mutation lands in
`pending_changes`:

```sql
CREATE TABLE pending_changes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    operation  TEXT NOT NULL,
    payload    TEXT NOT NULL DEFAULT '{}',
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    attempts   INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT ''
);
```

Queued operations: `UpdateBlockTarget`, `ReviewBlock`, `AddTMEntry`,
`UpdateTMEntry`, `AddConcept`, `UpdateConcept`. On reconnection the
queue replays in FIFO order. Replay stops on first failure to preserve
ordering.

**Port discovery.** The gRPC port is HTTP port + 1000 — connect to
the web URL and the desktop backend discovers gRPC automatically
(e.g., `localhost:8080` → `localhost:9080`). TLS is auto-detected from
the URL scheme.

**Authentication.** JWT in gRPC metadata (`authorization: Bearer <token>`).
The same tokens used for REST API access work for gRPC. Tokens are
stored split: secrets in the OS keyring (Keychain, Credential Manager,
Secret Service), metadata in `<UserConfigDir>/bowrain-desktop/auth.json`.
Desktop auth uses OAuth 2.0 Authorization Code + PKCE with a
`bowrain://auth/callback` URL handler.

### 3. Pulse (`bowrain/apps/pulse/`)

A public activity dashboard deployed at `pulse.bowrain.cloud`.
Standalone Vite SPA, served by bowrain-server when the `Host` header
matches `pulse.*`.

**Dashboard visibility.**

```go
type DashboardVisibility string

const (
    DashboardPrivate  DashboardVisibility = "private"   // workspace members only
    DashboardUnlisted DashboardVisibility = "unlisted"  // accessible via direct URL, not indexed
    DashboardPublic   DashboardVisibility = "public"    // listed, indexed, discoverable
)
```

Both `Workspace.DashboardVisibility` and `Project.DashboardVisibility`
default to `private`. Unlisted responses carry `X-Robots-Tag: noindex`.
Non-accessible resources return `404` (not `403`) to prevent
enumeration.

**Content surfaces.**

- **Contributor leaderboards** — names, avatars, translation and review
  counts, per-language breakdown. Community recognition is the default
  because open-source projects expect it.
- **Terminology explorer** — opt-in filtering lets workspaces publish
  glossary terms without exposing proprietary brand vocabulary.
  `PulseTermSources` on the workspace controls which concept sources
  appear.
- **Activity feed** — recent translation, review, push, and milestone
  activity, public filter.

**Contributor opt-out.** Individual contributors can set
`pulse_visible = false` on their profile. Their contributions still
count toward aggregate stats, but names and avatars replace with
"Anonymous Contributor" on leaderboards and feeds.

**URL-first filters.** All filter state lives in query parameters so
links are shareable, bookmarkable, and navigable with browser
back/forward. The `PulseFilterContext` reads initial state from
`useLocation().search` and calls `navigate` on every filter change.

**Caching.** TTL caching per endpoint with event-bus invalidation:

| Endpoint           | TTL    |
| ------------------ | ------ |
| Workspace overview | 5 min  |
| Leaderboard        | 10 min |
| Activity feed      | 1 min  |
| Terminology        | 15 min |
| Project detail     | 2 min  |

Cache keys include workspace slug, endpoint, and normalized query
parameters. HTTP `Cache-Control: public, max-age=60` on overview
responses allows CDN edge caching for extreme traffic.

**API surface.**

```
GET /api/v1/pulse/:ws
GET /api/v1/pulse/:ws/projects
GET /api/v1/pulse/:ws/projects/:pid
GET /api/v1/pulse/:ws/projects/:pid/lang/:locale
GET /api/v1/pulse/:ws/activity
GET /api/v1/pulse/:ws/leaderboard
GET /api/v1/pulse/:ws/terms
GET /api/v1/pulse/:ws/terms/:cid
```

### 4. Admin Control Plane (`bowrain/apps/ctrl/`)

An internal ops dashboard at `ctrl.bowrain.cloud`, used for customer
support and platform operations.

**Separate Keycloak realm.** The control plane authenticates against
a dedicated `bowrain-admin` realm, fully isolated from the customer
`bowrain` realm:

```
Keycloak
├── bowrain realm          → customers (app.bowrain.cloud)
│   ├── registration: open
│   └── identity providers: Google, GitHub
└── bowrain-admin realm    → operators (ctrl.bowrain.cloud)
    ├── registration: disabled (invite-only)
    ├── MFA: required in prod
    └── identity providers: none (email/password)
```

Realm isolation means admin accounts cannot leak into the customer
realm, and no `super_admin` column pollutes the user model — identity
is determined by which realm issued the JWT.

**`AdminGuard` middleware.** Every `/api/admin/*` route sits behind
middleware that validates the JWT against the admin realm's OIDC
issuer. Regular app users cannot access admin endpoints even with a
valid customer JWT.

**Customer impersonation.** "View as Customer" from a workspace detail
page creates a **short-lived API token** (1 hour) scoped to the target
workspace, impersonating the workspace owner. The token is a standard
`bwt_` API token — no new middleware needed. Every impersonation is
audited as an internal workspace note recording the admin's email,
timestamp, and token prefix.

**Workspace management.** Search and filter workspaces across the
platform; view subscription (plan, status, period, Stripe link),
credit balance and ledger, members, recent activity, and usage charts.
Actions: change plan, grant credits, feature overrides, internal notes.

**Member management.** Admins can add users to any workspace with a
role. The endpoint checks for existing membership — if the user is
already a member with a different role, it updates the role instead of
failing. User search uses `ILIKE` pattern matching on `name` and
`email` for fuzzy lookup.

**Feature overrides.** Per-workspace overrides enable or disable
features outside the plan matrix
([AD-018](018-billing-and-plans.md)). Overrides support optional
expiry — expired entries are ignored and cleaned by a periodic job.

**Activity read state.** The blue-dot "new activity" indicator uses
server-side cursor tracking in an `activity_state` table (per
user + workspace `last_seen_at`), not client-side local storage. The
list endpoint returns `new_count`; `POST /:ws/activities/seen` updates
the cursor. State syncs across devices.

**Stack.** React + TanStack Router + React Query + `@neokapi/ui`.
Reuses the same `AppShell` layout as the customer app, with an
admin-specific sidebar (Dashboard, Workspaces, Users, Events,
Overrides, Upsells). Deployed as static assets behind Azure Front
Door; the API base URL derives from the hostname
(`ctrl.dev.bowrain.cloud` → `dev.bowrain.cloud/api/admin`).

## Consequences

- The platform looks and feels consistent across surfaces because they
  share one component library.
- Each app can evolve independently — a desktop release does not
  require a ctrl release.
- Bowrain Server is the single source of truth; all four apps are
  clients of the same REST and gRPC endpoints.
- Offline-first desktop editing keeps translators productive on
  flights, in tunnels, and during server maintenance.
- Pulse gives the open-source community the visibility they expect
  from localization platforms without compromising private workspaces.
- Admin realm isolation keeps operator credentials out of the customer
  identity plane and vice versa.

## Related

- [AD-003: Identity and Permissions](003-permissions.md) — JWT, realms, roles
- [AD-008: Connector System](008-connector-system.md) — content source abstraction
- [AD-011: REST API](011-rest-api.md) — server endpoints consumed by the apps
- [AD-014: Translator Workflow](014-translator-workflow.md) — tasks, activities, notifications surfaced in the UI
- [AD-016: Bravo Agent](016-bravo-agent.md) — assistant panel in the desktop and web apps
- [AD-018: Billing and Plans](018-billing-and-plans.md) — admin control plane managing plans and overrides
