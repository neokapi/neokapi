---
sidebar_position: 3
title: Project Model
---

# Bowrain Project Model

Bowrain CLI uses a `.bowrain/` directory (like `.git`) to manage localization projects within your repository.

## Directory Structure

```
my-app/
├── .bowrain/
│   ├── config.yaml       # Project configuration
│   ├── flows/            # Custom flow definitions
│   │   └── pseudo.yaml
│   ├── .sync-cache       # Sync cache (gitignored)
│   └── .gitignore        # Auto-generated
├── src/
│   └── locales/
│       ├── en/
│       │   └── messages.json
│       └── fr/
│           └── messages.json
```

## config.yaml

The main configuration file defines the schema version, server connection, language defaults, content entries, and optional hooks:

```yaml
version: v1

# Compound URL: encodes server, workspace, and project ID
url: https://bowrain.example.com/my-team/abc123

# Content stream (default: $auto — auto-detect from git branch / CI)
stream: $auto

defaults:
  source_language: en-US
  target_languages:
    - fr-FR
    - de-DE
    - ja-JP
  collection: ui/strings

# Content entries: which files to track
content:
  - path: src/locales/**/*.json
    format: json
    dest: locales/{lang}/*.json
  - path: content/*.md
    format: markdown

# Hooks: flows that run automatically at lifecycle points
hooks:
  pre-push:
    - qa-check
    - term-enforce
  post-pull:
    - segmentation
```

### All config.yaml Fields

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Schema version (currently `v1`) |
| `url` | string | Compound project URL encoding server, workspace, and project ID |
| `stream` | string | Content stream name (`$auto` for auto-detection from git branch) |
| `defaults` | object | Project-wide language and organization defaults |
| `content` | list | File patterns to track (see [Content Entries](#content-entries)) |
| `plugins` | list | Plugin dependencies (e.g. `["okapi@1.0.0"]`) |
| `registries` | list | Plugin registry overrides |
| `preset` | string | Framework preset name (e.g. `nextjs`, `react-intl`, `angular`) |
| `format_presets` | map | Local format preset definitions |
| `exclude` | list | Glob patterns to skip during scanning |
| `hooks` | map | Flows that run at lifecycle points (`pre-push`, `post-pull`, etc.) |
| `flows` | map | Per-flow settings |
| `automations` | list | Local automation rules (see [Automations](#automations)) |

### Defaults

| Field | Type | Description |
|-------|------|-------------|
| `source_language` | string | BCP-47 source language (e.g. `en-US`) |
| `target_languages` | list | BCP-47 target languages |
| `collection` | string | Default collection name for organizing content |

## Content Entries

Content entries define which files to track. Each entry maps local file patterns to formats and output destinations.

```yaml
content:
  # Track all JSON files under src/locales/
  - path: src/locales/**/*.json
    format: json
    dest: locales/{lang}/*.json

  # Track Markdown docs
  - path: content/*.md
    format: markdown

  # Override source language for a specific entry
  - path: legacy/**/*.properties
    format: java-properties
    language: en-GB
    collection: legacy
```

### Content Entry Fields

| Field | Type | Description |
|-------|------|-------------|
| `path` | string | Glob pattern for source files (supports `{lang}` placeholder) |
| `dest` | string | Output path pattern for target files (supports `{lang}` placeholder) |
| `format` | string | File format ID (e.g. `json`, `html`) or `$auto` for auto-detection |
| `base` | string | Path prefix to strip when reporting files |
| `collection` | string | Collection override for this entry |
| `language` | string | Source language override for this entry |
| `target_languages` | list | Target language override for this entry |
| `overrides` | map | Per-entry format config overrides |

## Automations

Automations define rules that run automatically at lifecycle points:

```yaml
automations:
  - name: qa-before-push
    trigger: pre-push
    actions:
      - type: run_flow
        config:
          flow: qa-check
      - type: wait_translate

  - name: auto-pull-after-push
    trigger: post-push
    actions:
      - type: pull
```

### Automation Fields

| Field | Description |
|-------|-------------|
| `name` | Rule name |
| `trigger` | Lifecycle point: `pre-push`, `post-push`, `pre-pull`, `post-pull`, `pre-flow`, `post-flow` |
| `actions` | List of actions (`run_flow`, `wait_translate`, `pull`, `push`) |
| `enabled` | Optional boolean (defaults to `true`) |

## Project Discovery

Bowrain CLI searches for `.bowrain/` by walking up the directory tree (like git):

```bash
cd my-app/src/locales/fr/
bowrain status  # Finds .bowrain/ at ../../../.bowrain/
```

All commands work from any subdirectory within the project.

## Version Control

### Commit to git

Files to commit:
- `.bowrain/config.yaml` — project settings
- `.bowrain/flows/*.yaml` — flow definitions

### Do NOT commit

Files that should NOT be committed (auto-gitignored):
- `.bowrain/.sync-cache` — local sync cache (block hashes, stream cursors, claim token)

`bowrain init` automatically creates `.bowrain/.gitignore` with these entries.

## Initialization

Create a new Bowrain project:

```bash
cd my-app/
bowrain init
```

In interactive mode (default when stdin is a terminal), `bowrain init` presents a guided setup wizard where you can sign in, choose a workspace, and configure your project.

For non-interactive usage (e.g. CI/CD), use flags:

```bash
# Local-only project
bowrain init --source en-US --targets fr-FR,de-DE,ja-JP

# Connect to a server
bowrain init --server https://bowrain.example.com --anonymous

# Apply a framework preset
bowrain init --preset nextjs

# Connect to an existing project
bowrain init --server https://bowrain.example.com --project abc123
```

### Init Flags

| Flag | Description |
|------|-------------|
| `--server` | Server URL |
| `--project` | Connect to an existing project by ID |
| `--name` | Project name (default: current directory name) |
| `--source` | Source locale (default: `en`) |
| `--targets` | Target locales, comma-separated (e.g. `nb,fr`) |
| `--anonymous` | Create a project without signing in |
| `--email` | Create a project and email a link to claim it |
| `--preset` | Apply a framework preset (e.g. `nextjs`, `react-intl`, `angular`) |

This creates:
1. `.bowrain/` directory
2. `.bowrain/config.yaml` with specified settings
3. `.bowrain/flows/pseudo.yaml` — an example flow
4. `.bowrain/.gitignore` to exclude cache files

## Server Connection

The `url` field in `config.yaml` is a compound URL that encodes the server address, workspace, and project ID:

```yaml
# Workspace project
url: https://bowrain.example.com/my-team/abc123

# Direct project (no workspace)
url: https://bowrain.example.com/projects/abc123
```

Once connected, you can sync with the server:

```bash
bowrain push    # Upload local source blocks to server
bowrain pull    # Fetch translated blocks from server
bowrain status  # Show sync state (pending push/pull)
```

The server URL is resolved from (first match wins):
1. `url` field in `.bowrain/config.yaml`
2. `--server` flag
3. `BOWRAIN_SERVER_URL` environment variable / `server.url` in `~/.config/bowrain/bowrain.yaml`
4. Existing auth state (from `bowrain auth login`)
5. Built-in default (`http://localhost:8080`)

## Next Steps

- [Initialize a Project](/bowrain/cli/commands/init)
- [Custom Flows](/bowrain/cli/flows/custom-flows)
- [Server Sync](/bowrain/cli/commands/push)
