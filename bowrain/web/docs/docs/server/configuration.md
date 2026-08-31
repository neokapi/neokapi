---
sidebar_position: 3
title: Configuration
description: The complete reference for configuring bowrain-server and bowrain-worker, environment variables, service topology, and runtime settings.
---

# Server configuration

This page is the complete reference for configuring **bowrain-server** and
**bowrain-worker**. For the overall service topology (PostgreSQL + job queue +
worker + blob storage) and a production walkthrough, see
[Self-Hosting](/server/self-hosting).

## Precedence

The server reads command-line flags first, then applies environment-variable
overrides on top, so an environment variable wins over the same value passed as
a flag. All knobs have an environment variable; only a subset are also exposed as
flags. The worker is configured **only** through environment variables.

## Storage

Bowrain Server requires **PostgreSQL**. There is no SQLite or file backend. The
connection string must use the `postgres://` or `postgresql://` scheme; the
server refuses to start otherwise. The schema is created automatically on first
start, and migrations run on startup. The context graph runs on the same stock
PostgreSQL (no extension required).

```bash
BOWRAIN_DATABASE_URL=postgres://bowrain:password@localhost/bowrain
```

The same `BOWRAIN_DATABASE_URL` must be given to the worker.

## Server environment variables

All Bowrain variables use the `BOWRAIN_` prefix; a few external integrations
(AWS, Stripe, PostHog) use their vendor's conventional names.

### Core

| Variable | Default | Description |
| --- | --- | --- |
| `BOWRAIN_DATABASE_URL` | _(empty)_ | PostgreSQL connection string (`postgres://…`), **required** |
| `BOWRAIN_PORT` | `8080` | HTTP port to listen on (gRPC is multiplexed onto the same port) |
| `BOWRAIN_HOST` | `0.0.0.0` | Address to bind to |
| `BOWRAIN_DATA_DIR` | _(empty)_ | Directory for temporary files during processing |
| `BOWRAIN_QUEUE_BACKEND` | _(empty)_ | `sqs` selects the SQS job-queue backend; unset uses an in-process queue (single-instance development only) |
| `SQS_ENDPOINT` | _(empty)_ | SQS endpoint override for SQS-compatible emulators (ElasticMQ, LocalStack); empty on AWS |
| `BOWRAIN_SQS_QUEUE_PREFIX` | _(empty)_ | Optional name prefix applied to every job queue |
| `BOWRAIN_EVENT_BACKEND` | _(empty)_ | `redis` runs the event bus on Redis Streams (requires `BOWRAIN_REDIS_URL`); unset uses the in-memory bus |
| `BOWRAIN_REDIS_URL` | _(empty)_ | Redis URL for caching, session state, and the Redis Streams event bus |
| `BOWRAIN_REDIS_PASSWORD` | _(empty)_ | Redis password (overrides any password in `BOWRAIN_REDIS_URL`) |
| `BOWRAIN_MAX_PUSH_BYTES` | `256MB` | Max total upload size per push |
| `BOWRAIN_WEB_UI_DIR` | _(empty)_ | Path to built web UI static files (dev only; production serves the UI from a separate container) |
| `BOWRAIN_PULSE_ENABLED` | `false` | Mounts the public Pulse activity dashboard (`/api/v1/pulse` routes + the pulse subdomain SPA). Unmounted by default |
| `BOWRAIN_LOG_FORMAT` | _(empty)_ | `text` or `json` |
| `BOWRAIN_LOG_LEVEL` | _(empty)_ | `debug`, `info`, `warn`, `error` |

### Authentication

| Variable | Default | Description |
| --- | --- | --- |
| `BOWRAIN_JWT_SECRET` | _(empty)_ | JWT signing secret. When set, auth, OIDC login, and workspace management are enabled |
| `BOWRAIN_OIDC_ISSUER_URL` | _(empty)_ | OIDC issuer URL (internal, used for token validation) |
| `BOWRAIN_OIDC_PUBLIC_URL` | _(empty)_ | Browser-facing URL of the identity provider, when it differs from the issuer URL (for redirects) |
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
URL (common in Docker), set `BOWRAIN_OIDC_ISSUER_URL` to the internal URL (for
example `http://keycloak:8080/realms/bowrain`) and `BOWRAIN_OIDC_PUBLIC_URL` to
the browser-facing URL (for example `http://localhost:8180/realms/bowrain`).
Leave it unset when the two are the same: unset means "the browser reaches the
provider at the issuer URL", which is what each redirect already assumes. It
names the *identity provider's* address, not this application's; for the app's
own origin, see `BOWRAIN_APP_PUBLIC_URL` below.
:::

### Origins and cookies

These decide which other origins may talk to the API, and whether session
cookies are marked `Secure`.

| Variable | Default | Description |
| --- | --- | --- |
| `BOWRAIN_APP_PUBLIC_URL` | _(empty)_ | This application's browser-facing origin (for example `https://app.bowrain.cloud`). Used to build the CORS and WebSocket origin allowlists |
| `BOWRAIN_PUBLIC_SITE_URL` | _(empty)_ | Marketing landing origin, allowed one credentialed cross-origin read (`GET /api/v1/auth/whoami`) so the landing can render a signed-in link |
| `BOWRAIN_FORCE_SECURE_COOKIES` | `true`, or `false` in development | Marks session cookies `Secure` regardless of the request scheme |
| `BOWRAIN_ALLOW_INSECURE_DEV` | `false` | Marks the process a development instance. Also relaxes startup configuration checks and enables direct device approval |

:::warning Development mode is opted into, never inferred
A server is production unless `BOWRAIN_ALLOW_INSECURE_DEV` (or
`--allow-insecure-dev`) says otherwise. In production, only
`BOWRAIN_APP_PUBLIC_URL` and `BOWRAIN_PUBLIC_SITE_URL` may make credentialed
cross-origin requests, and WebSockets are accepted only from the app's own
origin. In development, any `localhost` origin is accepted as well.

Set `BOWRAIN_APP_PUBLIC_URL` on any deployment where a browser reaches the API
from a different origin than the one it was served from. If it is unset, no
cross-origin caller is allowed, which is the safe answer: same-origin requests
never consult CORS.

Do not use `BOWRAIN_ALLOW_INSECURE_DEV` on anything reachable from the
internet.
:::

:::tip Secure cookies behind a proxy
Bowrain infers the request scheme from `X-Forwarded-Proto`, so behind a
TLS-terminating proxy the `Secure` flag would otherwise depend on that header
arriving. It is forced on by default outside development for exactly that
reason. Development leaves it off, because browsers discard `Secure` cookies
sent over plain HTTP.
:::

### Blob storage

Bowrain stores in-flight sync push payloads in a blob store, shared with the
worker. The backend defaults to `local`; setting `S3_BLOB_BUCKET` selects `s3`.

| Variable | Default | Description |
| --- | --- | --- |
| `BLOB_STORAGE_BACKEND` | `local` | `local` or `s3` |
| `BLOB_STORAGE_LOCAL_DIR` | `$BOWRAIN_DATA_DIR/blobs` (or a temp dir) | Local blob storage directory (server) |
| `S3_BLOB_BUCKET` | _(empty)_ | Bucket for the `s3` backend; setting it selects `s3` |
| `S3_BLOB_PREFIX` | _(empty)_ | Key prefix inside the bucket |
| `AWS_REGION` | _(empty)_ | Region of the bucket (and of Bedrock, below) |
| `S3_ENDPOINT` | _(empty)_ | Endpoint override for an S3-compatible store such as MinIO; empty on AWS |
| `S3_PUBLIC_ENDPOINT` | _(empty)_ | Endpoint presigned upload URLs are issued against, when clients reach the store under a different name than the server does |
| `S3_FORCE_PATH_STYLE` | _(empty)_ | `true` for path-style addressing (MinIO) |

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

The in-product agent is dark by default on every plan. When enabled for a
workspace, it runs in containers; when `BOWRAIN_AGENT_RUNTIME` is unset it
falls back to local mock responses.

| Variable | Description |
| --- | --- |
| `BOWRAIN_AGENT_RUNTIME` | `docker` |
| `BOWRAIN_AGENT_IMAGE` | Agent container image |
| `BOWRAIN_AGENT_MAX_CONCURRENT` | Max concurrent agent containers per workspace |
| `BOWRAIN_AGENT_DOCKER_HOST` / `BOWRAIN_AGENT_DOCKER_NETWORK` | Docker runtime settings |
| `BOWRAIN_AGENT_MODEL_PROVIDER` / `_MODEL_NAME` / `_MODEL_API_BASE` / `_MODEL_API_KEY` | Agent model configuration |

### Billing, analytics, audit

| Variable | Description |
| --- | --- |
| `STRIPE_SECRET_KEY` / `STRIPE_WEBHOOK_SECRET` | Stripe API + webhook secrets |
| `STRIPE_PRO_PRICE_ID` / `STRIPE_TEAM_PRICE_ID` / `STRIPE_CREDIT_PRICE_ID` | Stripe price IDs |
| `POSTHOG_API_KEY` / `POSTHOG_HOST` | PostHog analytics |
| `BOWRAIN_AUDIT_RETENTION_DAYS` | Prune audit-log rows older than N days (0 = keep forever) |
| `BOWRAIN_AUDIT_SIEM_WEBHOOK_URL` | Forward every audit event as NDJSON to an external SIEM |

### GitHub App

| Variable | Description |
| --- | --- |
| `GITHUB_APP_ID` | The registered GitHub App's id |
| `GITHUB_APP_PRIVATE_KEY_FILE` / `GITHUB_APP_PRIVATE_KEY` | The app's private key, as a file path or PEM text |
| `GITHUB_APP_WEBHOOK_SECRET` | The app's webhook secret |

See [The Bowrain GitHub App](/cli/ci/github-app) for registering the app.

### Rate limiting

Abuse-prone endpoints carry per-IP rate limits. Each knob is an integer; when
unset, unparsable, or non-positive, the compiled default applies. The tightest
caps are on the unauthenticated and email-sending routes. All limits are
requests per minute except the claim-email cap, which is per hour.

| Variable | Default | Limits |
| --- | --- | --- |
| `BOWRAIN_RL_ANON_PER_MIN` | `10` | Anonymous project creation (unauthenticated) |
| `BOWRAIN_RL_ANON_BURST` | `5` | Burst for the anonymous limiter |
| `BOWRAIN_RL_CLAIM_EMAIL_PER_HOUR` | `5` | Claim emails per client IP (hourly) |
| `BOWRAIN_RL_AUTH_PER_MIN` | `30` | Pre-auth token endpoints |
| `BOWRAIN_RL_AUTH_BURST` | `15` | Burst for the auth limiter |
| `BOWRAIN_RL_INVITE_PER_MIN` | `20` | Invite routes (may send email) |
| `BOWRAIN_RL_INVITE_BURST` | `10` | Burst for the invite limiter |
| `BOWRAIN_RL_AI_PER_MIN` | `20` | AI-consuming routes (AI drafting, voice check, context scan, @bravo) |
| `BOWRAIN_RL_AI_BURST` | `10` | Burst for the AI limiter |

## Worker environment variables

The worker (`bowrain-worker`) shares the database, the job queue and the blob
store with the server, ingests pushes, and runs the drafting jobs a run
enqueues.

| Variable | Description |
| --- | --- |
| `BOWRAIN_DATABASE_URL` | Same PostgreSQL connection string as the server |
| `BOWRAIN_QUEUE_BACKEND` / `SQS_ENDPOINT` | Same job-queue selection as the server (the two must agree on the broker) |
| `BOWRAIN_EVENT_BACKEND` / `BOWRAIN_REDIS_URL` | Same event-bus selection as the server |
| `LOCAL_BLOB_DIR` | Local blob directory; must point at the same shared volume as the server's `BLOB_STORAGE_LOCAL_DIR` (the `s3` variables above apply to the worker too) |
| `BOWRAIN_PLATFORM_PROVIDER` | AI provider for platform jobs: `bedrock`, `gemini`, `openai`, `anthropic`, `ollama`, or `demo` (offline) |
| `BOWRAIN_PLATFORM_API_KEY` | Provider API key (or a provider-specific variable such as `GEMINI_API_KEY`); Bedrock uses the AWS credential chain and `AWS_REGION` instead |
| `BOWRAIN_PLATFORM_MODEL` | Default model for the provider |
| `BOWRAIN_PLATFORM_BASE_URL` | Provider API base URL (for example self-hosted Ollama) |
| `BOWRAIN_OPENAI_ENDPOINT` | Azure OpenAI endpoint (managed identity); consulted only when `BOWRAIN_PLATFORM_PROVIDER` is unset |

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

- [Installation](/server/installation): quick start and native binaries.
- [Self-Hosting](/server/self-hosting): production deployment, OIDC, backups.
- [Getting Started](/server/getting-started): first login, workspaces, invitations.
