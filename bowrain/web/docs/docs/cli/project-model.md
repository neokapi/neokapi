---
sidebar_position: 3
title: Project Model
---

# Bowrain Project Model

A bowrain project is a `.kapi` project with a `bowrain:` block on its recipe. There is one project model shared with the `kapi` CLI: a single `kapi.yaml` recipe file at the project root and a sibling `.kapi/` directory.

## Directory Structure

```
my-app/
├── kapi.yaml                   # the recipe (committed) — fixed, conventional filename
├── .kapi/                      # committed — the project's context
│   ├── manifest.yaml           # bookkeeping: block counts, fingerprints
│   ├── filters.json            # shared reader/writer configuration
│   ├── filters.local.json      # personal overrides (gitignored)
│   ├── flows/                  # optional file-per-flow definitions
│   │   └── pseudo.yaml
│   ├── terms.json              # terms (bound by defaults.terms_source)
│   ├── voice.yaml              # the voice profile
│   ├── memory/                 # content-memory bundles
│   │   └── memory.json         # the primary (bound by defaults.memory_source)
│   ├── profiles/               # per-profile governance overrides
│   │   └── bowrain/
│   │       └── voice.yaml
│   ├── state/                  # the unit-state record, one shard per document
│   │   └── src-locales-en-messages.jsonl
│   └── work/                   # gitignored — everything derived
│       ├── store.db            # the local index over everything committed
│       ├── vault/              # withheld redaction originals (local-only)
│       └── cache/              # free to delete, always
│           ├── sync-cache.json  # kapi push/pull state
│           ├── extractions/
│           └── collections/
└── src/
    └── locales/
        ├── en/
        │   └── messages.json
        └── fr/
            └── messages.json
```

Ownership zones at the project root:

- **`kapi.yaml`** — hand-edited, committed to git. The recipe is the single source of truth for project configuration. Its fixed, conventional filename means every editor and code host (GitHub, GitLab) applies YAML syntax highlighting to diffs and previews with no configuration.
- **`.kapi/`** — the committed context graph, flat: `terms.json`, `memory/` and `voice.yaml`, with per-profile overrides under `profiles/<name>/`, reviewed through `git diff` like any other source file. `.kapi/` is committed in full; only `.kapi/work/` is gitignored.
- **`.kapi/state/*.jsonl`** — the unit-state record, committed. `kapi commit` publishes staged unit state into it.
- **`.kapi/work/store.db`** — kapi-owned, gitignored. One SQLite file holding every subsystem's tables — block cache, terms store, content memory, the working set of unit state staged since the last `kapi commit`, and the project's context graph. It is an index over the committed sources above and rebuilds from them.
- **`.kapi/work/cache/`** — CLI-owned, gitignored. Everything cheaply regenerable: the kapi sync cache, extraction intermediates, overlay layers. Safe to delete at any time.
- **`.kapi/flows/*.yaml`** — optional file-per-flow definitions, hand-edited, committed. Bowrain reads these in addition to inline `flows:` declared on the recipe.

Local and server converge in shape. Bowrain answers graph questions over one Postgres spanning workspaces, projects and streams; a project answers the same query shapes over `store.db` with those dimensions fixed to one value — so which blocks use a given term, by collection and coordinate, is answerable with no server.

The pairing keeps the git-like shape of a committed config file beside a tool-owned directory: `kapi.yaml` alongside `.kapi/` at the same root. It is the same model git itself uses — `.git` is the tool's directory, and its index lives inside it.

## Recipe schema

The recipe is a YAML document. Bowrain projects layer a `bowrain:` block (and optional top-level `hooks`, `automations`, `assets`, `brand_voice`) onto the framework's `KapiProject` schema.

```yaml
version: v1
name: My App

defaults:
  source_language: en
  target_languages: [fr, de, ja]
  collection: ui/strings
  exclude:
    - "**/*.test.json"
    - "node_modules/**"

collections:
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

# The bowrain: block depends on the bowrain plugin. init declares the
# requirement so a plain kapi binary (without the plugin) fails fast instead of
# silently ignoring the connection.
requires:
  bowrain: "*"

# Optional bowrain-server connection — presence enables push/pull and makes the
# server the default venue for `kapi up`.
bowrain:
  url: https://app.bowrain.cloud/my-team/abc123
  stream: $auto              # auto-detect from git branch / CI
  converge: on-push          # on-push (default) | manual

# Top-level lifecycle policy:
hooks:
  pre-push: [qa]
  post-pull: [update-stats]

automations:
  - name: notify-on-parked
    trigger: run-parked
    actions:
      - type: slack
        config: { channel: "#translation" }

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
| `collections`  | list           | Content collections (see [Content Collections](#content-collections))  |
| `profiles`     | map            | Governance bound per product, keyed by profile name (see [Profiles and channels](#profiles-and-channels)) |
| `plugins`      | map            | Plugin dependencies as `name: version-constraint` (e.g. map form)      |
| `requires`     | map            | Plugin name → version constraint that gates loading; a `bowrain:` block adds `bowrain` so a plain kapi binary refuses the recipe |
| `flows`        | map            | Inline flow definitions (file-per-flow under `.kapi/flows/` also work) |
| `bowrain`      | object         | Optional bowrain-server connection coordinates                         |
| `hooks`        | map            | Flows that run at lifecycle points (`pre-push`, `post-pull`, ...)      |
| `automations`  | list           | Local automation rules (see [Automations](#automations))               |
| `assets`       | object         | Asset (image/binary) policy                                            |
| `brand_voice`  | object         | Brand voice profile and channel                                        |

### `defaults` block

| Field              | Type   | Description                                              |
| ------------------ | ------ | -------------------------------------------------------- |
| `source_language`  | string | BCP-47 source language (e.g. `en`)                       |
| `target_languages` | list   | BCP-47 target languages                                  |
| `collection`       | string | Default collection name for organizing content           |
| `exclude`          | list   | Glob patterns to skip during scanning                    |
| `formats`          | map    | Per-format default presets and config overrides          |
| `terms_source`     | string | Path to the committed terms source (e.g. `.kapi/terms.json`) |
| `memory_source`    | string | Path to the committed content memory source (e.g. `.kapi/memory/memory.json`) |

### `bowrain` block

Only the connection coordinates sit under `bowrain:`:

| Field      | Description                                                                  |
| ---------- | ---------------------------------------------------------------------------- |
| `url`      | Compound URL: `<server>/<workspace>/<project-id>` or `<server>/projects/<id>` |
| `stream`   | Server-side stream to sync against; `$auto` auto-detects from CI / git branch |
| `converge` | Server-side convergence policy: `on-push` (default) or `manual`              |

Lifecycle (`hooks`, `automations`) and content/governance (`assets`, `brand_voice`) live at the **top level** of the recipe, not under `bowrain:` — they describe project-owned policy, not server identity.

The framework has no built-in notion of a server: `bowrain:` (and `hooks:`, `automations:`, `assets:`, `brand_voice:`) are bowrain **recipe extensions** decoded only when the `kapi-bowrain` plugin is installed (the framework round-trips them verbatim otherwise). The key is the platform's own name because the block *is* the platform: kapi finds it through a venue flag on the plugin's schema registration and reads only `url:` and `converge:`, never spelling the key out. So `kapi init` / `kapi init-connect` (and `kapi config server.url …`) declare `requires: { bowrain: "*" }` whenever they write a `bowrain:` block. A plain `kapi` binary without the plugin then refuses the recipe with an actionable "requires the bowrain plugin" error rather than silently ignoring the connection. See [AD-framework-008: Project model — recipe extension mechanism](https://neokapi.github.io/contribute/architecture/008-project-model).

## Content Collections

Each entry under `collections:` is a content collection. Bare entries are single-pattern collections; named collections group multiple items together.

You can edit `collections:` by hand, or with the core `kapi` commands (no bowrain plugin required — they only touch the local recipe):

```bash
kapi add "src/**/*.json"                 # append a content pattern (format auto-detected)
kapi rm  "src/legacy/*.json"             # remove the mapping, or add to the exclude list
kapi ls                                  # list the files the content tracks
kapi add "src/**/*.md" --format markdown # …pass --format only to override detection
kapi ls --stats                          # …with per-file block and word counts
```

`add`/`rm`/`ls` are framework commands; sync state (changed-vs-server) is [`kapi status`](/cli/commands/status).

```yaml
collections:
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

  # Named collection — its items live under content:, relative to base:
  - name: ui
    channel: app
    base: src
    content:
      - path: "**/*.tsx"
        format:
          name: exec
          config:
            command: "vp neokapi-i18n extract --stream"
      - path: "i18n/en/*.json"
        format: json
```

### Collection fields

| Field              | Type            | Description                                                                |
| ------------------ | --------------- | -------------------------------------------------------------------------- |
| `name`             | string          | Collection name; required to bind a `channel:`                             |
| `base`             | string          | The directory this collection lives in — every `path`, `target` and item `base` below is written relative to it |
| `channel`          | string          | The point in the context space this content sits at: `profile/channel`, or a bare `channel` (see [Profiles and channels](#profiles-and-channels)) |
| `content`          | list            | The collection's content items                                             |
| `collection`       | string          | Collection routing override                                                |
| `source_language`  | string          | Source language override                                                   |
| `target_languages` | list            | Target language override                                                   |

### Content item fields

| Field              | Type            | Description                                                                |
| ------------------ | --------------- | -------------------------------------------------------------------------- |
| `path`             | string          | Glob pattern for source files (supports `{lang}` placeholder)              |
| `format`           | string / object | File format ID (e.g. `json`, `html`) or object with `name`/`config`/`preset` |
| `target`           | string          | Output path pattern for target files (supports `{lang}` and `{path}`)      |
| `base`             | string          | Directory a matched file's path is made relative to for target-token expansion; defaults to the glob's fixed prefix |
| `collection`       | string          | Collection routing override for this entry                                 |
| `source_language`  | string          | Source language override for this entry                                    |
| `target_languages` | list            | Target language override for this entry                                    |
| `assets`           | object          | Per-entry asset policy override                                            |
| `asset_max_size`   | string          | Per-entry asset max size override                                          |

A bare entry carries `path`, `format` and `target` directly on the collection and has no `content:` list.

## Profiles and channels

Content is written for a point in a two-axis space: the **product** it belongs to and the **channel** it ships on. The space is structural — a key under `profiles:` is a product, the channels that profile declares are the channels that product ships on, and a named collection names its point with one `channel:` reference.

```yaml
profiles:
  acme:
    channels: [app, docs]
    voice: .kapi/voice.yaml
  acme-labs:
    channels: [app]
    voice: .kapi/profiles/acme-labs/voice.yaml
    terms: .kapi/profiles/acme-labs/terms.json

collections:
  - name: acme-docs
    channel: docs        # only acme declares it — the bare form resolves
    content:
      - path: docs/**/*.md
  - name: labs-app
    channel: acme-labs/app   # both declare `app` — qualify it
    content:
      - path: labs/src/i18n/**/*.kbf.json
```

The profile name is also the directory under `.kapi/profiles/<name>/` holding what that profile overrides. Profile names and channels are slugs — lowercase letters, digits and hyphens — stable identifiers that cross the sync wire as the content's product and channel coordinates, never vocabulary. A bare `channel:` two profiles declare is a load error naming both qualified spellings; a collection binding no channel is governed by `defaults.voice` and the project's own terms.

A profile's `terms:` is the one binding that does not cross to the server, which governs terminology from the workspace vocabulary instead. A connected project that binds terms per profile warns on every run that the binding applies to local runs only.

### Format object form

When you need to configure a format (apply a preset, pass options, run a subprocess extractor) use the object form:

```yaml
collections:
  - path: "src/**/*.tsx"
    format:
      name: exec
      config:
        command: "vp neokapi-i18n extract --stream"

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
          flow: qa
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

kapi searches for a `kapi.yaml` recipe by walking up the directory tree (like git):

```bash
cd my-app/src/locales/fr/
kapi status  # finds kapi.yaml at ../../../kapi.yaml
```

All commands work from any subdirectory within the project. A directory holds at most one `kapi.yaml`, so discovery is unambiguous; an explicit `-p <path>` still overrides it.

## Version Control

### Commit to git

- `kapi.yaml` — the recipe (single source of truth for configuration)
- `.kapi/terms.json`, `.kapi/memory/memory.json`, `.kapi/voice.yaml` — the context sources the recipe binds
- `.kapi/state/*.jsonl` — the unit-state record
- `.kapi/flows/*.yaml` — file-per-flow definitions, if you use them
- `.kapi/manifest.yaml`, `.kapi/filters.json` — bookkeeping and shared reader configuration

### Do NOT commit

`kapi init` writes a two-line ignore rule, with no negation:

```gitignore
/.kapi/work/
/.kapi/filters.local.json
```

- `.kapi/work/` — everything derived: `store.db`, the caches, and the redaction vault
- `.kapi/filters.local.json` — your personal reader overrides

Deleting `.kapi/work/cache/` costs nothing. Deleting `.kapi/work/` costs two things: the review unit state staged since the last `kapi commit`, which live only in `store.db` — run `kapi commit` before you remove it — and, if the project uses redaction, the withheld originals in `.kapi/work/vault/`, which are local-only by design and rebuild from nothing.

## Initialization

Create a new bowrain project:

```bash
cd my-app/
kapi init
```

In interactive mode (default when stdin is a terminal), `kapi init` presents a guided setup wizard where you can sign in, choose a workspace, and configure your project.

For non-interactive usage (e.g. CI/CD), use flags:

```bash
# Local-only project (no bowrain: block written)
kapi init --source en --targets fr,de,ja

# Connect to a server (anonymous claim)
kapi init --server https://app.bowrain.cloud --anonymous

# Apply a framework preset
kapi init --preset nextjs

# Connect to an existing project
kapi init --server https://app.bowrain.cloud --project abc123
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

1. `kapi.yaml` recipe at the project root (with a `bowrain:` block when a server was supplied)
2. `.kapi/` directory
3. `.kapi/flows/pseudo.yaml` — an example flow
4. a two-line ignore rule — `/.kapi/work/` and `/.kapi/filters.local.json`

## Server Connection

The block's `url` field is a compound URL that encodes the server address, workspace, and project ID:

```yaml
bowrain:
  # Workspace project
  url: https://app.bowrain.cloud/my-team/abc123

  # Direct project (no workspace)
  # url: https://app.bowrain.cloud/projects/abc123

  stream: $auto
```

Once connected, you can sync with the server:

```bash
kapi push    # Upload local source blocks to server
kapi pull    # Fetch translated blocks from server
kapi status  # Show sync state (pending push/pull)
```

The active server URL is resolved from (first match wins):

1. `url:` under the recipe's `bowrain:` block
2. `--server` flag
3. `BOWRAIN_SERVER_URL` environment variable / `server.url` in the per-machine [bowrain config](/cli/commands/config)
4. Existing auth state (from `kapi auth login`)
5. The hosted service (`https://app.bowrain.cloud`) — commands that contact a
   server fall back to it; self-hosted deployments configure one of the above

## Next Steps

- [Initialize a Project](/cli/commands/init)
- [Custom Flows](/cli/flows/custom-flows)
- [Server Sync](/cli/commands/push)
