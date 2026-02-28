---
title: init
sidebar_position: 1
---

# brain init

Initialize a new Brain project in the current directory. Creates a `.brain/` directory
with configuration, flow definitions, and sync state tracking.

## Usage

```bash
brain init [flags]
```

## Interactive Mode

When run in a terminal without flags, `brain init` presents an interactive wizard.

**If you are already signed in** (via `brain auth login`), the wizard goes straight
to workspace selection, then project name and source locale.

**If you are not signed in**, the wizard offers four paths:

| Option | Description |
|--------|-------------|
| **Sign in to Bowrain** | Authenticate via browser (OAuth device flow), select workspace, create project |
| **Email me a claim link** | Create anonymous project, receive claim email |
| **Continue without signing in** | Create anonymous project, print claim URL |
| **Local only** | No server connection — pure local project |

All interactive paths include a **BCP-47 locale selector** with type-ahead
filtering (press `/` to search) for the source locale.

Authenticated paths include a **workspace selector** where you can choose an
existing workspace or create a new one.

## Examples

```bash
# Interactive mode (recommended)
brain init

# Non-interactive: local project with locales
brain init --name "My App" --source en-US --targets fr-FR,de-DE,ja-JP

# Non-interactive: anonymous project (prints claim URL)
brain init --anonymous --name "My App" --source en

# Non-interactive: anonymous project with email claim
brain init --name "My App" --email alex@example.com

# Non-interactive: connect to existing server project
brain init --server https://bowrain.example.com --project abc123
```

## What Happens

1. Checks that `.brain/` does not already exist (fails fast if it does)
2. Creates `.brain/` directory in the current folder
3. Generates `config.yaml` with project settings
4. Creates `flows/` subdirectory with an example flow definition
5. Adds `.brain/.gitignore` to exclude sync state and tokens
6. Optionally creates a project on the Bowrain Server and configures the connection

After initialization, the directory becomes a Brain project. You can run `brain status`,
`brain flow run`, and other commands from anywhere within the project tree.

## Options

| Flag | Description | Default |
|------|-------------|---------|
| `--name` | Project name | Directory name |
| `--source` | Source locale code (BCP 47) | `en` |
| `--targets` | Comma-separated target locale codes | (none) |
| `--server` | Bowrain Server URL | `BRAIN_SERVER_URL` or config |
| `--project` | Server project ID (connect to existing) | (none) |
| `--anonymous` | Create anonymous project (prints claim URL) | `false` |
| `--email` | Create anonymous project, send claim email | (none) |
| `--json` | Output in JSON format | `false` |
| `--text` | Output in text format (default) | `true` |

## JSON Output

Use `--json` for machine-readable output (useful in CI/CD):

```bash
brain init --anonymous --name "My App" --source en --json
```

```json
{
  "root": "/path/to/my-app",
  "config_dir": "/path/to/my-app/.brain/config.yaml",
  "project_id": "proj_abc123",
  "server": "https://bowrain.example.com",
  "claim_token": "clm_def456",
  "claim_url": "https://bowrain.example.com/claim/clm_def456"
}
```

## Configuration File

`brain init` creates `.brain/config.yaml` with this structure:

```yaml
project:
  name: my-app
  source_locale: en-US
  target_locales:
    - fr-FR
    - de-DE
    - ja-JP

# Optional: connect to Bowrain Server
server:
  url: https://bowrain.example.com
  project_id: abc123
  workspace: my-team

# File mappings: local paths <-> remote items
mappings:
  - local: src/locales/**/*.json
    remote: ui/strings/{path}
    format: json
  - local: content/*.md
    remote: docs/{filename}
    format: markdown

# Hooks: tool chains to run before/after sync
hooks:
  pre-push:
    - qa-check
    - term-enforce
  post-pull:
    - segmentation
```

## Server URL Resolution

The server URL is resolved from (first match wins):

1. `--server` flag
2. `BRAIN_SERVER_URL` environment variable
3. `server.url` in global config (`~/.config/brain/brain.yaml`)
4. Existing auth state (from `brain auth login`)
5. Built-in default (`http://localhost:8080`)

Set it once globally with:

```bash
brain config --global server.url https://bowrain.example.com
```

## Project Discovery

Once initialized, Brain searches for `.brain/` by walking up the directory tree
(like git). You can run commands from any subdirectory:

```bash
cd my-project/src/locales/
brain status  # Finds .brain/ at ../../.brain/
```

## Version Control

**Commit to git:**
- `.brain/config.yaml` — project settings
- `.brain/flows/*.yaml` — flow definitions

**Do NOT commit:**
- `.brain/.sync-cache` — sync cache (auto-gitignored)
- `.brain/.server-token` — auth token (auto-gitignored)

`brain init` automatically adds these to `.gitignore`.

## Next Steps

After initialization:

1. **Edit mappings** in `.brain/config.yaml` to match your file structure
2. **Create flows** in `.brain/flows/` for your translation workflows
3. **Run flows**: `brain flow list` and `brain flow run <flow-name>`
4. **Connect to server**: `brain pull` and `brain push` (if server configured)
