---
id: 002-authentication-and-workspaces
sidebar_position: 2
title: "AD-002: Authentication and Workspaces"
---

# AD-002: Authentication and Workspaces

## Summary

Bowrain organizes tenancy as a two-level hierarchy: workspaces contain
projects, members, connectors, and all project-scoped data. Identity is
federated through Keycloak, which bridges OIDC to GitHub, Google, LDAP, SAML,
and other identity sources. The server issues short-lived JWTs signed with
HMAC-SHA256; long-lived refresh tokens are rotated and hashed server-side.
Web apps get tokens via HttpOnly cookies, desktop apps via PKCE with OS
keyring storage, and CLIs via RFC 8628 device authorization.

## Context

Bowrain is a multi-user platform. A localization team's translators,
reviewers, developers, project admins, and AI agents all need to access the
same content with different levels of trust. Single-sign-on is mandatory for
enterprise deployments, and identity needs to federate to whatever directory
the customer already uses.

Access is organized around projects, but projects alone are not the isolation
unit: customers want multiple projects to share members, connectors, and
billing. Workspaces provide that grouping, and every API route is
workspace-scoped so multi-tenancy is enforced at the transport layer.

## Decision

### Workspace > Project Hierarchy

Workspaces are the top-level organizational unit. Every project belongs to
exactly one workspace. Workspaces own members (with roles), projects,
connectors, API tokens, billing, and default settings. Workspace slugs are
unique across the deployment.

```
Workspace
├── Members (users with roles)
├── Projects
│   ├── Blocks / Versions / Streams
│   └── Project Members (optional, narrows workspace role)
├── Connectors
├── API Tokens
└── Settings (brand, tags, defaults)
```

Every workspace-scoped REST route has the shape
`/api/v1/workspaces/:ws/...`. The `WorkspaceAccessMiddleware` verifies the
caller is a member before any handler runs.

### Roles

Workspace roles are hierarchical:

| Role   | Permissions                                                       |
| ------ | ----------------------------------------------------------------- |
| owner  | Full control, delete workspace, manage billing, manage all members |
| admin  | Manage members and projects, configure connectors                 |
| member | Create and edit content, run flows                                |
| viewer | Read-only access                                                  |

Workspace roles are the default ceiling; project memberships narrow them
further via role templates and language scopes. See
[AD-003: Permissions and Access Control](003-permissions.md).

### Identity Federation via Keycloak

A Keycloak instance fronts all identity. The customer-facing realm is
`bowrain`; Keycloak bridges upstream identity providers (GitHub, Google,
LDAP, SAML, corporate Azure AD) and exposes a single OIDC interface to
`bowrain-server`.

The admin realm is `bowrain-admin`, separate from the customer realm. The
admin control plane lives in its own app (`bowrain/apps/ctrl/`) and
authenticates against the admin realm only. This keeps customer identities
and admin identities fully isolated.

Keycloak login pages are themed to match Bowrain visual style via a custom
theme in `bowrain/apps/keycloak-theme/`.

### JWT Session Tokens

After OIDC authentication, the server issues a JWT signed with HMAC-SHA256.
Claims:

- `sub` — user ID
- `email` — user email
- `name` — display name
- `exp` — expiry (short, typically 15 minutes)
- `iat` — issued-at

Access tokens are short-lived. Refresh tokens (30-day expiry) rotate on every
use; the server stores a SHA-256 hash of each refresh token, not the token
itself. Any attempted reuse of an already-rotated refresh token invalidates
the entire session family — single-use rotation prevents replay and stolen
refresh attacks.

### Token Delivery

| Client type   | Delivery mechanism                                                                      |
| ------------- | --------------------------------------------------------------------------------------- |
| Web apps      | HttpOnly, Secure, SameSite=Lax cookies. JavaScript cannot read the token.               |
| Desktop app   | Access + refresh tokens live in the OS keyring; metadata (user ID, workspace) in a config file |
| CLI           | `~/.config/bowrain/auth.json`, file-permission-scoped                                   |
| Server-to-server | `Authorization: Bearer <token>` header                                                |

HttpOnly cookies protect web app tokens from XSS. CLI and desktop clients use
the `Authorization: Bearer` header.

All CRUD endpoints require JWT authentication when the server is running with
a `JWTSecret` configured — there are no unauthenticated project routes.

### Web Login Flow

```
Browser                   bowrain-server              Keycloak
   │                            │                       │
   │  GET /login?return=/        │                       │
   ├───────────────────────────►│                       │
   │  302 /auth/oidc/login       │                       │
   │◄───────────────────────────┤                       │
   │  GET /auth/oidc/callback?code=...                   │
   ├────────────────────────────┼──────────────────────►│
   │                            │  token exchange       │
   │                            ├──────────────────────►│
   │                            │◄──────────────────────┤
   │  Set-Cookie: bowrain-session=... (HttpOnly)        │
   │◄───────────────────────────┤                       │
```

The server stores the PKCE verifier and OIDC nonce in the
`SessionStateStore` (see below) keyed by a short-lived state parameter.

### Desktop PKCE Flow

The Bowrain desktop app uses OAuth 2.0 Authorization Code with PKCE (RFC
7636):

1. App generates a PKCE code verifier and challenge.
2. App opens the system browser to
   `{server}/api/v1/auth/desktop/login?code_challenge=...`.
3. User authenticates via Keycloak in the browser.
4. Server redirects to `bowrain://auth/callback?code=...&state=...`.
5. The OS routes the custom URL scheme to the desktop app's handler.
6. App exchanges the code at `/api/v1/auth/desktop/token` with the verifier.
7. Tokens are split: secrets (access + refresh) go into the OS keyring
   (macOS Keychain, Windows Credential Manager, Linux Secret Service);
   metadata goes into `<UserConfigDir>/bowrain-desktop/auth.json`.

On subsequent launches the app silently refreshes the access token before
connecting to the server.

### CLI Device Flow

The `bowrain` CLI uses OAuth 2.0 Device Authorization Grant (RFC 8628), which
is terminal-friendly and works over SSH:

1. `bowrain auth login --server <url>` calls
   `POST /api/v1/auth/device/code`.
2. Server returns `{ device_code, user_code, verification_url }`.
3. The CLI prints the user code and verification URL, offers to open the
   browser.
4. The user authenticates via Keycloak and enters the user code.
5. The CLI polls `POST /api/v1/auth/device/token` until the user authorizes.
6. Tokens are stored at `~/.config/bowrain/auth.json`.

The device authorization claim page is hosted in the web app with the same
theme as the rest of Bowrain.

### Server Modes

| Feature    | `bowrain-server`              | `bowrain serve` (standalone)       |
| ---------- | ----------------------------- | ---------------------------------- |
| Binding    | `0.0.0.0` (configurable)      | `127.0.0.1`                        |
| Auth       | OIDC + JWT                    | None                               |
| Workspaces | Multi-workspace, multi-user   | Single implicit workspace          |
| gRPC       | Yes                           | No                                 |
| Use case   | Production / team deployment  | Local single-user editing          |

The selector is the `Config.JWTSecret` field: when set, OIDC routes are
registered and all CRUD endpoints require authentication; when empty, routes
are registered without auth middleware.

The server reports its mode via `GET /api/v1/config` so the web UI, desktop
app, and CLI can adapt their behavior.

### SessionStateStore

During OIDC handshakes, device flows, and desktop PKCE, the server holds
short-lived state: device codes awaiting authorization, PKCE verifiers, OIDC
nonces. These entries are write-once/read-once with a 10-minute TTL.

A `SessionStateStore` interface abstracts the backend:

| Backend   | When used                              | Configuration           |
| --------- | -------------------------------------- | ----------------------- |
| In-memory | Default; single-instance deployments   | No config needed        |
| Redis     | Multi-instance / horizontal scaling    | `BOWRAIN_REDIS_URL`     |

The Redis backend uses `SETEX` for automatic expiry. The in-memory backend
runs a lazy expiry plus periodic background cleanup. `BOWRAIN_REDIS_PASSWORD`
is honored separately from the URL for environments where the password comes
from a secret store.

The same interface also holds session grants for @bravo and MCP sessions
(see [AD-003: Permissions and Access Control](003-permissions.md)).

Durable auth data — refresh token hashes, workspace memberships, API tokens
— lives in PostgreSQL, never Redis.

### Database Schema

Auth tables share the same PostgreSQL database as the content store
([AD-004: Content Store and Versioning](004-content-store.md)), selected by
`DATABASE_URL`:

```sql
CREATE TABLE users (
    id         TEXT PRIMARY KEY,
    email      TEXT UNIQUE NOT NULL,
    name       TEXT NOT NULL,
    avatar_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE workspaces (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    logo_url    TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE workspace_members (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         TEXT NOT NULL DEFAULT 'member',
    joined_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (workspace_id, user_id)
);
```

The `projects` table carries a `workspace_id` column linking every project to
its owning workspace.

### Package Layout

| Package                       | Role                                                          |
| ----------------------------- | ------------------------------------------------------------- |
| `bowrain/core/auth/`          | Auth types (User, Workspace, Token), JWT, PKCE, device flow client |
| `bowrain/auth/`               | `AuthStore` interface + SQLite / PostgreSQL implementations, OIDC helpers |
| `bowrain/server/`             | REST/gRPC server, auth middleware chain, workspace handlers, gRPC auth interceptors |
| `bowrain/service/auth.go`     | `AuthService` business logic                                  |
| `bowrain/cli/cmd/bowrain/auth.go` | `bowrain auth login | logout | status`                    |

## Consequences

- Every project, block, connector, and API token is workspace-scoped. There
  is no global namespace a user can escape into.
- OIDC through Keycloak gives enterprise customers a single federation
  surface for GitHub, Google, LDAP, SAML, and Azure AD without Bowrain
  re-implementing each.
- HttpOnly cookies protect web app tokens from JavaScript access;
  Bearer tokens flow through headers for non-browser clients.
- Single-use refresh token rotation with server-side hashing makes stolen
  tokens detectable and short-lived.
- CLI device flow works over SSH and in CI; desktop PKCE gives a seamless
  browser redirect with OS keyring storage.
- The admin realm is fully separated from the customer realm, so admin
  access cannot leak into customer identity and vice versa.
- An OIDC provider is a deployment dependency for multi-user mode but not
  for `bowrain serve` standalone local use.

## Related

- [AD-001: Bowrain Vision and Module Architecture](001-vision-and-modules.md)
- [AD-003: Permissions and Access Control](003-permissions.md)
- [AD-004: Content Store and Versioning](004-content-store.md)
