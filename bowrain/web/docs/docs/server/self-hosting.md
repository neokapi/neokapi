---
title: Self-Hosting
sidebar_position: 12
description: Run your own Bowrain instance with Docker, the cooperating services a self-hosted deployment needs and how they fit together.
---

# Self-Hosting

Run your own Bowrain instance with Docker. A deployment is a few cooperating
services:

- **bowrain-server**: the REST + gRPC API.
- **bowrain-worker**: the async worker; ingests pushes and runs the drafting
  jobs a run enqueues against an upstream AI provider.
- **PostgreSQL**: the authoritative store (projects, blocks, workspaces, users,
  jobs). The server requires PostgreSQL; there is no SQLite or file backend.
- A **job queue** (Amazon SQS, or an SQS-compatible broker such as ElasticMQ)
  and a **Redis** instance for the event bus (Redis Streams), shared by the
  server and worker. Push processing is asynchronous: the server enqueues, the
  worker ingests and drafts.
- **Blob storage** for in-flight push payloads: a local directory shared by
  the server and worker, or an S3 bucket (MinIO for a local stack).
- **bowrain-web**: the static web UI, served as its own container.
- An **OIDC identity provider** (for example Keycloak) and an **SMTP** sender.

A reverse proxy routes `/api` + gRPC to the server and everything else to the web UI.

## Quick Start

The repository ships a complete reference stack at
[`bowrain/deploy/docker/compose.yaml`](https://github.com/neokapi/neokapi/blob/main/bowrain/deploy/docker/compose.yaml):
Traefik, PostgreSQL, ElasticMQ (SQS-compatible job queue), Redis, the server,
the worker, and the web UI, all from published images. Copy it together with
its sibling `traefik.yml`, set the required values, then start it:

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
BOWRAIN_APP_PUBLIC_URL=https://bowrain.example # the URL browsers use to reach Bowrain
# Drafting runs in the worker; configure an upstream AI provider:
BOWRAIN_PLATFORM_PROVIDER=gemini               # or bedrock / openai / anthropic / ollama
BOWRAIN_PLATFORM_API_KEY=...                   # provider API key
```

Once up, the web UI is served through the proxy on port 80; new users self-register
through your OIDC provider.

:::tip
For a one-command local stack that also bundles Keycloak, MinIO and Mailpit,
with no OIDC setup and an offline AI provider by default, see the
[Installation guide](/server/installation).
:::

## Environment Variables

### Server (`bowrain-server`)

| Variable                     | Default   | Description                                                      |
| ---------------------------- | --------- | ---------------------------------------------------------------- |
| `BOWRAIN_DATABASE_URL`       |           | PostgreSQL connection string (`postgres://…`), **required**      |
| `BOWRAIN_JWT_SECRET`         |           | JWT signing secret, required for auth                            |
| `BOWRAIN_OIDC_ISSUER_URL`    |           | OIDC issuer URL (internal, reachable from the server)            |
| `BOWRAIN_OIDC_PUBLIC_URL`    |           | Browser-facing URL of the identity provider, if it differs from the issuer URL |
| `BOWRAIN_APP_PUBLIC_URL`     |           | The URL browsers use to reach Bowrain; sets the CORS and WebSocket origin allowlists |
| `BOWRAIN_FORCE_SECURE_COOKIES` | `true`  | Marks session cookies `Secure` regardless of the request scheme  |
| `BOWRAIN_OIDC_CLIENT_ID`     | `bowrain` | OIDC client ID                                                   |
| `BOWRAIN_OIDC_CLIENT_SECRET` |           | OIDC client secret                                               |
| `BOWRAIN_QUEUE_BACKEND`      |           | `sqs` selects the SQS job-queue backend                          |
| `SQS_ENDPOINT`               |           | SQS endpoint override for ElasticMQ/LocalStack; empty on AWS     |
| `BOWRAIN_EVENT_BACKEND`      |           | `redis` runs the event bus on Redis Streams                      |
| `BOWRAIN_REDIS_URL`          |           | Redis URL (event bus, caching, session state)                    |
| `BLOB_STORAGE_BACKEND`       | `local`   | `local` or `s3` for in-flight push payloads                      |
| `BLOB_STORAGE_LOCAL_DIR`     |           | Local blob directory (shared with the worker)                    |
| `S3_BLOB_BUCKET`             |           | Bucket for the `s3` backend (setting it selects `s3`)            |
| `BOWRAIN_SMTP_HOST`          |           | SMTP server `host:port` for transactional emails                 |
| `BOWRAIN_SMTP_FROM`          |           | Sender email address for transactional emails                    |
| `BOWRAIN_PORT`               | `8080`    | HTTP port to listen on                                           |
| `BOWRAIN_HOST`               | `0.0.0.0` | Address to bind to                                              |

### Worker (`bowrain-worker`)

| Variable                    | Description                                                                                          |
| --------------------------- | ---------------------------------------------------------------------------------------------------- |
| `BOWRAIN_DATABASE_URL`      | Same PostgreSQL connection string as the server                                                      |
| `BOWRAIN_QUEUE_BACKEND` / `SQS_ENDPOINT` | Same job-queue selection as the server (the two must agree on the broker)               |
| `BOWRAIN_EVENT_BACKEND` / `BOWRAIN_REDIS_URL` | Same event-bus selection as the server                                             |
| `LOCAL_BLOB_DIR`            | Local blob directory; must point at the same shared volume as the server's `BLOB_STORAGE_LOCAL_DIR` |
| `BOWRAIN_PLATFORM_PROVIDER` | AI provider for platform jobs: `bedrock`, `gemini`, `openai`, `anthropic`, `ollama`, or `demo` (offline) |
| `BOWRAIN_PLATFORM_API_KEY`  | Provider API key (or a provider-specific variable such as `GEMINI_API_KEY`)                          |
| `BOWRAIN_PLATFORM_MODEL`    | Default model for the provider                                                                       |

The complete reference, including the S3 endpoint variables for MinIO, is on
the [Configuration](/server/configuration) page.

## OIDC Provider Setup

Any OIDC-compliant identity provider works with bowrain-server (Keycloak, Auth0,
Okta, Google, Azure AD, Dex, and others). The server uses standard OIDC
discovery to resolve authorization and token endpoints automatically.

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

Two volumes hold durable state. Back both with named volumes or bind mounts so
they survive container restarts:

- **PostgreSQL**: the authoritative store, in `postgres-data`
  (`/var/lib/postgresql/data`).
- **Blob storage**: in-flight push payloads shared by the server and worker,
  in `blob-data` (`/data`) for the `local` backend, or an S3 bucket.

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

When using a reverse proxy, set `BOWRAIN_APP_PUBLIC_URL` and the OIDC client's
redirect URI to the public HTTPS URL. Set `BOWRAIN_OIDC_PUBLIC_URL` too if the
browser reaches your identity provider under a different name than the server
does.

`BOWRAIN_FORCE_SECURE_COOKIES` is already `true` by default, which is what you
want here: TLS terminates at the proxy, so without it the `Secure` flag would
depend on the `X-Forwarded-Proto` header above continuing to arrive.

A server is production unless `BOWRAIN_ALLOW_INSECURE_DEV=1` says otherwise, so
a self-hosted deployment gets the strict origin policy by default. That flag is
for a local development box and should never be set on anything reachable from
the internet.

## Docker Image Tags

A deployment uses three images, `bowrain-server`, `bowrain-worker`, and
`bowrain-web`, published under `ghcr.io/neokapi/`:

| Tag      | Description                      |
| -------- | -------------------------------- |
| `latest` | Most recent release              |
| `X.Y.Z`  | Specific version (for example `0.5.0`) |

Pull a specific version (keep server, worker, and web on the same tag):

```bash
docker pull ghcr.io/neokapi/bowrain-server:0.5.0
docker pull ghcr.io/neokapi/bowrain-worker:0.5.0
docker pull ghcr.io/neokapi/bowrain-web:0.5.0
```

## Backup & Restore

All authoritative data lives in PostgreSQL; back it up with `pg_dump`. (The blob
store holds only in-flight push payloads; committed content is in PostgreSQL, so
the blob store does not need backing up.)

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

Connect kapi to your self-hosted server:

```bash
kapi auth login --server https://bowrain.example.com
```

This starts a device authorization flow. Open the URL shown in your terminal,
authenticate with your identity provider, and the CLI receives a token
automatically.
