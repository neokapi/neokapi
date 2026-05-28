---
sidebar_position: 3
title: Configuration
---

# Server Configuration

Configure Bowrain Server via environment variables or command-line flags.

## Precedence

Command-line flags take priority over defaults. Environment variables override both.

## Environment Variables

All environment variables use the `BOWRAIN_` prefix.

### Server

| Variable            | Description                   | Default                           |
| ------------------- | ----------------------------- | --------------------------------- |
| `BOWRAIN_PORT`      | HTTP port to listen on        | `8080`                            |
| `BOWRAIN_HOST`      | Address to bind to            | `0.0.0.0`                         |
| `BOWRAIN_STORE`     | Path to SQLite database       | _(empty — content APIs disabled)_ |
| `BOWRAIN_DATA_DIR`  | Directory for temporary files | _(empty)_                         |
| `BOWRAIN_GRPC_PORT` | gRPC port (0 to disable)      | `0`                               |

### Authentication

| Variable                     | Description                                           | Default                                   |
| ---------------------------- | ----------------------------------------------------- | ----------------------------------------- |
| `BOWRAIN_JWT_SECRET`         | JWT signing secret                                    | _(empty)_                                 |
| `BOWRAIN_OIDC_ISSUER_URL`    | OIDC issuer URL (internal, used for token validation) | _(empty)_                                 |
| `BOWRAIN_OIDC_PUBLIC_URL`    | Browser-facing OIDC URL (for redirects)               | _falls back to `BOWRAIN_OIDC_ISSUER_URL`_ |
| `BOWRAIN_OIDC_CLIENT_ID`     | OIDC OAuth client ID                                  | _(empty)_                                 |
| `BOWRAIN_OIDC_CLIENT_SECRET` | OIDC OAuth client secret                              | _(empty)_                                 |

### Email

| Variable            | Description                       | Default                    |
| ------------------- | --------------------------------- | -------------------------- |
| `BOWRAIN_SMTP_HOST` | SMTP server in `host:port` format | _(empty — email disabled)_ |
| `BOWRAIN_SMTP_FROM` | Sender email address              | _(empty)_                  |

:::tip OIDC Public URL
When your OIDC provider has a different internal hostname than the browser-facing URL (common in Docker), set `BOWRAIN_OIDC_ISSUER_URL` to the internal URL (e.g. `http://keycloak:8080/realms/bowrain`) and `BOWRAIN_OIDC_PUBLIC_URL` to the browser-facing URL (e.g. `http://localhost:8180/realms/bowrain`). If not set, `BOWRAIN_OIDC_PUBLIC_URL` defaults to `BOWRAIN_OIDC_ISSUER_URL`.
:::

## Command-Line Flags

```bash
bowrain-server \
  --port 8080 \
  --host 0.0.0.0 \
  --store /data/bowrain.db \
  --data-dir /tmp/bowrain \
  --jwt-secret your-secret \
  --oidc-issuer-url https://keycloak.example.com/realms/bowrain \
  --oidc-client-id bowrain \
  --oidc-client-secret your-client-secret \
  --grpc-port 9090 \
  --web-ui-dir /path/to/web/dist
```

| Flag                   | Description                       | Default      |
| ---------------------- | --------------------------------- | ------------ |
| `--port`               | HTTP port to listen on            | `8080`       |
| `--host`               | Address to bind to                | `0.0.0.0`    |
| `--store`              | Path to SQLite database           | _(empty)_    |
| `--data-dir`           | Directory for temporary files     | _(empty)_    |
| `--jwt-secret`         | JWT signing secret                | _(empty)_    |
| `--oidc-issuer-url`    | OIDC issuer URL                   | _(empty)_    |
| `--oidc-client-id`     | OIDC OAuth client ID              | _(empty)_    |
| `--oidc-client-secret` | OIDC OAuth client secret          | _(empty)_    |
| `--grpc-port`          | gRPC port (0 to disable)          | `0`          |
| `--web-ui-dir`         | Path to built web UI static files | _(embedded)_ |

## Database

Bowrain Server requires **PostgreSQL**. Set the `BOWRAIN_DATABASE_URL` environment variable to a PostgreSQL connection string:

```bash
BOWRAIN_DATABASE_URL=postgres://bowrain:password@localhost/bowrain
```

The schema is created automatically on first start; migrations run on startup.

## Example: Docker Compose

See the [Installation guide](/server/installation) for a complete Docker Compose example with Keycloak and Mailpit.

## Next Steps

- [Getting Started](/server/getting-started) — first login, workspaces, invitations
- [Workspaces](/server/workspaces) — workspace concepts and API reference
- [Automation](/server/automation) — CI/CD integration
