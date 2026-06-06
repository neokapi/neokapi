---
sidebar_position: 3
title: Project Model
---

# Bowrain Project Model

A bowrain project is a `.kapi` project with a `server:` block on its recipe. There is one project model shared with the `kapi` CLI: a single `<dir-name>.kapi` recipe file at the project root and a sibling `.kapi/` state directory.

## Directory Structure

```
my-app/
├── my-app.kapi             # the recipe (committed) — directory-named
├── .kapi/                  # state (gitignored)
│   ├── manifest.yaml       # bookkeeping: block counts, fingerprints
│   ├── tm.db               # authoritative project TM
│   ├── termbase.db         # authoritative project termbase
│   ├── flows/              # optional file-per-flow definitions (committed)
│   │   └── pseudo.yaml
│   └── cache/              # all regenerable caches under one roof
│       ├── blocks.db        # block store (SQLite)
│       ├── sync-cache.json  # kapi push/pull state
│       ├── extractions/
│       └── collections/
└── src/
    └── locales/
        ├── en/
        │   └── messages.json
        └── fr/
            └── messages.json
```

Three ownership zones at the project root:

- **`<dir-name>.kapi`** — hand-edited, committed to git. The recipe is the single source of truth for project configuration.
- **`.kapi/cache/`** — CLI-owned, gitignored. Contains everything that's cheaply regenerable: the block store, the kapi sync cache, extraction intermediates, overlay layers. Safe to delete at any time.
- **`.kapi/tm.db`, `.kapi/termbase.db`, `.kapi/manifest.yaml`** — kapi-owned, authoritative. Gitignored by default; opt in to commit the TM/termbase when cross-clone reproducibility matters.
- **`.kapi/flows/*.yaml`** — optional file-per-flow definitions, hand-edited, committed. Bowrain reads these in addition to inline `flows:` declared on the recipe.

The name pair mirrors git: `<name>.kapi` file plus `.kapi/` folder at the same root.

## Recipe schema

The recipe is a YAML document. Bowrain projects layer a `server:` block (and optional top-level `hooks`, `automations`, `assets`, `brand_voice`) onto the framework's `KapiProject` schema.

```yaml
version: v1
name: My App

defaults:
  source_language: en-US
  target_languages: [fr-FR, de-DE, ja-JP]
  collection: ui/strings
  exclude:
    - "**/*.test.json"
    - "node_modules/**"

content:
  - path: src/locales/**/*.json
    format: json
  - path: content/docs/**/*.md
    format: markdown
    target: i18n/{lang}/docs/{path}/{filename}
  - path: src/es/**/*.json
    format: json
    source_language: es      # per-entry source language override
    collection: spanish-ui   # per-entry collection routing override

plugins:
  okapi-bridge: "^1.47.0"    # map form: name → version constraint

flows:
  pseudo:
    steps:
      - tool: pseudo-translate
        config: { method: extended }

# A server: block depends on the bowrain plugin. init declares the requirement
# so a plain kapi binary (without the plugin) fails fast instead of silently
# ignoring the connection.
requires:
  bowrain: "*"

# Optional bowrain-server connection — presence enables push/pull/sync.
server:
  url: https://bowrain.cloud/my-team/abc123
  stream: $auto              # auto-detect from git branch / CI

# Top-level lifecycle policy:
hooks:
  pre-push: [qa-check]
  post-pull: [update-stats]

automations:
  - name: auto-translate-on-push
    trigger: post-push
    actions:
      - type: wait_translate
        config: { timeout: 5m }
      - type: pull

# Top-level governance / asset policy:
assets:
  enabled: true
  max_size: 100MB

brand_voice:
  profile: company-profile
  channel: marketing
```

### Top-level fields

| Field          | Type           | Description                                                            |
| -------------- | -------------- | ---------------------------------------------------------------------- |
| `version`      | string         | Schema version (currently `v1`)                                        |
| `name`         | string         | Project display name                                                   |
| `defaults`     | object         | Project-wide language and execution defaults                           |
| `content`      | list           | Content collections (see [Content Collections](#content-collections))  |
| `plugins`      | map            | Plugin dependencies as `name: version-constraint` (e.g. map form)      |
| `requires`     | map            | Plugin name → version constraint that gates loading; a `server:` block adds `bowrain` so a plain kapi binary refuses the recipe |
| `flows`        | map            | Inline flow definitions (file-per-flow under `.kapi/flows/` also work) |
| `server`       | object         | Optional bowrain-server connection coordinates                         |
| `hooks`        | map            | Flows that run at lifecycle points (`pre-push`, `post-pull`, ...)      |
| `automations`  | list           | Local automation rules (see [Automations](#automations))               |
| `assets`       | object         | Asset (image/binary) policy                                            |
| `brand_voice`  | object         | Brand voice profile and channel                                        |

### `defaults` block

| Field              | Type   | Description                                              |
| ------------------ | ------ | -------------------------------------------------------- |
| `source_language`  | string | BCP-47 source language (e.g. `en-US`)                    |
| `target_languages` | list   | BCP-47 target languages                                  |
| `collection`       | string | Default collection name for organizing content           |
| `exclude`          | list   | Glob patterns to skip during scanning                    |
| `formats`          | map    | Per-format default presets and config overrides          |

### `server` block

Only the connection coordinates sit under `server:`:

| Field    | Description                                                                  |
| -------- | ---------------------------------------------------------------------------- |
| `url`    | Compound URL: `<server>/<workspace>/<project-id>` or `<server>/projects/<id>` |
| `stream` | Server-side stream to sync against; `$auto` auto-detects from CI / git branch |

Lifecycle (`hooks`, `automations`) and content/governance (`assets`, `brand_voice`) live at the **top level** of the recipe, not under `server:` — they describe project-owned policy, not server identity.

The framework has no built-in notion of a server: `server:` (and `hooks:`, `automations:`, `assets:`, `brand_voice:`) are bowrain **recipe extensions** decoded only when the `kapi-bowrain` plugin is installed (the framework round-trips them verbatim otherwise). So `kapi init` / `kapi init-connect` (and `kapi config server.url …`) declare `requires: { bowrain: "*" }` whenever they write a `server:` block. A plain `kapi` binary without the plugin then refuses the recipe with an actionable "requires the bowrain plugin" error rather than silently ignoring the connection. See [AD-framework-008: Project model — recipe extension mechanism](https://neokapi.github.io/web/neokapi/docs/architecture/008-project-model).

## Content Collections

Each entry under `content:` is a content collection. Bare entries are single-pattern collections; named collections group multiple items together.

You can edit `content:` by hand, or with the core `kapi` commands (no bowrain plugin required — they only touch the local recipe):

```bash
kapi add "src/**/*.json" --format json   # append a content pattern (format auto-detected)
kapi rm  "src/legacy/*.json"             # remove the mapping, or add to the exclude list
kapi ls                                  # list the files the content tracks
kapi ls --stats                          # …with per-file block and word counts
```

`add`/`rm`/`ls` are framework commands; sync state (changed-vs-server) is [`kapi status`](/cli/commands/status).

```yaml
content:
  # Bare entry — single source pattern
  - path: src/locales/**/*.json
    format: json

  # With output path template
  - path: content/docs/**/*.md
    format: markdown
    target: i18n/{lang}/docs/{path}/{filename}

  # Per-entry overrides
  - path: legacy/**/*.properties
    format: java-properties
    source_language: en-GB
    collection: legacy

  # Named collection with nested items
  - name: ui
    items:
      - path: "src/**/*.tsx"
        format:
          name: exec
          config:
            command: "vp kapi-react extract --stream"
      - path: "src/i18n/en/*.json"
        format: json
```

### Content collection fields

| Field              | Type            | Description                                                                |
| ------------------ | --------------- | -------------------------------------------------------------------------- |
| `path`             | string          | Glob pattern for source files (supports `{lang}` placeholder)              |
| `format`           | string / object | File format ID (e.g. `json`, `html`) or object with `name`/`config`/`preset` |
| `target`           | string          | Output path pattern for target files (supports `{lang}` and `{path}`)      |
| `base`             | string          | Path prefix to strip when reporting files                                  |
| `collection`       | string          | Collection routing override for this entry                                 |
| `source_language`  | string          | Source language override for this entry                                    |
| `target_languages` | list            | Target language override for this entry                                    |
| `assets`           | object          | Per-entry asset policy override                                            |
| `asset_max_size`   | string          | Per-entry asset max size override                                          |

### Format object form

When you need to configure a format (apply a preset, pass options, run a subprocess extractor) use the object form:

```yaml
content:
  - path: "src/**/*.tsx"
    format:
      name: exec
      config:
        command: "vp kapi-react extract --stream"

  - path: "docs/**/*.html"
    format:
      name: html
      preset: strict-extraction
```

## Automations

Automations are rules that run automatically at lifecycle points, declared at the top level of the recipe:

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

### Automation fields

| Field     | Description                                                                                |
| --------- | ------------------------------------------------------------------------------------------ |
| `name`    | Rule name                                                                                  |
| `trigger` | Lifecycle point: `pre-push`, `post-push`, `pre-pull`, `post-pull`, `pre-flow`, `post-flow` |
| `actions` | List of actions (`run_flow`, `wait_translate`, `pull`, `push`)                             |
| `enabled` | Optional boolean (defaults to `true`)                                                      |

For lightweight pre/post hooks that simply call existing flows, prefer the top-level `hooks:` map.

## Project Discovery

kapi searches for a `*.kapi` recipe by walking up the directory tree (like git):

```bash
cd my-app/src/locales/fr/
kapi status  # finds my-app.kapi at ../../../my-app.kapi
```

All commands work from any subdirectory within the project. If a directory contains multiple `*.kapi` files, pass `-p <path>` explicitly.

## Version Control

### Commit to git

- `<dir-name>.kapi` — the recipe (single source of truth for configuration)
- `.kapi/flows/*.yaml` — file-per-flow definitions, if you use them

### Do NOT commit

The whole `.kapi/` directory is gitignored by default by `kapi init`:

- `.kapi/cache/` — block store, sync cache, extraction intermediates
- `.kapi/manifest.yaml` — regenerable bookkeeping
- `.kapi/tm.db`, `.kapi/termbase.db` — authoritative but local-only by default; opt in to commit when cross-clone reproducibility matters

## Initialization

Create a new bowrain project:

```bash
cd my-app/
kapi init
```

In interactive mode (default when stdin is a terminal), `kapi init` presents a guided setup wizard where you can sign in, choose a workspace, and configure your project.

For non-interactive usage (e.g. CI/CD), use flags:

```bash
# Local-only project (no server: block written)
kapi init --source en-US --targets fr-FR,de-DE,ja-JP

# Connect to a server (anonymous claim)
kapi init --server https://bowrain.cloud --anonymous

# Apply a framework preset
kapi init --preset nextjs

# Connect to an existing project
kapi init --server https://bowrain.cloud --project abc123
```

### Init flags

| Flag          | Description                                                       |
| ------------- | ----------------------------------------------------------------- |
| `--server`    | Server URL                                                        |
| `--project`   | Connect to an existing project by ID                              |
| `--name`      | Project name (default: current directory name)                    |
| `--source`    | Source locale (default: `en`)                                     |
| `--targets`   | Target locales, comma-separated (e.g. `nb,fr`)                    |
| `--anonymous` | Create a project without signing in                               |
| `--email`     | Create a project and email a link to claim it                     |
| `--preset`    | Apply a framework preset (e.g. `nextjs`, `react-intl`, `angular`) |

`kapi init` writes:

1. `<dir-name>.kapi` recipe at the project root (with a `server:` block when a server was supplied)
2. `.kapi/` state directory
3. `.kapi/flows/pseudo.yaml` — an example flow
4. `.gitignore` updates to exclude `.kapi/`

## Server Connection

The `server.url` field is a compound URL that encodes the server address, workspace, and project ID:

```yaml
server:
  # Workspace project
  url: https://bowrain.cloud/my-team/abc123

  # Direct project (no workspace)
  # url: https://bowrain.cloud/projects/abc123

  stream: $auto
```

Once connected, you can sync with the server:

```bash
kapi push    # Upload local source blocks to server
kapi pull    # Fetch translated blocks from server
kapi status  # Show sync state (pending push/pull)
```

The active server URL is resolved from (first match wins):

1. `server.url` field on the recipe
2. `--server` flag
3. `BOWRAIN_SERVER_URL` environment variable / `server.url` in `~/.config/bowrain/bowrain.yaml`
4. Existing auth state (from `kapi auth login`)
5. Built-in default (`http://localhost:8080`)

## Next Steps

- [Initialize a Project](/cli/commands/init)
- [Custom Flows](/cli/flows/custom-flows)
- [Server Sync](/cli/commands/push)
