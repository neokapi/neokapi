---
sidebar_position: 2
title: Installation
---

# Installing Bowrain Server

Deploy Bowrain Server using Docker, Kubernetes, or as a native binary.

## Docker (Recommended)

### Quick Start

```bash
docker run -d \
  --name bowrain-server \
  -p 8080:8080 \
  -e DATABASE_URL=postgres://user:pass@localhost/bowrain \
  -e OIDC_ISSUER=https://dex.example.com \
  -e OIDC_CLIENT_ID=bowrain \
  -e OIDC_CLIENT_SECRET=secret \
  ghcr.io/gokapi/bowrain-server:latest
```

### Docker Compose

`docker-compose.yml`:

```yaml
version: '3.8'

services:
  bowrain:
    image: ghcr.io/gokapi/bowrain-server:latest
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: postgres://bowrain:password@db:5432/bowrain
      OIDC_ISSUER: https://dex.example.com
      OIDC_CLIENT_ID: bowrain
      OIDC_CLIENT_SECRET: ${OIDC_CLIENT_SECRET}
    depends_on:
      - db

  db:
    image: postgres:16
    environment:
      POSTGRES_USER: bowrain
      POSTGRES_PASSWORD: password
      POSTGRES_DB: bowrain
    volumes:
      - postgres_data:/var/lib/postgresql/data

  dex:
    image: dexidp/dex:v2.37.0
    ports:
      - "5556:5556"
    volumes:
      - ./dex-config.yaml:/etc/dex/config.yaml

volumes:
  postgres_data:
```

Start the stack:

```bash
docker compose up -d
```

## Kubernetes

### Helm Chart

Add the Gokapi Helm repository:

```bash
helm repo add gokapi https://gokapi.github.io/helm-charts
helm repo update
```

Install Bowrain Server:

```bash
helm install bowrain gokapi/bowrain-server \
  --set database.url=postgres://... \
  --set oidc.issuer=https://dex.example.com \
  --set oidc.clientId=bowrain \
  --set oidc.clientSecret=... \
  --set ingress.enabled=true \
  --set ingress.host=bowrain.example.com
```

### Custom Values

`values.yaml`:

```yaml
replicaCount: 3

image:
  repository: ghcr.io/gokapi/bowrain-server
  tag: latest

database:
  url: postgres://bowrain:password@postgres:5432/bowrain

oidc:
  issuer: https://dex.example.com
  clientId: bowrain
  clientSecret: secret
  redirectUrl: https://bowrain.example.com/auth/callback

ingress:
  enabled: true
  className: nginx
  host: bowrain.example.com
  tls:
    enabled: true
    secretName: bowrain-tls

resources:
  requests:
    memory: "512Mi"
    cpu: "500m"
  limits:
    memory: "2Gi"
    cpu: "2000m"

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 80
```

Install with custom values:

```bash
helm install bowrain gokapi/bowrain-server -f values.yaml
```

## Native Binary

### Download

Download the latest release from [GitHub Releases](https://github.com/gokapi/gokapi/releases):

```bash
# Linux (x86_64)
curl -LO https://github.com/gokapi/gokapi/releases/latest/download/bowrain-server-linux-amd64.tar.gz
tar xzf bowrain-server-linux-amd64.tar.gz
sudo mv bowrain-server /usr/local/bin/

# macOS (Apple Silicon)
curl -LO https://github.com/gokapi/gokapi/releases/latest/download/bowrain-server-darwin-arm64.tar.gz
tar xzf bowrain-server-darwin-arm64.tar.gz
sudo mv bowrain-server /usr/local/bin/
```

### systemd Service

`/etc/systemd/system/bowrain-server.service`:

```ini
[Unit]
Description=Bowrain Server
After=network.target postgresql.service

[Service]
Type=simple
User=bowrain
Group=bowrain
ExecStart=/usr/local/bin/bowrain-server \
  --database postgres://bowrain:password@localhost/bowrain \
  --oidc-issuer https://dex.example.com \
  --oidc-client-id bowrain \
  --oidc-client-secret ${OIDC_CLIENT_SECRET} \
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

## Database Setup

Bowrain Server requires PostgreSQL 14+.

### Create Database

```sql
CREATE DATABASE bowrain;
CREATE USER bowrain WITH PASSWORD 'password';
GRANT ALL PRIVILEGES ON DATABASE bowrain TO bowrain;
```

### Migrations

Migrations run automatically on startup. To run manually:

```bash
bowrain-server migrate --database postgres://...
```

## OIDC Provider Setup

Bowrain Server requires an OIDC provider for authentication. We recommend [Dex](https://dexidp.io/).

### Dex Configuration

`dex-config.yaml`:

```yaml
issuer: https://dex.example.com

storage:
  type: postgres
  config:
    host: localhost
    port: 5432
    database: dex
    user: dex
    password: password
    ssl:
      mode: disable

web:
  http: 0.0.0.0:5556

staticClients:
  - id: bowrain
    redirectURIs:
      - 'https://bowrain.example.com/auth/callback'
    name: 'Bowrain Server'
    secret: your-client-secret

connectors:
  - type: oidc
    id: google
    name: Google
    config:
      issuer: https://accounts.google.com
      clientID: your-google-client-id
      clientSecret: your-google-client-secret
      redirectURI: https://dex.example.com/callback
```

## Health Checks

Verify the server is running:

```bash
curl http://localhost:8080/health
```

Expected response:

```json
{
  "status": "ok",
  "version": "1.0.0",
  "database": "connected",
  "oidc": "configured"
}
```

## Next Steps

- [Configuration](/docs/bowrain-server/configuration)
- [Workspaces](/docs/bowrain-server/workspaces)
- [Connectors](/docs/bowrain-server/connectors)
- [Self-Hosting](/docs/bowrain-server/self-hosting)
