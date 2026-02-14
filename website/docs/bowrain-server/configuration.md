---
sidebar_position: 3
title: Configuration
---

# Server Configuration

Configure Bowrain Server via environment variables, command-line flags, or YAML config file.

## Configuration Sources

Precedence (highest to lowest):

1. Command-line flags
2. Environment variables
3. Config file (`/etc/bowrain/config.yaml`)
4. Defaults

## Environment Variables

### Database

```bash
DATABASE_URL=postgres://user:pass@host:5432/dbname
DATABASE_MAX_CONNECTIONS=20
DATABASE_MAX_IDLE_CONNECTIONS=5
```

### OIDC Authentication

```bash
OIDC_ISSUER=https://dex.example.com
OIDC_CLIENT_ID=bowrain
OIDC_CLIENT_SECRET=your-secret
OIDC_REDIRECT_URL=https://bowrain.example.com/auth/callback
```

### Server

```bash
SERVER_PORT=8080
SERVER_HOST=0.0.0.0
SERVER_BASE_URL=https://bowrain.example.com
SERVER_CORS_ORIGINS=https://app.example.com,https://web.example.com
```

### Storage

```bash
STORAGE_TYPE=s3
STORAGE_S3_BUCKET=bowrain-content
STORAGE_S3_REGION=us-east-1
STORAGE_S3_ENDPOINT=https://s3.amazonaws.com
```

### Logging

```bash
LOG_LEVEL=info  # debug, info, warn, error
LOG_FORMAT=json  # json, text
```

## Command-Line Flags

```bash
bowrain-server \
  --database postgres://user:pass@localhost/bowrain \
  --oidc-issuer https://dex.example.com \
  --oidc-client-id bowrain \
  --oidc-client-secret secret \
  --port 8080 \
  --log-level info
```

## Config File

`/etc/bowrain/config.yaml`:

```yaml
database:
  url: postgres://user:pass@localhost:5432/bowrain
  max_connections: 20
  max_idle_connections: 5

oidc:
  issuer: https://dex.example.com
  client_id: bowrain
  client_secret: your-secret
  redirect_url: https://bowrain.example.com/auth/callback

server:
  port: 8080
  host: 0.0.0.0
  base_url: https://bowrain.example.com
  cors_origins:
    - https://app.example.com
    - https://web.example.com

storage:
  type: s3
  s3:
    bucket: bowrain-content
    region: us-east-1
    endpoint: https://s3.amazonaws.com

logging:
  level: info
  format: json
```

## Next Steps

- [Workspaces](/docs/bowrain-server/workspaces)
- [Connectors](/docs/bowrain-server/connectors)
- [Automation](/docs/bowrain-server/automation)
