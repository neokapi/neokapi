---
id: 015-auth-and-workspaces
sidebar_position: 15
title: "AD-015: Authentication and Workspaces"
---
# AD-015: Authentication and Workspaces

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

Authentication uses a federated OpenID Connect identity provider —
[Keycloak](https://www.keycloak.org/) in production.
OIDC providers support multiple upstream identity sources (GitHub,
Google, LDAP, SAML, etc.) while presenting a single OIDC interface to gokapi.

- **Server mode**: `bowrain-server` starts with OIDC configuration (issuer URL,
  client ID, client secret). The web UI redirects to the OIDC provider for login.
- **Standalone mode**: `kapi serve` runs without authentication on localhost.
  Auth behavior is determined by the `JWTSecret` config: when set, the server
  allows both authenticated and unauthenticated access; when empty, all
  endpoints require JWT authentication.

### JWT Token-based API Auth with HttpOnly Cookies

After OIDC authentication, the server issues a JWT (HMAC-SHA256 signed) containing:
- `sub`: user ID
- `email`: user email
- `name`: display name
- `exp`/`iat`: standard expiry and issued-at claims

**Web apps** receive tokens via HttpOnly cookies (not localStorage or URL
parameters), preventing JavaScript access and reducing XSS attack surface.
**API clients** (CLI, desktop) use the `Authorization: Bearer <token>` header.
All CRUD endpoints require JWT authentication — there are no unauthenticated
project routes.

### Refresh Token Security

Refresh tokens use server-side hashing with single-use rotation:
- Refresh tokens are hashed (SHA-256) before storage in SQLite
- Each refresh grants a new token pair (single-use rotation prevents reuse attacks)
- 30-day expiry window
- Client auto-refresh: 401 responses trigger automatic token refresh

### OAuth Device Flow for CLI

The CLI authenticates using RFC 8628 (Device Authorization Grant):

1. `kapi auth login --server <url>` calls the device auth endpoint
2. Server returns a user code and verification URL
3. User opens the URL in a browser and enters the code
4. CLI polls the token endpoint until the user authorizes
5. Token is stored at `~/.config/bowrain/auth.json`

### PKCE + Authorization Code for Desktop

The Bowrain Desktop app uses OAuth 2.0 Authorization Code with PKCE
(RFC 7636) instead of device flow, providing a more seamless UX:

1. App generates a PKCE code verifier and challenge
2. Opens system browser to `{server}/api/v1/auth/desktop/login` with the challenge
3. User authenticates via Keycloak OIDC in the browser
4. Server redirects to `bowrain://auth/callback` with tokens
5. OS routes the URL protocol to the app's handler
6. Tokens are split: secrets in OS keyring (macOS Keychain, Windows Credential
   Manager, Linux Secret Service), metadata in `<UserConfigDir>/bowrain-desktop/auth.json`

See [AD-020](./020-collaborative-editor.md) for the full desktop connection
architecture.

### Device Auth Pages

Device authorization claim pages (where users enter codes and approve devices)
are hosted in the web app with the same glass UI theme, providing a consistent
visual experience across all authentication flows.

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
| Auth | OIDC + JWT | None (standalone) |
| Workspaces | Multi-workspace, multi-user | Single implicit workspace |
| gRPC | Yes (HTTP port + 1000) | No |
| Use case | Production deployment | Local editing (like `jupyter notebook`) |

The server reports its mode via `GET /api/v1/config` so the web UI can adapt.

### Package Structure

- `platform/auth/` — Auth types, JWT handling, PKCE support, device flow client
- `bowrain/auth/` — `AuthStore` interface, SQLite implementation, OIDC helpers
- `bowrain/server/` — REST/gRPC server, auth middleware, workspace handlers,
  gRPC auth interceptors
- `bowrain/service/auth.go` — `AuthService` business logic
- `kapi/cmd/kapi/auth.go` — `kapi auth login|logout|status`

## Consequences

- All projects are workspace-scoped. The `workspace_id` column links each project
  to its owning workspace. All API routes are workspace-scoped
  (`/api/v1/workspaces/:ws/projects/:id`).
- The web UI, Bowrain desktop, and kapi-web share components via `packages/ui/`
  to avoid divergence across frontends.
- CLI uses device flow (terminal-friendly); desktop uses PKCE (seamless browser
  redirect). Both store tokens securely — CLI in config files, desktop in OS keyring.
- HttpOnly cookies protect web app tokens from JavaScript access. API clients
  use Bearer tokens in headers.
- Refresh token rotation with server-side hashing prevents token reuse attacks.
- Keycloak is the primary OIDC provider in production, with custom-themed
  login pages matching the gokapi visual style.
- An OIDC provider is a deployment dependency for multi-user mode but not
  required for standalone or local use.
