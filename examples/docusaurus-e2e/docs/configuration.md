---
sidebar_position: 4
---

# Configuration

Acme can be configured through a configuration file, environment variables, or CLI flags.

## Configuration File

Create an `acme.config.json` file in your project root:

```json
{
  "name": "my-app",
  "region": "us-east-1",
  "framework": "nextjs",
  "buildCommand": "npm run build",
  "outputDirectory": "dist"
}
```

## Environment Variables

Sensitive values should be stored as environment variables:

```bash
acme env set DATABASE_URL="postgres://..."
acme env set API_KEY="sk-..."
acme env set SMTP_HOST="smtp.example.com"
```

Environment variables are encrypted at rest and injected at runtime.

## Build Settings

| Option | Description | Default |
|---|---|---|
| `buildCommand` | Command to build your project | Auto-detected |
| `outputDirectory` | Directory containing build output | Auto-detected |
| `installCommand` | Command to install dependencies | `npm install` |
| `rootDirectory` | Project root relative to repository | `.` |

## Custom Domains

Add a custom domain to your project:

```bash
acme domains add docs.acme.example
```

SSL certificates are provisioned automatically via Let's Encrypt.

## Advanced Options

### Headers

Configure custom response headers in `acme.config.json`:

```json
{
  "headers": [
    {
      "source": "/api/(.*)",
      "headers": [
        { "key": "Cache-Control", "value": "no-store" }
      ]
    }
  ]
}
```

### Redirects

Set up URL redirects:

```json
{
  "redirects": [
    {
      "source": "/old-page",
      "destination": "/new-page",
      "permanent": true
    }
  ]
}
```
