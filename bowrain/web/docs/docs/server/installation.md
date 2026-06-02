---
sidebar_position: 2
title: Installation
---

# Installing Bowrain Server

A Bowrain deployment is several cooperating services, not a single binary:

- **bowrain-server** — the REST + gRPC API (one process; gRPC is multiplexed onto the HTTP port).
- **bowrain-worker** — the async worker that ingests pushes and runs the auto-translate-on-push automation against an upstream translation provider.
- **PostgreSQL** — the authoritative store (projects, blocks, workspaces, users, jobs). The server requires PostgreSQL; **there is no SQLite or file backend.**
- **NATS** — the job queue and event bus shared by the server and worker.
- **bowrain-web** — the static web UI, served as its own container.
- An **OIDC identity provider** (e.g. Keycloak) and an **SMTP** sender.

This page covers local evaluation. For a production stack with TLS, backups, and a reverse proxy, see [Self-Hosting](/server/self-hosting), which is the canonical reference for the full architecture and the complete environment-variable set.

## One-command local stack

The repository ships a self-contained local stack at [`bowrain/compose.full.yaml`](https://github.com/neokapi/neokapi/blob/main/bowrain/compose.full.yaml) — server, worker, PostgreSQL, NATS, Redis, Keycloak (with a pre-imported realm), and Mailpit. It defaults to the offline `demo` translation provider, so the full push → translate → pull cycle works with no API keys and no OIDC setup:

```bash
docker compose -f bowrain/compose.full.yaml up -d --build --wait
```

Endpoints once it is up:

| URL | Service |
| --- | --- |
| `http://localhost:8080` | bowrain-server API (and the web UI with `--profile web`) |
| `http://localhost:8080/api/v1/health` | Health check |
| `http://localhost:8180` | Keycloak admin console (admin / admin) |
| `http://localhost:8025` | Mailpit (captured emails) |

To serve the web UI from the server, add the `web` profile:

```bash
docker compose -f bowrain/compose.full.yaml --profile web up -d --build
```

For real translations, set `BOWRAIN_PLATFORM_PROVIDER` (e.g. `gemini`) and the matching API key in a `.env` file — see [Self-Hosting](/server/self-hosting#environment-variables).

Tear down with `docker compose -f bowrain/compose.full.yaml down -v`.

## Self-hosting from published images

To run from the published `ghcr.io/neokapi/` images against your own OIDC provider, use the reference stack at [`bowrain/deploy/docker/compose.yaml`](https://github.com/neokapi/neokapi/blob/main/bowrain/deploy/docker/compose.yaml) — Traefik, PostgreSQL, NATS, the server, the worker, and the web UI.

```bash
docker compose -f deploy/docker/compose.yaml up -d
```

At minimum, provide an external OIDC issuer, a JWT secret, and (for auto-translate) an upstream provider:

```bash
POSTGRES_PASSWORD=...                          # database password
BOWRAIN_JWT_SECRET=$(openssl rand -base64 32)  # JWT signing secret
BOWRAIN_OIDC_ISSUER_URL=...                    # your realm's issuer URL
BOWRAIN_OIDC_CLIENT_SECRET=...                 # the bowrain client's secret
BOWRAIN_PLATFORM_PROVIDER=gemini               # or openai / anthropic / ollama
BOWRAIN_PLATFORM_API_KEY=...                   # provider API key
```

See [Self-Hosting](/server/self-hosting) for the full production walkthrough, including TLS, backups, and the complete service topology.

## Native binary

The server and worker also ship as native binaries on [GitHub Releases](https://github.com/neokapi/neokapi/releases). Both still require a reachable PostgreSQL and NATS.

```bash
# Linux (x86_64)
curl -LO https://github.com/neokapi/neokapi/releases/latest/download/bowrain-server-linux-amd64.tar.gz
tar xzf bowrain-server-linux-amd64.tar.gz
sudo mv bowrain-server /usr/local/bin/
```

Run the server (PostgreSQL is required; the connection string must use the `postgres://` scheme):

```bash
bowrain-server \
  --database-url postgres://bowrain:password@localhost/bowrain \
  --jwt-secret change-me-in-production \
  --oidc-issuer-url https://keycloak.example.com/realms/bowrain \
  --oidc-client-id bowrain \
  --oidc-client-secret your-client-secret \
  --port 8080
```

The schema is created automatically on first start; migrations run on startup. The async worker is configured entirely through environment variables — see [Configuration](/server/configuration).

### systemd service

`/etc/systemd/system/bowrain-server.service`:

```ini
[Unit]
Description=Bowrain Server
After=network.target

[Service]
Type=simple
User=bowrain
Group=bowrain
Environment=BOWRAIN_DATABASE_URL=postgres://bowrain:password@localhost/bowrain
Environment=BOWRAIN_NATS_URL=nats://localhost:4222
ExecStart=/usr/local/bin/bowrain-server \
  --jwt-secret ${BOWRAIN_JWT_SECRET} \
  --oidc-issuer-url https://keycloak.example.com/realms/bowrain \
  --oidc-client-id bowrain \
  --oidc-client-secret ${BOWRAIN_OIDC_CLIENT_SECRET} \
  --port 8080
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now bowrain-server
sudo systemctl status bowrain-server
```

## OIDC provider setup

Bowrain Server requires an OIDC provider for authentication. Any OIDC-compliant
provider works (Keycloak, Auth0, Okta, Azure AD, Dex). The local stack above
imports a pre-configured Keycloak realm automatically; for your own provider see
the [OIDC provider setup in Self-Hosting](/server/self-hosting#oidc-provider-setup).

## Health check

Verify the server is running:

```bash
curl http://localhost:8080/api/v1/health
```

## Next steps

- [Configuration](/server/configuration) — the complete environment-variable and CLI-flag reference.
- [Getting Started](/server/getting-started) — first login, workspaces, invitations.
- [Self-Hosting](/server/self-hosting) — production deployment with TLS and backups.
