---
sidebar_position: 24
title: "Admin Control Plane"
---

# Admin Control Plane

Implementation details for [AD-039](/docs/ad/039-admin-control-plane).

## Admin API Endpoints

All routes require `AdminGuard` middleware (Keycloak `bowrain-admin` realm).

### Workspace Management

| Method | Path                                    | Handler                     | Purpose                                                 |
| ------ | --------------------------------------- | --------------------------- | ------------------------------------------------------- |
| GET    | `/api/admin/workspaces`                 | `HandleAdminListWorkspaces` | List all workspaces (search, filter by plan/status)     |
| GET    | `/api/admin/workspaces/:id`             | `HandleAdminGetWorkspace`   | Full workspace detail: plan, credits, members, activity |
| PUT    | `/api/admin/workspaces/:id/plan`        | `HandleAdminUpdatePlan`     | Change workspace plan                                   |
| POST   | `/api/admin/workspaces/:id/credits`     | `HandleAdminGrantCredits`   | Grant bonus credits                                     |
| POST   | `/api/admin/workspaces/:id/impersonate` | `HandleAdminImpersonate`    | Create 1-hour impersonation token                       |
| POST   | `/api/admin/workspaces/:id/members`     | `HandleAdminAddMember`      | Add/update workspace member                             |

### Feature Overrides & Notes

| Method | Path                                          | Handler                          | Purpose        |
| ------ | --------------------------------------------- | -------------------------------- | -------------- |
| GET    | `/api/admin/workspaces/:id/feature-overrides` | `HandleAdminGetFeatureOverrides` | List overrides |
| PUT    | `/api/admin/workspaces/:id/feature-overrides` | `HandleAdminSetFeatureOverrides` | Set override   |
| GET    | `/api/admin/workspaces/:id/notes`             | `HandleAdminGetNotes`            | Internal notes |
| POST   | `/api/admin/workspaces/:id/notes`             | `HandleAdminAddNote`             | Add note       |
| GET    | `/api/admin/workspaces/:id/ledger`            | `HandleAdminGetLedger`           | Credit ledger  |

### Users & Platform

| Method | Path                   | Handler                    | Purpose                              |
| ------ | ---------------------- | -------------------------- | ------------------------------------ |
| GET    | `/api/admin/users`     | `HandleAdminListUsers`     | Search users (ILIKE on name + email) |
| GET    | `/api/admin/users/:id` | `HandleAdminGetUser`       | User detail + workspace memberships  |
| GET    | `/api/admin/metrics`   | `HandleAdminGetMetrics`    | Platform KPIs                        |
| GET    | `/api/admin/events`    | `HandleAdminListEvents`    | Billing events feed                  |
| GET    | `/api/admin/upsells`   | `HandleAdminGetUpsells`    | Upsell opportunities                 |
| GET    | `/api/admin/overrides` | `HandleAdminListOverrides` | All feature overrides                |

## Impersonation Flow

```
Admin clicks "View as Customer" in ctrl
  → POST /api/admin/workspaces/:id/impersonate
    1. Look up workspace + find owner
    2. CreateAPIToken(ownerID, wsID, "admin-impersonation", 1h expiry)
    3. AddNote("Admin impersonation by admin@... — token bwt_xxxx expires ...")
    4. Derive app URL from Origin header (ctrl.dev.bowrain.cloud → dev.bowrain.cloud)
    5. Return { url, token, expires_at }
  → Frontend copies token to clipboard, opens URL in new tab
```

The token is a standard `bwt_` API token validated by the existing `handleAPIToken` middleware. No special impersonation middleware needed.

## Activity Read State Schema

```sql
CREATE TABLE activity_state (
    user_id      TEXT NOT NULL,
    workspace_id TEXT NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, workspace_id)
);
```

**Methods** (on `ActivityStore`):

- `GetActivitySeenAt(ctx, userID, workspaceID)` → `time.Time`
- `SetActivitySeenAt(ctx, userID, workspaceID, seenAt)` → `error`
- `CountNewActivities(ctx, userID, workspaceID)` → `int`

**API**: `GET /activities` returns `new_count` field. `POST /activities/seen` updates cursor.

**Frontend**: `ActivityIndicator` receives `newCount` prop. `TopBar` calls `markActivitiesSeen()` when dropdown opens, then invalidates the activities query.

## Member Add/Update Logic

```go
// Check if already a member — update role instead of failing on duplicate key.
if existing, err := s.AuthStore.GetMembership(ctx, wsID, userID); err == nil && existing != nil {
    if existing.Role == role {
        return 409 Conflict "already a member with this role"
    }
    s.AuthStore.UpdateRole(ctx, wsID, userID, role)  // update
} else {
    s.AuthStore.AddMember(ctx, wsID, userID, role)   // insert
}
```

## Key Files

| File                                                    | Purpose                                     |
| ------------------------------------------------------- | ------------------------------------------- |
| `platform/server/handlers_admin.go`                     | All admin API handlers                      |
| `platform/server/server.go` (~line 850)                 | Admin route registration                    |
| `platform/billing/middleware.go`                        | `AdminGuard` middleware                     |
| `platform/apps/ctrl/src/`                               | Control plane React app                     |
| `platform/apps/ctrl/src/api.ts`                         | Admin API client                            |
| `platform/apps/ctrl/src/routes/workspace-detail.tsx`    | Workspace detail + impersonate + add member |
| `platform/apps/ctrl/src/components/AddMemberDialog.tsx` | User search + role dialog                   |
| `platform/store/activity.go`                            | Activity read state (Get/Set/Count)         |
