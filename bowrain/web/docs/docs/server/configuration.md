---
sidebar_position: 3
title: Configuration
---

# Server configuration

This page is the complete reference for configuring **bowrain-server** and
**bowrain-worker**. For the overall service topology (PostgreSQL + NATS + worker
+ blob storage) and a production walkthrough, see [Self-Hosting](/server/self-hosting).

## Precedence

The server reads command-line flags first, then applies environment-variable
overrides on top — so an environment variable wins over the same value passed as
a flag. All knobs have an environment variable; only a subset are also exposed as
flags. The worker is configured **only** through environment variables.

## Storage

Bowrain Server requires **PostgreSQL**. There is no SQLite or file backend. The
connection string must use the `postgres://` or `postgresql://` scheme — the
server refuses to start otherwise. The schema is created automatically on first
start, and migrations run on startup.

```bash
BOWRAIN_DATABASE_URL=postgres://bowrain:password@localhost/bowrain
```

The same `BOWRAIN_DATABASE_URL` must be given to the worker.

## Server environment variables

All Bowrain variables use the `BOWRAIN_` prefix; a few external integrations
(Azure, Stripe, PostHog) use their vendor's conventional names.

### Core

| Variable | Default | Description |
| --- | --- | --- |
| `BOWRAIN_DATABASE_URL` | _(empty)_ | PostgreSQL connection string (`postgres://…`) — **required** |
| `BOWRAIN_DATABASE_AUTH` | _(empty)_ | `azure` to use Entra ID managed-identity tokens; otherwise password auth from the URL |
| `BOWRAIN_PORT` | `8080` | HTTP port to listen on (gRPC is multiplexed onto the same port) |
| `BOWRAIN_HOST` | `0.0.0.0` | Address to bind to |
| `BOWRAIN_DATA_DIR` | _(empty)_ | Directory for temporary files during processing |
| `BOWRAIN_NATS_URL` | _(empty)_ | NATS URL for the job queue + event bus (e.g. `nats://nats:4222`) |
| `BOWRAIN_REDIS_URL` | _(empty)_ | Redis URL for caching and session state |
| `BOWRAIN_REDIS_PASSWORD` | _(empty)_ | Redis password (overrides any password in `BOWRAIN_REDIS_URL`) |
| `BOWRAIN_MAX_PUSH_BYTES` | `256MB` | Max total upload size per push |
| `BOWRAIN_WEB_UI_DIR` | _(empty)_ | Path to built web UI static files (dev only; production serves the UI from a separate container) |
| `BOWRAIN_LOG_FORMAT` | _(empty)_ | `text` or `json` |
| `BOWRAIN_LOG_LEVEL` | _(empty)_ | `debug`, `info`, `warn`, `error` |

### Authentication

| Variable | Default | Description |
| --- | --- | --- |
| `BOWRAIN_JWT_SECRET` | _(empty)_ | JWT signing secret. When set, auth, OIDC login, and workspace management are enabled |
| `BOWRAIN_OIDC_ISSUER_URL` | _(empty)_ | OIDC issuer URL (internal, used for token validation) |
| `BOWRAIN_OIDC_PUBLIC_URL` | _(falls back to issuer URL)_ | Browser-facing OIDC URL (for redirects) |
| `BOWRAIN_OIDC_CLIENT_ID` | _(empty)_ | OIDC OAuth client ID |
| `BOWRAIN_OIDC_CLIENT_SECRET` | _(empty)_ | OIDC OAuth client secret |

The Keycloak admin API (used to write back email changes initiated from the UI)
and the admin control plane are optional:

| Variable | Description |
| --- | --- |
| `BOWRAIN_KEYCLOAK_ADMIN_URL` | In-cluster Keycloak admin URL (enables Bowrain-managed email change) |
| `BOWRAIN_KEYCLOAK_REALM` | Realm name (default `bowrain`) |
| `BOWRAIN_KEYCLOAK_ADMIN_CLIENT_ID` | Service-account client with `realm-management:manage-users` |
| `BOWRAIN_KEYCLOAK_ADMIN_CLIENT_SECRET` | Service-account client secret |
| `BOWRAIN_ADMIN_OIDC_ISSUER_URL` | Issuer URL for the `/api/admin/*` control plane |
| `BOWRAIN_ADMIN_OIDC_CLIENT_ID` | Admin control-plane client ID |
| `BOWRAIN_ADMIN_OIDC_CLIENT_SECRET` | Admin control-plane client secret |

:::tip OIDC public URL
When your OIDC provider has a different internal hostname than the browser-facing
URL (common in Docker), set `BOWRAIN_OIDC_ISSUER_URL` to the internal URL (e.g.
`http://keycloak:8080/realms/bowrain`) and `BOWRAIN_OIDC_PUBLIC_URL` to the
browser-facing URL (e.g. `http://localhost:8180/realms/bowrain`). If unset,
`BOWRAIN_OIDC_PUBLIC_URL` defaults to `BOWRAIN_OIDC_ISSUER_URL`.
:::

### Blob storage

Bowrain stores in-flight sync push payloads in a blob store, shared with the
worker. The backend defaults to `local`.

| Variable | Default | Description |
| --- | --- | --- |
| `BLOB_STORAGE_BACKEND` | `local` | `local` or `azure` |
| `BLOB_STORAGE_LOCAL_DIR` | `$BOWRAIN_DATA_DIR/blobs` (or a temp dir) | Local blob storage directory (server) |
| `AZURE_STORAGE_ACCOUNT_URL` | _(empty)_ | Azure Blob Storage account URL |
| `AZURE_STORAGE_CONTAINER` | `bowrain-assets` | Azure Blob Storage container |
| `AZURE_STORAGE_CONNECTION_STRING` | _(empty)_ | Azure connection string (dev / Azurite) |

### Email

Set `BOWRAIN_SMTP_HOST` + `BOWRAIN_SMTP_FROM` for an unauthenticated relay
(local dev / Mailpit), add username/password for authenticated SMTP, or set
`BOWRAIN_RESEND_API_KEY` to send via Resend instead.

| Variable | Description |
| --- | --- |
| `BOWRAIN_SMTP_HOST` | SMTP server in `host:port` format (empty = email disabled) |
| `BOWRAIN_SMTP_FROM` | Sender email address |
| `BOWRAIN_SMTP_USERNAME` | SMTP auth username (empty = no auth) |
| `BOWRAIN_SMTP_PASSWORD` | SMTP auth password |
| `BOWRAIN_SMTP_USE_TLS` | `true`/`1` for implicit TLS (SMTPS); otherwise STARTTLS |
| `BOWRAIN_RESEND_API_KEY` | Resend API key (used instead of SMTP when set; reuses `BOWRAIN_SMTP_FROM`) |

### Agent (@bravo)

The in-product agent runs in containers. When `BOWRAIN_AGENT_RUNTIME` is unset it
falls back to local mock responses.

| Variable | Description |
| --- | --- |
| `BOWRAIN_AGENT_RUNTIME` | `docker` or `aca` (Azure Container Apps) |
| `BOWRAIN_AGENT_IMAGE` | Agent container image |
| `BOWRAIN_AGENT_MAX_CONCURRENT` | Max concurrent agent containers per workspace |
| `BOWRAIN_AGENT_DOCKER_HOST` / `BOWRAIN_AGENT_DOCKER_NETWORK` | Docker runtime settings |
| `BOWRAIN_AGENT_ACA_SUBSCRIPTION` / `_RESOURCE_GROUP` / `_ENVIRONMENT_ID` / `_LOCATION` | Azure Container Apps settings |
| `BOWRAIN_AGENT_MODEL_PROVIDER` / `_MODEL_NAME` / `_MODEL_API_BASE` / `_MODEL_API_KEY` | Agent model configuration |

### Billing, analytics, audit

| Variable | Description |
| --- | --- |
| `STRIPE_SECRET_KEY` / `STRIPE_WEBHOOK_SECRET` | Stripe API + webhook secrets |
| `STRIPE_PRO_PRICE_ID` / `STRIPE_TEAM_PRICE_ID` / `STRIPE_CREDIT_PRICE_ID` | Stripe price IDs |
| `POSTHOG_API_KEY` / `POSTHOG_HOST` | PostHog analytics |
| `BOWRAIN_AUDIT_RETENTION_DAYS` | Prune audit-log rows older than N days (0 = keep forever) |
| `BOWRAIN_AUDIT_SIEM_WEBHOOK_URL` | Forward every audit event as NDJSON to an external SIEM |

### Azure integration

| Variable | Description |
| --- | --- |
| `AZURE_CLIENT_ID` | Managed-identity client ID (used when `BOWRAIN_DATABASE_AUTH=azure`) |
| `BOWRAIN_SERVICE_BUS_CONNECTION` | Azure Service Bus connection string (alternative job queue to NATS) |

## Worker environment variables

The worker (`bowrain-worker`) shares the database and job queue with the server
and runs the auto-translate-on-push automation.

| Variable | Description |
| --- | --- |
| `BOWRAIN_DATABASE_URL` | Same PostgreSQL connection string as the server |
| `BOWRAIN_NATS_URL` | Same NATS URL as the server |
| `LOCAL_BLOB_DIR` | Sync push payload dir — must point at the same shared volume as the server's `BLOB_STORAGE_LOCAL_DIR` |
| `BOWRAIN_PLATFORM_PROVIDER` | Translation provider: `gemini`, `openai`, `anthropic`, `ollama`, or `demo` (offline) |
| `BOWRAIN_PLATFORM_API_KEY` | Provider API key (or a provider-specific variable such as `GEMINI_API_KEY`) |
| `BOWRAIN_PLATFORM_MODEL` | Default model for the provider |
| `BOWRAIN_PLATFORM_BASE_URL` | Provider API base URL (e.g. self-hosted Ollama) |
| `BOWRAIN_OPENAI_ENDPOINT` | Azure OpenAI endpoint (hosted-cloud path; uses managed identity) |

:::note Blob directory env var differs by service
The server reads `BLOB_STORAGE_LOCAL_DIR`; the worker reads `LOCAL_BLOB_DIR`.
Point both at the same shared volume.
:::

## Command-line flags

These flags are accepted by **bowrain-server**. Each maps to the corresponding
`BOWRAIN_` environment variable, which takes precedence.

```bash
bowrain-server \
  --port 8080 \
  --host 0.0.0.0 \
  --database-url postgres://bowrain:password@localhost/bowrain \
  --data-dir /tmp/bowrain \
  --jwt-secret your-secret \
  --oidc-issuer-url https://keycloak.example.com/realms/bowrain \
  --oidc-client-id bowrain \
  --oidc-client-secret your-client-secret \
  --web-ui-dir /path/to/web/dist
```

| Flag | Default | Description |
| --- | --- | --- |
| `--port` | `8080` | HTTP port to listen on |
| `--host` | `0.0.0.0` | Address to bind to |
| `--data-dir` | _(empty)_ | Directory for temporary files |
| `--database-url` | _(empty)_ | PostgreSQL connection string (`postgres://…`) |
| `--jwt-secret` | _(empty)_ | JWT signing secret |
| `--oidc-issuer-url` | _(empty)_ | OIDC issuer URL |
| `--oidc-client-id` | _(empty)_ | OIDC OAuth client ID |
| `--oidc-client-secret` | _(empty)_ | OIDC OAuth client secret |
| `--web-ui-dir` | _(empty)_ | Path to built web UI static files |

## Next steps

- [Installation](/server/installation) — quick start and native binaries.
- [Self-Hosting](/server/self-hosting) — production deployment, OIDC, backups.
- [Getting Started](/server/getting-started) — first login, workspaces, invitations.
