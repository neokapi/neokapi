---
sidebar_position: 12
title: "Docker Compose Development Setup"
---
# Docker Compose Development Setup

This note documents the `compose.yaml` configuration at the repository root, which provides external dependencies for local development without containerizing the bowrain-server itself.

## Dependencies-Only Approach

The Docker Compose setup deliberately does **not** run bowrain-server in a container. Instead, it provides only external infrastructure dependencies while the server runs natively on the host:

```
+-------------------+     +-------------------+
|   Keycloak        |     |   Mailpit         |
|   (OIDC provider) |     |   (dev SMTP)      |
|   :8180           |     |   :8025 / :1025   |
+-------------------+     +-------------------+
         |                         |
         +----------+--------------+
                    |
            +-------v-------+
            | bowrain-server |
            | (local binary) |
            |   :8080        |
            +----------------+
```

This pattern enables fast iteration: Go code changes only require rebuilding the server binary, not rebuilding a Docker image. The `make dev-server` target handles the full cycle.

## compose.yaml

```yaml
services:
  keycloak:
    image: quay.io/keycloak/keycloak:26.1
    command: start-dev --import-realm
    environment:
      KC_HEALTH_ENABLED: "true"
      KEYCLOAK_ADMIN: admin
      KEYCLOAK_ADMIN_PASSWORD: admin
    volumes:
      - ./docker/keycloak/realm.json:/opt/keycloak/data/import/realm.json
      - ./bowrain/apps/keycloak-theme/dist_keycloak/keycloak-theme-for-kc-all-other-versions.jar:/opt/keycloak/providers/keycloak-theme.jar
    ports:
      - "8180:8080"
    healthcheck:
      test: ["CMD-SHELL", "{ printf 'HEAD /health/ready HTTP/1.0\\r\\n\\r\\n' >&0; grep 'HTTP/1.0 200'; } 0<>/dev/tcp/localhost/9000"]
      interval: 10s
      timeout: 5s
      retries: 15

  mailpit:
    image: axllent/mailpit:latest
    ports:
      - "8025:8025"
      - "1025:1025"
```

### Keycloak (OIDC Provider)

- **Image**: `quay.io/keycloak/keycloak:26.1`
- **Mode**: `start-dev` (development mode, no TLS, no persistent database)
- **Port**: Host `8180` mapped to container `8080`
- **Admin console**: `http://localhost:8180` with credentials `admin`/`admin`
- **Realm import**: `--import-realm` loads `docker/keycloak/realm.json` at startup, which configures the `bowrain` realm with:
  - OIDC client `bowrain` (confidential, secret `bowrain-secret`)
  - OAuth2 device authorization grant enabled (for CLI auth)
  - Email-as-username registration with email verification
  - Pre-seeded user: `admin@example.com` / `password`
  - Google and GitHub identity providers (placeholder credentials)
- **Custom theme**: The built Keycloakify JAR is volume-mounted as a provider. The realm sets `loginTheme: "bowrain"`. See [Keycloak Theming](keycloak-theming.md) for details.
- **Health check**: Uses a raw TCP/HTTP probe against Keycloak's health endpoint on port 9000. The `--wait` flag in `docker compose up -d --wait` blocks until the health check passes.

### Mailpit (Development SMTP)

- **Image**: `axllent/mailpit:latest`
- **SMTP port**: Host `1025` (no auth, no TLS)
- **Web UI port**: Host `8025` for inspecting captured emails
- **Purpose**: Catches all outbound email from Keycloak (verification emails, password resets) and bowrain-server (invite emails). No emails leave the development machine.

The Keycloak realm configures SMTP to point to `mailpit:1025` (Docker network hostname). The bowrain-server uses `BOWRAIN_SMTP_HOST=localhost:1025` (host network).

## make dev-server Workflow

The `dev-server` Makefile target builds the server binary and launches it with environment variables pointing to the Docker dependencies:

```makefile
dev-server: build-server
	BOWRAIN_JWT_SECRET=dev-secret-change-in-production \
	BOWRAIN_OIDC_ISSUER_URL=http://localhost:8180/realms/bowrain \
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
# 1. Start infrastructure
docker compose up -d --wait

# 2. Build and run the server
make dev-server

# 3. Access services
#    bowrain-server:  http://localhost:8080
#    Keycloak admin:  http://localhost:8180  (admin/admin)
#    Mailpit inbox:   http://localhost:8025

# 4. Stop infrastructure
# (Ctrl-C the server first)
docker compose down -v
```

The `-v` flag on `docker compose down` removes volumes, ensuring a clean state for the next session. Keycloak runs in dev mode with no persistent storage, so realm data is re-imported from `realm.json` on every startup.

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

All three pipelines run in parallel in GitHub Actions (`.github/workflows/screenshots-recordings.yml`):
- On-demand via `workflow_dispatch`
- On release via version tags (auto-commits assets)
- Nightly at 2 AM UTC (uploads artifacts only)

## Makefile Convenience Targets

```makefile
dev-deps:      ## Start dev dependencies (Keycloak + Mailpit) in Docker
	docker compose up -d --wait

dev-deps-down: ## Stop dev dependencies
	docker compose down -v
```

These targets provide shorthand for developers who prefer `make dev-deps` over typing the full `docker compose` commands.
