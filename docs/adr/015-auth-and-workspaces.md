---
id: 015-auth-and-workspaces
sidebar_position: 15
title: "ADR-015: Authentication and Workspaces"
---
# ADR-015: Authentication and Workspaces

## Context

gokapi is evolving from a single-user localization toolkit into a multi-user
platform. This requires user authentication, organizational hierarchy, and
access control. The architecture draws inspiration from Speckle's
Workspace > Project model for organizational hierarchy, and Slack's
workspace-switching UI for the frontend experience.

## Decision

### Workspace > Project Hierarchy

**Workspaces** are the top-level organizational unit. Every project belongs to
exactly one workspace. Workspaces contain members with assigned roles.

```
Workspace
├── Members (users with roles)
├── Project A
│   ├── Blocks
│   └── Versions
├── Project B
└── Settings
```

### User Model and Roles

Users are identified by email address (unique) and carry an ID, name, and
avatar URL. Roles within a workspace are hierarchical:

| Role    | Permissions |
|---------|-------------|
| owner   | Full control, delete workspace, manage all members |
| admin   | Manage members, create/delete projects |
| member  | Create/edit content, run flows |
| viewer  | Read-only access |

### Authentication: OIDC

Authentication uses a federated OpenID Connect identity provider such as
[Keycloak](https://www.keycloak.org/) or [Dex](https://dexidp.io/).
OIDC providers support multiple upstream identity sources (GitHub,
Google, LDAP, SAML, etc.) while presenting a single OIDC interface to gokapi.

- **Server mode**: `bowrain-server` starts with OIDC configuration (issuer URL,
  client ID, client secret). The web UI redirects to the OIDC provider for login.
- **Local mode**: `kapi serve` runs without authentication on localhost.

### JWT Token-based API Auth

After OIDC authentication, the server issues a JWT (HMAC-SHA256 signed) containing:
- `sub`: user ID
- `email`: user email
- `name`: display name
- `exp`/`iat`: standard expiry and issued-at claims

API requests include the JWT in the `Authorization: Bearer <token>` header.
The `AuthMiddleware` validates tokens and sets user context.

### OAuth Device Flow for CLI and Desktop

Both the CLI and Bowrain Desktop authenticate using RFC 8628 (Device
Authorization Grant), which works well for environments without a browser
redirect (terminal, native desktop webview):

**CLI flow:**

1. `kapi auth login --server <url>` calls the device auth endpoint
2. Server returns a user code and verification URL
3. User opens the URL in a browser and enters the code
4. CLI polls the token endpoint until the user authorizes
5. Token is stored at `~/.config/kapi/auth.json`

**Desktop flow:**

1. Translator enters the bowrain-server URL and clicks "Connect"
2. Backend calls the device auth endpoint via REST
3. UI displays the user code and a "Open in Browser" button (uses Wails
   `BrowserOpenURL` to launch the verification URL)
4. Backend polls the token endpoint; UI shows a spinner
5. On authorization, the JWT is used to establish a gRPC connection
6. Token and user info are stored at `~/.config/bowrain/desktop-auth.json`
   for auto-reconnect on next launch

### Database Schema

Auth data lives in the same SQLite database as the content store (or a
separate database). Three new tables:

```sql
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    avatar_url TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    logo_url TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE workspace_members (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member',
    joined_at TEXT NOT NULL,
    PRIMARY KEY (workspace_id, user_id)
);
```

The existing `projects` table gains a `workspace_id` column (migration 4)
to associate projects with workspaces.

### Server Modes

| Feature | `bowrain-server` | `kapi serve` |
|---------|-----------------|--------------|
| Binding | 0.0.0.0 (configurable) | 127.0.0.1 |
| Auth | OIDC + JWT | None |
| Workspaces | Multi-workspace, multi-user | Single implicit workspace |
| Use case | Production deployment | Local editing (like `jupyter notebook`) |

The server reports its mode via `GET /api/v1/config` so the web UI can adapt.

### Package Structure

- `core/auth/` — Domain types (`User`, `Workspace`, `Role`, `Membership`),
  `AuthStore` interface, SQLite implementation, JWT handling, OIDC helpers,
  device flow client
- `internal/server/` — REST/gRPC server (refactored from `cmd/bowrain-server/`),
  auth middleware, workspace handlers
- `core/service/auth.go` — `AuthService` business logic
- `cmd/kapi/auth.go` — `kapi auth login|logout|status`
- `cmd/kapi/serve.go` — `kapi serve` local project server

## Consequences

- All projects are now workspace-scoped. Existing databases receive a migration
  that adds `workspace_id` with a default empty string for backward compatibility.
- The web UI and Bowrain desktop share components via `packages/ui/` to avoid
  divergence between the two frontends.
- Device auth flow is shared between CLI and desktop — same REST endpoints,
  same user experience (enter code in browser). Desktop stores credentials
  separately (`desktop-auth.json`) to allow independent sessions.
- An OIDC provider (e.g., Keycloak, Dex) is a deployment dependency for multi-user
  mode but not required for local use or desktop operation.
