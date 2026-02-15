---
title: init
sidebar_position: 1
---

# kapi init

Initialize a new Kapi project in the current directory. Creates a `.kapi/` directory
with configuration, flow definitions, and sync state tracking.

## Usage

```bash
kapi init [flags]
```

## Examples

```bash
# Initialize a local project
kapi init

# Initialize with server connection
kapi init --server https://bowrain.example.com --project abc123

# Initialize with custom name
kapi init --name "My Localization Project"

# Initialize with specific source and target locales
kapi init --source en-US --targets fr-FR,de-DE,ja-JP
```

## What Happens

1. Creates `.kapi/` directory in the current folder
2. Generates `config.yaml` with project settings
3. Creates `flows/` subdirectory for YAML flow definitions
4. Adds `.kapi/.sync-cache` to `.gitignore` (sync cache is local)
5. Optionally configures connection to a Bowrain Server instance

After initialization, the directory becomes a Kapi project. You can run `kapi status`,
`kapi flow run`, and other commands from anywhere within the project tree.

## Options

| Flag | Description | Default |
|------|-------------|---------|
| `--name` | Project name | Directory name |
| `--source` | Source locale code | `en-US` |
| `--targets` | Comma-separated target locale codes | `[]` |
| `--server` | Bowrain Server URL | (none) |
| `--project` | Server project ID | (none) |

## Configuration File

`kapi init` creates `.kapi/config.yaml` with this structure:

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

# File mappings: local paths ↔ remote items
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

## Project Discovery

Once initialized, Kapi searches for `.kapi/` by walking up the directory tree
(like git). You can run commands from any subdirectory:

```bash
cd my-project/src/locales/
kapi status  # Finds .kapi/ at ../../.kapi/
```

## Version Control

**Commit to git:**
- `.kapi/config.yaml` — project settings
- `.kapi/flows/*.yaml` — flow definitions

**Do NOT commit:**
- `.kapi/.sync-cache` — sync cache (auto-gitignored)
- `.kapi/.server-token` — auth token (auto-gitignored)

`kapi init` automatically adds these to `.gitignore`.

## When to Use

Initialize a Kapi project when you want to:

- **Define flows** for translation pipelines (AI, MT, TM, QA)
- **Track sync state** with a Bowrain Server instance
- **Map local files** to remote translation items
- **Run hooks** for quality checks and terminology enforcement

## Next Steps

After initialization:

1. **Edit mappings** in `.kapi/config.yaml` to match your file structure
2. **Create flows** in `.kapi/flows/` for your translation workflows
3. **Run flows**: `kapi flow list` and `kapi flow run <flow-name>`
4. **Connect to server**: `kapi pull` and `kapi push` (if server configured)
