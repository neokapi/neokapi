---
title: Self-Hosting
sidebar_position: 12
---

# Self-Hosting

Run your own bowrain-server instance using Docker. The server includes an embedded
web UI, OIDC authentication via [Dex](https://dexidp.io/), and SQLite-based
storage that requires no external database.

## Quick Start

The fastest way to get started is with Docker Compose. This sets up both
bowrain-server and Dex (the OIDC identity provider).

Create a `docker-compose.yml`:

```yaml
services:
  dex:
    image: dexidp/dex:latest
    volumes:
      - ./dex/config.yaml:/etc/dex/config.yaml
      - dex-data:/var/dex
    ports:
      - "5556:5556"
    command: ["dex", "serve", "/etc/dex/config.yaml"]

  gokapi:
    image: ghcr.io/gokapi/bowrain-server:latest
    ports:
      - "8080:8080"
    environment:
      - GOKAPI_JWT_SECRET=change-this-to-a-random-secret
      - GOKAPI_DEX_ISSUER_URL=http://dex:5556/dex
      - GOKAPI_DEX_CLIENT_ID=gokapi
      - GOKAPI_DEX_CLIENT_SECRET=gokapi-secret
      - GOKAPI_STORE=/data/gokapi.db
    volumes:
      - gokapi-data:/data
    depends_on:
      - dex

volumes:
  dex-data:
  gokapi-data:
```

Create `dex/config.yaml` with your identity provider configuration (see
[Dex Connectors](#dex-connector-examples) below), then start the stack:

```bash
docker compose up -d
```

Open `http://localhost:8080` to access the web UI.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GOKAPI_PORT` | `8080` | HTTP port to listen on |
| `GOKAPI_HOST` | `0.0.0.0` | Address to bind to |
| `GOKAPI_STORE` | | Path to SQLite database |
| `GOKAPI_DATA_DIR` | | Directory for temporary files |
| `GOKAPI_JWT_SECRET` | | JWT signing secret (required for auth) |
| `GOKAPI_DEX_ISSUER_URL` | | Dex OIDC issuer URL |
| `GOKAPI_DEX_CLIENT_ID` | | Dex OAuth client ID |
| `GOKAPI_DEX_CLIENT_SECRET` | | Dex OAuth client secret |
| `GOKAPI_GRPC_PORT` | `0` | gRPC port (0 to disable) |

## Dex Connector Examples

Dex supports many [identity providers](https://dexidp.io/docs/connectors/).
Here are common configurations.

### GitHub

```yaml
connectors:
  - type: github
    id: github
    name: GitHub
    config:
      clientID: $GITHUB_CLIENT_ID
      clientSecret: $GITHUB_CLIENT_SECRET
      redirectURI: http://localhost:5556/dex/callback
      orgs:
        - name: your-org
```

### Google

```yaml
connectors:
  - type: google
    id: google
    name: Google
    config:
      clientID: $GOOGLE_CLIENT_ID
      clientSecret: $GOOGLE_CLIENT_SECRET
      redirectURI: http://localhost:5556/dex/callback
      hostedDomains:
        - your-domain.com
```

### LDAP

```yaml
connectors:
  - type: ldap
    id: ldap
    name: LDAP
    config:
      host: ldap.example.com:636
      rootCA: /etc/dex/ldap-ca.pem
      bindDN: cn=admin,dc=example,dc=com
      bindPW: admin-password
      userSearch:
        baseDN: ou=users,dc=example,dc=com
        filter: "(objectClass=person)"
        username: uid
        idAttr: uid
        emailAttr: mail
        nameAttr: cn
```

### Development (Mock)

For local development and testing:

```yaml
connectors:
  - type: mockCallback
    id: mock
    name: "Login with Email"
```

## Production Tips

### JWT Secret

Generate a strong random secret for `GOKAPI_JWT_SECRET`:

```bash
openssl rand -base64 32
```

Never use the default development secret in production.

### Persistent Storage

The Docker image stores SQLite databases in `/data`. Use a named volume
or bind mount to persist data across container restarts:

```yaml
volumes:
  - /opt/gokapi/data:/data  # Bind mount
  # or
  - gokapi-data:/data       # Named volume
```

### Reverse Proxy

For production, put bowrain-server behind a reverse proxy (Nginx, Caddy, Traefik)
to handle TLS termination:

```nginx
server {
    listen 443 ssl;
    server_name gokapi.example.com;

    ssl_certificate /etc/ssl/certs/gokapi.crt;
    ssl_certificate_key /etc/ssl/private/gokapi.key;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

When using a reverse proxy, update the Dex redirect URI and `GOKAPI_DEX_ISSUER_URL`
to use the public HTTPS URL.

## Docker Image Tags

| Tag | Description |
|-----|-------------|
| `latest` | Most recent release |
| `X.Y.Z` | Specific version (e.g., `0.5.0`) |

Pull a specific version:

```bash
docker pull ghcr.io/gokapi/bowrain-server:0.5.0
```

## Backup & Restore

bowrain-server uses SQLite databases stored in the `/data` volume:

- `gokapi.db` — content store (projects, blocks, workspaces)
- `gokapi.db.auth` — authentication store (users, tokens)

### Backup

**Option 1: Docker volume backup**

```bash
docker run --rm \
  -v gokapi-data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/gokapi-backup.tar.gz /data
```

**Option 2: SQLite online backup**

```bash
docker exec gokapi sqlite3 /data/gokapi.db ".backup /data/backup-content.db"
docker exec gokapi sqlite3 /data/gokapi.db.auth ".backup /data/backup-auth.db"
docker cp gokapi:/data/backup-content.db ./
docker cp gokapi:/data/backup-auth.db ./
```

### Restore

Reverse the backup process:

```bash
# From volume backup
docker run --rm \
  -v gokapi-data:/data \
  -v $(pwd):/backup \
  alpine sh -c "cd / && tar xzf /backup/gokapi-backup.tar.gz"
```

### Scheduled Backups

Add a cron job for regular backups:

```bash
# Daily backup at 2 AM
0 2 * * * docker run --rm -v gokapi-data:/data -v /opt/backups:/backup alpine tar czf /backup/gokapi-$(date +\%Y\%m\%d).tar.gz /data
```

### Dex Data

Dex stores its own database. Back it up separately:

```bash
docker run --rm \
  -v dex-data:/var/dex \
  -v $(pwd):/backup \
  alpine tar czf /backup/dex-backup.tar.gz /var/dex
```

## CLI Connection

Connect the kapi CLI to your self-hosted server:

```bash
kapi auth login --server https://gokapi.example.com
```

This starts a device authorization flow. Open the URL shown in your terminal,
authenticate with your identity provider, and the CLI receives a token
automatically.
