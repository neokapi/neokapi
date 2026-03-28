---
id: 039-admin-control-plane
sidebar_position: 39
title: "AD-039: Admin Control Plane"
---
# AD-039: Admin Control Plane

## Context

The Bowrain platform needs an internal control plane for platform operators to manage workspaces, users, billing, and provide customer support. This is separate from the customer-facing app — operators authenticate against a dedicated Keycloak realm (`bowrain-admin`) and access admin-only API endpoints.

Key requirements:
- View and manage all workspaces across the platform
- Impersonate customers for debugging and support (with audit trail)
- Add/remove workspace members directly
- Manage billing: change plans, grant credits, set feature overrides
- Monitor platform health: metrics, upsell signals, billing events

## Decision

### Separate authentication realm

The control plane uses a dedicated Keycloak realm (`bowrain-admin`), completely isolated from the customer realm (`bowrain`). Admin tokens are verified by `AdminGuard` middleware which checks the `bowrain-admin` realm's OIDC issuer. Regular app users cannot access admin endpoints.

### Admin API namespace

All admin endpoints live under `/api/admin/*` with the `AdminGuard` middleware applied to the group. The admin API shares the same bowrain-server process — no separate admin service.

### Customer impersonation

Operators can "View as Customer" from the workspace detail page. This creates a **short-lived API token** (1 hour) scoped to the target workspace, impersonating the workspace owner. The token is a standard `bwt_` API token — no special middleware needed.

Every impersonation is **audited** via an internal workspace note recording the admin's email, timestamp, and token prefix. The customer app URL is derived from the admin's Origin header (`ctrl.dev.bowrain.cloud` → `dev.bowrain.cloud`).

### Member management

Admins can add users to any workspace with a specific role. The admin add-member endpoint checks for existing membership — if the user is already a member with a different role, it updates the role instead of failing. All member changes are audited as workspace notes.

### User search

The admin user search uses `ILIKE` pattern matching on both `name` and `email` fields, enabling fuzzy search (e.g., searching "asgeir" finds "asgeirf@gmail.com"). This is admin-only — the customer app's member management uses exact email matching.

### Activity read state

Activity indicators (the blue dot showing new activity) use **server-side cursor tracking** rather than client-side localStorage. An `activity_state` table stores `last_seen_at` per user+workspace. The activity list endpoint returns `new_count` (activities newer than the cursor), and a dedicated `POST /activities/seen` endpoint updates the cursor. This syncs across all devices.

### Frontend architecture

The control plane is a standalone React app (`platform/apps/ctrl/`) using TanStack Router + React Query. It communicates with the same bowrain-server via `/api/admin/*` endpoints. The API base URL is derived from the hostname: `ctrl.dev.bowrain.cloud` → `dev.bowrain.cloud/api/admin`.

### Deployment

- **Dev**: `ctrl.dev.bowrain.cloud` — static assets deployed to Azure Storage
- **Prod**: `ctrl.bowrain.cloud`
- Both routed through Azure Front Door alongside the main app
- MFA required in prod (CONFIGURE_TOTP as default action), optional in dev
