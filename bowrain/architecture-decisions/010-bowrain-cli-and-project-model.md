---
id: 010-bowrain-cli-and-project-model
sidebar_position: 10
title: "AD-010: Bowrain CLI and Project Model"
---

# AD-010: Bowrain CLI and Project Model

## Summary

The `bowrain` binary is the project-sync companion CLI for bowrain-server.
Every bowrain project is a directory containing a `.bowrain/` subdirectory
that holds project identity, content mappings, hooks, automations, and
sync state. Commands discover the project root by walking upward from the
current working directory, in the same style as `git` and `terraform`.

## Context

Developer-facing localization pipelines live inside source repositories.
They need a first-class command surface that:

- tracks which files feed a bowrain-server project,
- pushes source changes and pulls translations reliably,
- composes with git, CI, and Makefile-driven workflows, and
- stores its own configuration alongside the code it describes.

A `.bowrain/` directory at the project root satisfies all four concerns:
configuration ships with the repository, sync cache is per-checkout and
gitignored, and discovery is trivial for any command invoked from a
subdirectory.

## Decision

### The `bowrain` binary

The `bowrain` CLI is built as a separate Go module (`bowrain/cli/`) that
depends on the framework module and the shared CLI base. It shares
command factories (formats, plugins, tools, flows, presets, termbase, TM,
version) with the open-source `kapi` CLI described in
[AD-framework-013](/docs/ad/013-kapi-cli),
and extends them with project-aware behavior.

All `bowrain` commands require a `.bowrain/` project. Commands search
upward from the current directory for a `.bowrain/config.yaml`, and fail
fast if none is found. This mirrors `git` and keeps command semantics
predictable regardless of the working subdirectory.

### Project layout

```
my-app/
├── .bowrain/
│   ├── config.yaml      # Project identity, server URL, content entries, hooks, automations
│   ├── flows/           # Flow definitions (one YAML per flow)
│   │   └── pseudo.yaml
│   ├── .sync-cache      # Local sync state (gitignored, regenerable)
│   └── .gitignore       # Excludes .sync-cache
├── src/
│   └── locales/
│       ├── en-US.json
│       └── fr-FR.json
```

Ownership:

- **`config.yaml`** — hand-edited, committed to git. The single source of
  truth for project configuration.
- **`flows/*.yaml`** — hand-edited, committed. Same flow syntax as
  [AD-framework-006](/docs/ad/006-tool-system).
- **`.sync-cache`** — CLI-owned, gitignored. JSON document tracking the
  last known server state, per-file block hashes, sync cursor, and cached
  server metadata.

### `config.yaml` schema

The config file uses **flat YAML with `version: v1`**. Because
`config.yaml` lives at a well-known path inside `.bowrain/`, the
discriminator is the file path itself — no Kubernetes-style
`kind`/`metadata` envelope is required.

```yaml
version: v1

url: https://bowrain.example.com/my-team/abc123

defaults:
  source_language: en-US
  target_languages: [fr-FR, de-DE, ja-JP]
  collection: ui/strings

content:
  - path: src/locales/**/*.json
    format: json
  - path: content/docs/**/*.md
    format: markdown
    dest: i18n/{lang}/docs/{path}/{filename}
  - path: src/es/**/*.json
    format: json
    language: es          # per-entry source language override
    collection: spanish-ui

plugins:
  - okapi@1.0.0

hooks:
  pre-push: [qa-check, term-enforce]
  post-pull: [update-stats]

flows:
  pseudo:
    target_locale: qps
    method: extended

automations:
  - trigger: post-push
    actions:
      - type: wait_translate
      - type: pull
```

### Compound server URL

The `url` field encodes server, workspace, and project in a single
field. Two forms are supported:

| Form                                                  | Meaning                                            |
| ----------------------------------------------------- | -------------------------------------------------- |
| `https://bowrain.example.com/my-team/abc123`          | Workspace project (`my-team` = workspace slug)     |
| `https://bowrain.example.com/projects/abc123`         | Direct project (anonymous / no workspace)          |

Users paste URLs directly from their browser. Accessor methods
(`ServerURL()`, `ProjectID()`, `Workspace()`, `HasServer()`) parse the
URL on demand. Claim tokens for anonymous projects live in
`.sync-cache`, never in `config.yaml`, so the file is safe to commit.

### Content entries

A content entry connects local files to remote project items:

- **`path`** — glob pattern, relative to the project root. Supports the
  `{lang}` placeholder expanded with the entry's effective source
  language.
- **`format`** — format ID from the FormatRegistry, or `$auto` to detect
  by extension.
- **`dest`** — output pattern for translated files (`{lang}`, `{locale}`,
  `{path}`, `{filename}` substitutions).
- **`base`** — path prefix to strip before reporting to the server.
- **`language`** — per-entry source language override, enabling projects
  with multiple source languages.
- **`target_languages`** — per-entry target override.
- **`collection`** — per-entry collection override.

Collections organize content server-side; each block is pushed with the
resolved collection (entry override, then `defaults.collection`).

When `defaults.target_languages` is empty, the CLI fetches the target
locale list from the server during sync and caches it in `.sync-cache`
under `server_meta`. The resolution order is: CLI flag, then config,
then server cache.

### `.sync-cache`

```json
{
  "server_url": "https://bowrain.example.com",
  "project_id": "abc123",
  "sync_cursor": 4821,
  "last_sync": "2026-02-15T10:30:00Z",
  "claim_token": "clm_abc123",
  "files": {
    "src/locales/en-US.json": {
      "mtime": "2026-02-15T10:25:00Z",
      "size": 4096,
      "blocks": {
        "greeting": "a1b2c3d4...",
        "farewell": "e5f6a7b8..."
      }
    }
  },
  "server_meta": {
    "target_locales": ["fr-FR", "de-DE"],
    "fetched_at": "2026-02-15T10:30:00Z"
  }
}
```

The sync cache is a cache, not authoritative state. Deleting it forces a
full re-scan on the next sync but does not lose work — the server is the
source of truth. Block-level hashes allow push to send only the blocks
that actually changed.

### Commands

| Command                      | Purpose                                                                       |
| ---------------------------- | ----------------------------------------------------------------------------- |
| `bowrain init`               | Create `.bowrain/` (interactive by default; `--anonymous`, `--server` flags)  |
| `bowrain auth login`         | OAuth 2.0 Device Authorization Grant against bowrain-server                   |
| `bowrain auth claim`         | Transfer ownership of an anonymous project to the authenticated user         |
| `bowrain status`             | Show modified files, pending remote changes, conflicts                        |
| `bowrain ls`                 | List tracked files                                                            |
| `bowrain add` / `bowrain rm` | Mutate `content` entries                                                      |
| `bowrain config`             | Get/set config values                                                         |
| `bowrain push`               | Scan locally, diff against `.sync-cache`, upload changed blocks               |
| `bowrain pull`               | Fetch changes since the cached cursor, write translated files                |
| `bowrain sync`               | `push` + `wait_translate` + `pull` in one operation                           |
| `bowrain run <flow>`         | Execute a flow from `.bowrain/flows/`                                         |
| `bowrain serve`              | Start a local dashboard at `http://localhost:3000`                            |
| `bowrain mcp`                | Serve the project-scoped MCP server (stdio)                                   |

Shared commands inherited from the CLI base (`formats`, `tools`,
`flows`, `plugins`, `presets`, `termbase`, `tm`, `version`) operate
within the active project context.

### Hooks

Hooks are local commands that run at well-defined sync boundaries. Six
trigger points are supported, listed under `hooks:` or `automations:`:

- `pre-push`, `post-push`
- `pre-pull`, `post-pull`
- `pre-flow`, `post-flow`

Hooks block the parent operation until complete. They can be skipped
with `--no-hooks` for emergency bypass.

### Action types

Within `automations`, four action types coordinate client+server work:

| Action           | Effect                                                        |
| ---------------- | ------------------------------------------------------------- |
| `run_flow`       | Execute a flow from `.bowrain/flows/`                         |
| `wait_translate` | Poll bowrain-server until translation jobs for the last push complete |
| `pull`           | Fetch translated content                                      |
| `push`           | Send content to the server                                    |

`wait_translate` + `pull` after `push` is the canonical
"push-and-wait-for-translations" pattern and is what `bowrain sync`
runs.

### CLI automation vs. server automation

CLI hooks coordinate the local+server boundary — synchronous, no event
bus, no conditions. Server automation ([AD-013](013-automation-engine.md))
orchestrates asynchronous multi-step workflows inside bowrain-server.
The two complement each other: server automation handles AI translation
and review fan-out; CLI hooks ensure the developer's machine waits for
the right moment before pulling results.

### GitHub Action integration

The `neokapi/bowrain-action` composite action brings bowrain sync into
CI/CD pipelines:

1. `neokapi/setup-bowrain@v1` installs the CLI with platform detection,
   checksum verification, and GitHub Actions caching.
2. `neokapi/bowrain-action@v1` runs `bowrain sync` (push → wait → pull),
   commits translated files, and pushes to the repository.

This closes the loop for fully automated translation: a developer
changes source strings, the action syncs with the server, waits for AI
translation to complete, and commits the results back.

### MCP server

`bowrain mcp` starts a stdio-based Model Context Protocol server scoped
to the active project. It exposes:

| Tool             | Purpose                                                        |
| ---------------- | -------------------------------------------------------------- |
| `project_config` | Return the parsed `config.yaml`                                |
| `project_status` | Same data as `bowrain status`                                  |
| `project_ls`     | List tracked files                                             |
| `project_push`   | Run `bowrain push`                                             |
| `project_pull`   | Run `bowrain pull`                                             |
| `list_flows`     | Enumerate `.bowrain/flows/`                                    |

The MCP server runs under the caller's identity (no elevation) and lets
AI agents operate a project without shell access.

### Relationship to `.kapi` project files

The framework defines `.kapi` project files
([AD-framework-008](/docs/ad/008-project-model))
as portable, single-file YAML recipes for local workflows. `.kapi` and
`.bowrain/` are complementary:

| Concern        | `.kapi` files                                 | `.bowrain/` directories                          |
| -------------- | --------------------------------------------- | ------------------------------------------------ |
| Scope          | Framework-level, local-first                  | Platform-level, server-connected                 |
| Format         | Single YAML file                              | Directory with `config.yaml`, `flows/`, cache    |
| Server sync    | None                                          | Push/pull against bowrain-server                 |
| Use            | Workflow recipe                               | Project sync boundary                            |

A single directory can host both: a `.bowrain/` project for server-side
sync and a `my-app.kapi` recipe for ad-hoc flow composition. They share
the flow steps format and the tool system, so the same flow definition
runs under either driver.

## Consequences

- Every bowrain command operates with full project context; there is no
  ambiguity about which server, workspace, or project a command
  addresses.
- Content is addressed by stable block identity, so push is
  bandwidth-efficient even for large repositories.
- Claim tokens never leak into version control.
- CI/CD pipelines can drive the same CLI used for local development —
  no separate agent or SDK.
- The config and flows are ordinary YAML in a committed directory, so
  code review, branching, and history work the same way as any other
  source file.

## Related

- [AD-011: REST API](011-rest-api.md) — sync endpoints consumed by the CLI
- [AD-013: Automation Engine](013-automation-engine.md) — server-side counterpart to CLI hooks
- [AD-framework-008: Kapi Project Model](/docs/ad/008-project-model) — `.kapi` files
- [AD-framework-013: Kapi CLI](/docs/ad/013-kapi-cli) — shared CLI base
- [Bowrain Sync Protocol](/bowrain/notes/sync-protocol) — push/pull algorithms, cache format
- [CLI Commands Reference](/bowrain/notes/cli-commands-reference) — full command tree
