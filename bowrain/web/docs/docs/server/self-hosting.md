---
title: Self-Hosting
sidebar_position: 12
---

# Self-Hosting

Run your own bowrain-server instance using Docker. The server includes an embedded
web UI, OIDC authentication, and SQLite-based storage that requires no external
database.

## Quick Start

The fastest way to get started is with Docker Compose. This sets up bowrain-server,
Keycloak (OIDC identity provider), and Mailpit (dev SMTP for email verification).

Create a `docker-compose.yml`:

```yaml
services:
  keycloak:
    image: quay.io/keycloak/keycloak:latest
    command: start-dev --import-realm
    environment:
      KC_HEALTH_ENABLED: "true"
      KEYCLOAK_ADMIN: admin
      KEYCLOAK_ADMIN_PASSWORD: admin
    volumes:
      - ./keycloak/realm.json:/opt/keycloak/data/import/realm.json
    ports:
      - "8180:8080"

  mailpit:
    image: axllent/mailpit:latest
    ports:
      - "8025:8025" # Web UI
      - "1025:1025" # SMTP

  bowrain:
    image: ghcr.io/neokapi/bowrain-server:latest
    ports:
      - "8080:8080"
    environment:
      - BOWRAIN_JWT_SECRET=change-this-to-a-random-secret
      - BOWRAIN_OIDC_ISSUER_URL=http://keycloak:8080/realms/bowrain
      - BOWRAIN_OIDC_PUBLIC_URL=http://localhost:8180/realms/bowrain
      - BOWRAIN_OIDC_CLIENT_ID=bowrain
      - BOWRAIN_OIDC_CLIENT_SECRET=bowrain-secret
      - BOWRAIN_STORE=/data/bowrain.db
      - BOWRAIN_SMTP_HOST=mailpit:1025
      - BOWRAIN_SMTP_FROM=noreply@bowrain.cloud
    volumes:
      - bowrain-data:/data
    depends_on:
      - keycloak

volumes:
  bowrain-data:
```

Create a `keycloak/realm.json` with your realm configuration (see the
`e2e/keycloak/realm.json` in the repository for an example), then start the stack:

```bash
docker compose up -d
```

Open `http://localhost:8080` to access the web UI. New users can self-register
through Keycloak at `http://localhost:8180`. Verification emails arrive in
Mailpit at `http://localhost:8025`.

## Environment Variables

| Variable                     | Default   | Description                                              |
| ---------------------------- | --------- | -------------------------------------------------------- |
| `BOWRAIN_PORT`               | `8080`    | HTTP port to listen on                                   |
| `BOWRAIN_HOST`               | `0.0.0.0` | Address to bind to                                       |
| `BOWRAIN_STORE`              |           | Path to SQLite database                                  |
| `BOWRAIN_DATA_DIR`           |           | Directory for temporary files                            |
| `BOWRAIN_JWT_SECRET`         |           | JWT signing secret (required for auth)                   |
| `BOWRAIN_OIDC_ISSUER_URL`    |           | OIDC issuer URL (internal, reachable from server)        |
| `BOWRAIN_OIDC_PUBLIC_URL`    |           | OIDC public URL (browser-facing; defaults to issuer URL) |
| `BOWRAIN_OIDC_CLIENT_ID`     |           | OIDC client ID                                           |
| `BOWRAIN_OIDC_CLIENT_SECRET` |           | OIDC client secret                                       |
| `BOWRAIN_SMTP_HOST`          |           | SMTP server host:port for transactional emails           |
| `BOWRAIN_SMTP_FROM`          |           | Sender email address for transactional emails            |
| `BOWRAIN_GRPC_PORT`          | `0`       | gRPC port (0 to disable)                                 |

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
- **Valid Redirect URIs**: `http://localhost:8080/api/v1/auth/callback`
- **Web Origins**: `http://localhost:8080`

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

The Docker image stores SQLite databases in `/data`. Use a named volume
or bind mount to persist data across container restarts:

```yaml
volumes:
  - /opt/bowrain/data:/data # Bind mount
  # or
  - bowrain-data:/data # Named volume
```

### Reverse Proxy

For production, put bowrain-server behind a reverse proxy (Nginx, Caddy, Traefik)
to handle TLS termination:

```nginx
server {
    listen 443 ssl;
    server_name bowrain.example.com;

    ssl_certificate /etc/ssl/certs/bowrain.crt;
    ssl_certificate_key /etc/ssl/private/bowrain.key;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

When using a reverse proxy, update the OIDC redirect URI and the
`BOWRAIN_OIDC_PUBLIC_URL` to use the public HTTPS URL.

## Docker Image Tags

| Tag      | Description                      |
| -------- | -------------------------------- |
| `latest` | Most recent release              |
| `X.Y.Z`  | Specific version (e.g., `0.5.0`) |

Pull a specific version:

```bash
docker pull ghcr.io/neokapi/bowrain-server:0.5.0
```

## Backup & Restore

bowrain-server uses SQLite databases stored in the `/data` volume:

- `bowrain.db` — content store (projects, blocks, workspaces)
- `bowrain.db.auth` — authentication store (users, tokens)

### Backup

**Option 1: Docker volume backup**

```bash
docker run --rm \
  -v bowrain-data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/bowrain-backup.tar.gz /data
```

**Option 2: SQLite online backup**

```bash
docker exec bowrain sqlite3 /data/bowrain.db ".backup /data/backup-content.db"
docker exec bowrain sqlite3 /data/bowrain.db.auth ".backup /data/backup-auth.db"
docker cp bowrain:/data/backup-content.db ./
docker cp bowrain:/data/backup-auth.db ./
```

### Restore

Reverse the backup process:

```bash
# From volume backup
docker run --rm \
  -v bowrain-data:/data \
  -v $(pwd):/backup \
  alpine sh -c "cd / && tar xzf /backup/bowrain-backup.tar.gz"
```

### Scheduled Backups

Add a cron job for regular backups:

```bash
# Daily backup at 2 AM
0 2 * * * docker run --rm -v bowrain-data:/data -v /opt/backups:/backup alpine tar czf /backup/bowrain-$(date +\%Y\%m\%d).tar.gz /data
```

## CLI Connection

Connect the Bowrain CLI to your self-hosted server:

```bash
kapi auth login --server https://bowrain.example.com
```

This starts a device authorization flow. Open the URL shown in your terminal,
authenticate with your identity provider, and the CLI receives a token
automatically.
