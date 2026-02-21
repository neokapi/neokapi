---
sidebar_position: 12
title: "Docker Compose Development Setup"
---
# Docker Compose Development Setup

This note documents the `compose.yaml` configuration at the repository root, which provides external dependencies for local development without containerizing the bowrain-server itself.

## Architecture

The Docker Compose setup runs **Traefik** as a TLS-terminating reverse proxy in front of containerized services (Keycloak, Mailpit) and host-running services (bowrain-server, Vite dev server). This gives local development a production-like HTTPS experience.

```
                     ┌──────────────┐
                     │   Traefik    │
                     │   :80 → :443 │
                     └──────┬───────┘
           ┌────────────────┼────────────────┐
           │                │                │
  ┌────────▼────────┐ ┌────▼─────┐ ┌────────▼────────┐
  │   Keycloak      │ │ Mailpit  │ │ host.docker.     │
  │ auth.bowrain.   │ │ mail.    │ │    internal      │
  │    mymac        │ │ bowrain. │ │                  │
  │ (OIDC provider) │ │  mymac   │ │ bowrain-server   │
  │ container:8080  │ │ :8025    │ │ host:8080        │
  └─────────────────┘ └──────────┘ │ Vite dev server  │
                                   │ host:5173        │
                                   └──────────────────┘
```

| Service | URL | Routes to |
|---|---|---|
| Web app (dev) | `https://bowrain.mymac` | host:5173 (Vite HMR) |
| API | `https://bowrain.mymac/api/*` | host:8080 (bowrain-server) |
| Keycloak | `https://auth.bowrain.mymac` | keycloak container:8080 |
| Mailpit | `https://mail.bowrain.mymac` | mailpit container:8025 |
| Traefik dashboard | `https://traefik.bowrain.mymac` | traefik:8080 |

The bowrain-server and Vite dev server run **natively on the host** for fast iteration — no Docker image rebuild needed for Go or TypeScript changes. Traefik reaches them via `host.docker.internal`.

## First-Time Setup (macOS)

One-time prerequisites for DNS resolution and TLS certificates:

```bash
# 1. Install tools
brew install dnsmasq mkcert

# 2. Configure dnsmasq to resolve *.mymac → 127.0.0.1
echo 'address=/.mymac/127.0.0.1' >> $(brew --prefix)/etc/dnsmasq.conf
sudo brew services restart dnsmasq
sudo mkdir -p /etc/resolver
echo 'nameserver 127.0.0.1' | sudo tee /etc/resolver/mymac

# 3. Install mkcert CA into system trust store
mkcert -install

# 4. Generate wildcard TLS certificate
make certs
```

After this, `*.bowrain.mymac` resolves to localhost and the mkcert CA is trusted by browsers and Go's `net/http` client.

## compose.yaml

```yaml
services:
  traefik:
    image: traefik:v3
    volumes:
      - ./docker/traefik/traefik.yml:/etc/traefik/traefik.yml:ro
      - ./docker/traefik/dynamic.yml:/etc/traefik/dynamic.yml:ro
      - ./docker/traefik/certs:/etc/certs:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro
    ports:
      - "80:80"
      - "443:443"

  keycloak:
    image: quay.io/keycloak/keycloak:26.1
    command: start-dev --import-realm
    environment:
      KC_HOSTNAME: https://auth.bowrain.mymac
      KC_PROXY_HEADERS: xforwarded
      # ...
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.keycloak.rule=Host(`auth.bowrain.mymac`)"
      - "traefik.http.routers.keycloak.tls=true"
      - "traefik.http.services.keycloak.loadbalancer.server.port=8080"

  mailpit:
    image: axllent/mailpit:latest
    ports:
      - "1025:1025"   # SMTP still on host for bowrain-server
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.mailpit.rule=Host(`mail.bowrain.mymac`)"
      - "traefik.http.routers.mailpit.tls=true"
      - "traefik.http.services.mailpit.loadbalancer.server.port=8025"
```

### Traefik

- **Image**: `traefik:v3`
- **Static config**: `docker/traefik/traefik.yml` — entrypoints (80→443 redirect), Docker + file providers, dashboard
- **Dynamic config**: `docker/traefik/dynamic.yml` — routers for `bowrain.mymac` (API at priority 100, Vite at 90), TLS cert paths, dashboard router
- **TLS certificates**: `docker/traefik/certs/` — mkcert-generated wildcard cert (`*.bowrain.mymac`), gitignored
- **Docker labels**: Used for containerized services (Keycloak, Mailpit)
- **File provider**: Used for host-running services (bowrain-server, Vite) via `host.docker.internal`

### Keycloak (OIDC Provider)

- **Image**: `quay.io/keycloak/keycloak:26.1`
- **Mode**: `start-dev` (development mode, no persistent database)
- **Hostname**: `KC_HOSTNAME=https://auth.bowrain.mymac` — Keycloak uses this for OIDC discovery URLs and redirect validation
- **Proxy**: `KC_PROXY_HEADERS=xforwarded` — trusts Traefik's `X-Forwarded-*` headers
- **Admin console**: `https://auth.bowrain.mymac` with credentials `admin`/`admin`
- **Realm import**: `--import-realm` loads `docker/keycloak/realm.json` at startup, which configures the `bowrain` realm with:
  - OIDC client `bowrain` (confidential, secret `bowrain-secret`)
  - OAuth2 device authorization grant enabled (for CLI auth)
  - Email-as-username registration with email verification
  - Pre-seeded user: `admin@example.com` / `password`
  - Google and GitHub identity providers (placeholder credentials)
  - Both HTTPS (`https://bowrain.mymac/*`) and HTTP (`http://localhost:8080/*`) redirect URIs for local and CI compatibility
- **Custom theme**: The built Keycloakify JAR is volume-mounted as a provider. The realm sets `loginTheme: "bowrain"`. See [Keycloak Theming](keycloak-theming.md) for details.
- **Health check**: Uses a raw TCP/HTTP probe against Keycloak's health endpoint on port 9000. The `--wait` flag in `docker compose up -d --wait` blocks until the health check passes.

### Mailpit (Development SMTP)

- **Image**: `axllent/mailpit:latest`
- **SMTP port**: Host `1025` (no auth, no TLS) — directly mapped for bowrain-server
- **Web UI**: `https://mail.bowrain.mymac` via Traefik
- **Purpose**: Catches all outbound email from Keycloak (verification emails, password resets) and bowrain-server (invite emails). No emails leave the development machine.

The Keycloak realm configures SMTP to point to `mailpit:1025` (Docker network hostname). The bowrain-server uses `BOWRAIN_SMTP_HOST=localhost:1025` (host network).

## make dev-server Workflow

The `dev-server` Makefile target builds the server binary and launches it with environment variables pointing to the Docker dependencies:

```makefile
dev-server: build-server
	BOWRAIN_JWT_SECRET=dev-secret-change-in-production \
	BOWRAIN_OIDC_ISSUER_URL=https://auth.bowrain.mymac/realms/bowrain \
	BOWRAIN_OIDC_CLIENT_ID=bowrain \
	BOWRAIN_OIDC_CLIENT_SECRET=bowrain-secret \
	BOWRAIN_SMTP_HOST=localhost:1025 \
	BOWRAIN_SMTP_FROM=noreply@bowrain.cloud \
	BOWRAIN_STORE=bowrain-dev.db \
	BOWRAIN_GRPC_PORT=9080 \
	bin/bowrain-server
```

The `build-server` prerequisite chains through `web-build`, which in turn depends on `ui-deps` and `web-deps`, so a single `make dev-server` command handles the entire build pipeline from shared UI to server binary.

The development database (`bowrain-dev.db`) is a SQLite file created in the current directory. It is gitignored (`bowrain-dev.db*` matches both the database and its WAL/SHM files).

## Typical Development Session

```bash
# 1. Start infrastructure (Traefik + Keycloak + Mailpit)
docker compose up -d --wait

# 2. Build and run the server
make dev-server

# 3. (In another terminal) Start Vite dev server with HMR
make dev-web

# 4. Access services
#    Web app:     https://bowrain.mymac
#    Keycloak:    https://auth.bowrain.mymac  (admin/admin)
#    Mailpit:     https://mail.bowrain.mymac
#    Traefik:     https://traefik.bowrain.mymac

# 5. Stop infrastructure
# (Ctrl-C the server and Vite first)
docker compose down -v
```

The `-v` flag on `docker compose down` removes volumes, ensuring a clean state for the next session. Keycloak runs in dev mode with no persistent storage, so realm data is re-imported from `realm.json` on every startup.

## CI Overlay (compose.ci.yaml)

CI runs without Traefik, mkcert, or DNS configuration. A `compose.ci.yaml` overlay re-exposes direct ports and disables the TLS proxy:

```bash
docker compose -f compose.yaml -f compose.ci.yaml up -d --wait
```

This overlay:
- Disables the `traefik` service via `profiles: ["disabled"]`
- Re-exposes Keycloak on host port `8180`
- Re-exposes Mailpit web UI on host port `8025`
- Clears `KC_HOSTNAME` and `KC_PROXY_HEADERS` so Keycloak works without a proxy

CI scripts (e.g. `e2e/setup.sh`, GitHub Actions workflows) use `http://localhost:8180/realms/bowrain` as the OIDC issuer and `http://localhost:8080` as the server URL.

## Supporting E2E Testing

The Docker Compose setup supports three testing pipelines:

### Web App Screenshots and Recordings

The web app's Playwright tests (`bowrain/apps/web/e2e/`) require a running bowrain-server with Keycloak for real OIDC authentication:

```bash
docker compose up -d --wait
make dev-server &
cd bowrain/apps/web && npm run e2e:screenshots
cd bowrain/apps/web && npm run e2e:recordings
```

Recordings are captured per-theme (glass, light, aurora) and copied to the website static directory:

```bash
THEME=glass  bash bowrain/apps/web/scripts/copy-recordings.sh
THEME=light  bash bowrain/apps/web/scripts/copy-recordings.sh
THEME=aurora bash bowrain/apps/web/scripts/copy-recordings.sh
```

### Bowrain Desktop Screenshots and Recordings

The Bowrain desktop app's Playwright tests (`bowrain/apps/bowrain/frontend/e2e/`) are self-contained. They auto-start a Vite dev server and do not require Docker Compose or a running bowrain-server:

```bash
make screenshots    # screenshots -> website/static/img/bowrain/{dark,light}/
make recordings     # recordings -> website/static/video/bowrain/{dark,light}/
```

### CLI Recordings

VHS terminal recordings (`website/tapes/*.tape`) generate CLI demo videos. Some tapes require a running server:

```bash
make cli-recordings
```

### CI Integration

All three pipelines run in parallel in GitHub Actions (`.github/workflows/screenshots-recordings.yml`), using the CI overlay for HTTP-only mode:
- On-demand via `workflow_dispatch`
- On release via version tags (auto-commits assets)
- Nightly at 2 AM UTC (uploads artifacts only)

## Makefile Convenience Targets

```makefile
certs:         ## Generate mkcert TLS certificates for *.bowrain.mymac
dev-deps:      ## Start dev dependencies (Traefik + Keycloak + Mailpit) in Docker
dev-deps-down: ## Stop dev dependencies
```

These targets provide shorthand for developers who prefer `make dev-deps` over typing the full `docker compose` commands.
