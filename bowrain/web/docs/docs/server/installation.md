---
sidebar_position: 2
title: Installation
---

# Installing Bowrain Server

Deploy Bowrain Server using Docker, Docker Compose, or as a native binary. Bowrain Server uses **PostgreSQL** for storage and any **OIDC provider** (e.g. Keycloak) for authentication.

:::tip Production deployments
For a production-ready stack with Traefik, PostgreSQL, NATS, and all required services, see [Self-Hosting](/server/self-hosting). The quick-start below is for development and evaluation.
:::

## Docker (Recommended)

### Quick Start

```bash
docker run -d \
  --name bowrain-server \
  -p 8080:8080 \
  -v bowrain-data:/data \
  -e BOWRAIN_STORE=/data/bowrain.db \
  -e BOWRAIN_JWT_SECRET=change-me-in-production \
  -e BOWRAIN_OIDC_ISSUER_URL=https://keycloak.example.com/realms/bowrain \
  -e BOWRAIN_OIDC_CLIENT_ID=bowrain \
  -e BOWRAIN_OIDC_CLIENT_SECRET=your-client-secret \
  ghcr.io/neokapi/bowrain-server:latest
```

### Docker Compose

This is the recommended setup for self-hosting. It runs Keycloak (identity provider), Mailpit (email for development), and Bowrain Server together:

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
    healthcheck:
      test:
        [
          "CMD-SHELL",
          "exec 3<>/dev/tcp/localhost/8080 && echo -e 'GET /health/ready HTTP/1.1\r\nHost: localhost\r\n\r\n' >&3 && cat <&3 | grep -q '200'",
        ]
      interval: 10s
      timeout: 5s
      retries: 12

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
      - BOWRAIN_JWT_SECRET=change-me-in-production
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
      keycloak:
        condition: service_healthy

volumes:
  bowrain-data:
```

Start the stack:

```bash
docker compose up -d
```

Open `http://localhost:8080` in your browser. The Keycloak admin console is available at `http://localhost:8180` (admin / admin). Mailpit's web UI is at `http://localhost:8025`.

:::tip OIDC Public URL
When Keycloak runs in Docker, it has a different internal hostname (`keycloak:8080`) than the browser-facing URL (`localhost:8180`). Set `BOWRAIN_OIDC_PUBLIC_URL` to the browser-facing URL and `BOWRAIN_OIDC_ISSUER_URL` to the internal URL.
:::

## Native Binary

### Download

Download the latest release from [GitHub Releases](https://github.com/neokapi/neokapi/releases):

```bash
# Linux (x86_64)
curl -LO https://github.com/neokapi/neokapi/releases/latest/download/bowrain-server-linux-amd64.tar.gz
tar xzf bowrain-server-linux-amd64.tar.gz
sudo mv bowrain-server /usr/local/bin/

# Linux (ARM64)
curl -LO https://github.com/neokapi/neokapi/releases/latest/download/bowrain-server-linux-arm64.tar.gz
tar xzf bowrain-server-linux-arm64.tar.gz
sudo mv bowrain-server /usr/local/bin/

# macOS (Apple Silicon)
curl -LO https://github.com/neokapi/neokapi/releases/latest/download/bowrain-server-darwin-arm64.tar.gz
tar xzf bowrain-server-darwin-arm64.tar.gz
sudo mv bowrain-server /usr/local/bin/
```

### Run

```bash
bowrain-server \
  --store /var/lib/bowrain/bowrain.db \
  --jwt-secret change-me-in-production \
  --oidc-issuer-url https://keycloak.example.com/realms/bowrain \
  --oidc-client-id bowrain \
  --oidc-client-secret your-client-secret
```

### systemd Service

`/etc/systemd/system/bowrain-server.service`:

```ini
[Unit]
Description=Bowrain Server
After=network.target

[Service]
Type=simple
User=bowrain
Group=bowrain
ExecStart=/usr/local/bin/bowrain-server \
  --store /var/lib/bowrain/bowrain.db \
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
sudo systemctl enable bowrain-server
sudo systemctl start bowrain-server
sudo systemctl status bowrain-server
```

## OIDC Provider Setup

Bowrain Server requires an OIDC provider for authentication. We recommend [Keycloak](https://www.keycloak.org/).

### Keycloak Realm Configuration

1. Create a new realm (e.g. `bowrain`)
2. Create a client:
   - **Client ID**: `bowrain`
   - **Client Protocol**: `openid-connect`
   - **Access Type**: `confidential`
   - **Valid Redirect URIs**: `https://bowrain.example.com/*`
3. Copy the client secret from the **Credentials** tab
4. Enable **User Registration** if you want self-service sign-up

For local development, the Docker Compose setup above imports a pre-configured realm automatically.

## Health Check

Verify the server is running:

```bash
curl http://localhost:8080/api/v1/health
```

## Next Steps

- [Configuration](/server/configuration) — all environment variables and CLI flags
- [Getting Started](/server/getting-started) — first login, workspaces, invitations
- [Self-Hosting](/server/self-hosting) — production deployment with TLS and backups
