---
title: Self-Hosting
sidebar_position: 12
---

# Self-Hosting

Run your own Bowrain instance with Docker. A deployment is a few cooperating
services:

- **bowrain-server** — the REST + gRPC API.
- **bowrain-worker** — the async worker; runs the auto-translate-on-push
  automation and upstream machine translation.
- **PostgreSQL** — the authoritative store (projects, blocks, workspaces, users,
  jobs). The server requires PostgreSQL; there is no SQLite/file backend.
- **NATS** — the job queue and event bus shared by the server and worker. Push
  processing is asynchronous ([AD-009](../architecture-decisions/009-sync-protocol.md)):
  the server enqueues, the worker ingests and translates.
- **bowrain-web** — the static web UI, served as its own container.
- An **OIDC identity provider** (e.g. Keycloak) and an **SMTP** sender.

A reverse proxy routes `/api` + gRPC to the server and everything else to the web UI.

## Quick Start

The repository ships a complete reference stack at
[`bowrain/deploy/docker/compose.yaml`](https://github.com/neokapi/neokapi/blob/main/bowrain/deploy/docker/compose.yaml)
— Traefik, PostgreSQL, NATS, the server, the worker, and the web UI, all from
published images. Copy it together with its sibling `traefik.yml`, set the
required values, then start it:

```bash
docker compose -f deploy/docker/compose.yaml up -d
```

The stack expects an external OIDC provider (see
[OIDC Provider Setup](#oidc-provider-setup)). At minimum, provide:

```bash
POSTGRES_PASSWORD=...                          # database password
BOWRAIN_JWT_SECRET=$(openssl rand -base64 32)  # JWT signing secret
BOWRAIN_OIDC_ISSUER_URL=...                    # your realm's issuer URL
BOWRAIN_OIDC_CLIENT_SECRET=...                 # the bowrain client's secret
# Machine translation runs in the worker — configure an upstream provider:
BOWRAIN_PLATFORM_PROVIDER=gemini               # or openai / anthropic / ollama
BOWRAIN_PLATFORM_API_KEY=...                   # provider API key
```

Once up, the web UI is served through the proxy on port 80; new users self-register
through your OIDC provider.

:::tip
For a one-command local stack that also bundles Keycloak and Mailpit — with no
OIDC setup and an offline translation provider by default — see
[Local Development](../developer/local-development.md).
:::

## Environment Variables

### Server (`bowrain-server`)

| Variable                     | Default   | Description                                                      |
| ---------------------------- | --------- | ---------------------------------------------------------------- |
| `BOWRAIN_DATABASE_URL`       |           | PostgreSQL connection string (`postgres://…`) — **required**     |
| `BOWRAIN_JWT_SECRET`         |           | JWT signing secret — required for auth                           |
| `BOWRAIN_OIDC_ISSUER_URL`    |           | OIDC issuer URL (internal, reachable from the server)            |
| `BOWRAIN_OIDC_PUBLIC_URL`    |           | OIDC public URL (browser-facing; defaults to the issuer URL)     |
| `BOWRAIN_OIDC_CLIENT_ID`     | `bowrain` | OIDC client ID                                                   |
| `BOWRAIN_OIDC_CLIENT_SECRET` |           | OIDC client secret                                               |
| `BOWRAIN_NATS_URL`           |           | NATS URL for the job queue + event bus (e.g. `nats://nats:4222`) |
| `BLOB_STORAGE_LOCAL_DIR`     |           | Directory for sync push payloads (shared with the worker)        |
| `BOWRAIN_SMTP_HOST`          |           | SMTP server `host:port` for transactional emails                 |
| `BOWRAIN_SMTP_FROM`          |           | Sender email address for transactional emails                    |
| `BOWRAIN_PORT`               | `8080`    | HTTP port to listen on                                           |
| `BOWRAIN_HOST`               | `0.0.0.0` | Address to bind to                                              |

### Worker (`bowrain-worker`)

| Variable                    | Description                                                                                          |
| --------------------------- | ---------------------------------------------------------------------------------------------------- |
| `BOWRAIN_DATABASE_URL`      | Same PostgreSQL connection string as the server                                                      |
| `BOWRAIN_NATS_URL`          | Same NATS URL as the server                                                                          |
| `LOCAL_BLOB_DIR`            | Sync push payload dir — must point at the same shared volume as the server's `BLOB_STORAGE_LOCAL_DIR` |
| `BOWRAIN_PLATFORM_PROVIDER` | Translation provider: `gemini`, `openai`, `anthropic`, `ollama` (or `demo` for offline output)       |
| `BOWRAIN_PLATFORM_API_KEY`  | Provider API key (or a provider-specific variable such as `GEMINI_API_KEY`)                          |
| `BOWRAIN_PLATFORM_MODEL`    | Default model for the provider                                                                       |

## OIDC Provider Setup

Any OIDC-compliant identity provider works with bowrain-server (Keycloak, Auth0,
Okta, Google, Azure AD, Dex, etc.). The server uses standard OIDC discovery to
resolve authorization and token endpoints automatically.

### Keycloak (Recommended)

Keycloak provides self-registration, email verification, social login federation,
and fine-grained access control out of the box.

Key configuration for the Keycloak client:

- **Client ID**: `bowrain`
- **Client protocol**: `openid-connect`
- **Access Type**: `confidential` (with client secret)
- **Standard Flow Enabled**: `true` (authorization code flow)
- **Valid Redirect URIs**: `https://your-domain.com/api/v1/auth/callback`
- **Web Origins**: `https://your-domain.com`

Enable self-registration in the realm settings to allow new users to create
accounts. Configure SMTP in the realm for email verification.

### Other Providers

For other OIDC providers, create an OAuth2/OIDC application with:

- **Redirect URI**: `https://your-domain.com/api/v1/auth/callback`
- **Scopes**: `openid profile email`
- **Grant type**: Authorization code flow

Set `BOWRAIN_OIDC_ISSUER_URL` to the provider's issuer URL (found in the
`.well-known/openid-configuration` endpoint).

## Production Tips

### JWT Secret

Generate a strong random secret for `BOWRAIN_JWT_SECRET`:

```bash
openssl rand -base64 32
```

Never use the default development secret in production.

### Persistent Storage

Two volumes hold durable state — back both with named volumes or bind mounts so
they survive container restarts:

- **PostgreSQL** — the authoritative store, in `postgres-data`
  (`/var/lib/postgresql/data`).
- **Blob storage** — sync push payloads shared by the server and worker, in
  `blob-data` (`/data`).

```yaml
volumes:
  - /opt/bowrain/postgres:/var/lib/postgresql/data # bind mount
  # or
  - postgres-data:/var/lib/postgresql/data # named volume
```

### Reverse Proxy

For production, put the stack behind a reverse proxy (Nginx, Caddy, Traefik) to
handle TLS termination. The reference compose uses Traefik; with another proxy,
route `/api` and gRPC (`/neokapi.*`) to bowrain-server and everything else to
bowrain-web. A minimal Nginx server block fronting Traefik (or the services):

```nginx
server {
    listen 443 ssl;
    server_name bowrain.example.com;

    ssl_certificate /etc/ssl/certs/bowrain.crt;
    ssl_certificate_key /etc/ssl/private/bowrain.key;

    location / {
        proxy_pass http://localhost:80;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

When using a reverse proxy, set `BOWRAIN_OIDC_PUBLIC_URL` and the OIDC client's
redirect URI to the public HTTPS URL.

## Docker Image Tags

A deployment uses three images — `bowrain-server`, `bowrain-worker`, and
`bowrain-web` — published under `ghcr.io/neokapi/`:

| Tag      | Description                      |
| -------- | -------------------------------- |
| `latest` | Most recent release              |
| `X.Y.Z`  | Specific version (e.g., `0.5.0`) |

Pull a specific version (keep server, worker, and web on the same tag):

```bash
docker pull ghcr.io/neokapi/bowrain-server:0.5.0
docker pull ghcr.io/neokapi/bowrain-worker:0.5.0
docker pull ghcr.io/neokapi/bowrain-web:0.5.0
```

## Backup & Restore

All authoritative data lives in PostgreSQL — back it up with `pg_dump`. (The blob
volume holds only in-flight push payloads; committed content is in PostgreSQL, so
the blob volume does not need backing up.)

### Backup

```bash
docker compose -f deploy/docker/compose.yaml exec -T postgres \
  pg_dump -U bowrain -Fc bowrain > bowrain-$(date +%Y%m%d).dump
```

### Restore

```bash
docker compose -f deploy/docker/compose.yaml exec -T postgres \
  pg_restore -U bowrain -d bowrain --clean --if-exists < bowrain-YYYYMMDD.dump
```

### Scheduled Backups

Add a cron job for regular backups:

```bash
# Daily backup at 2 AM
0 2 * * * docker compose -f /opt/bowrain/deploy/docker/compose.yaml exec -T postgres pg_dump -U bowrain -Fc bowrain > /opt/backups/bowrain-$(date +\%Y\%m\%d).dump
```

## CLI Connection

Connect the Bowrain CLI to your self-hosted server:

```bash
kapi auth login --server https://bowrain.example.com
```

This starts a device authorization flow. Open the URL shown in your terminal,
authenticate with your identity provider, and the CLI receives a token
automatically.
