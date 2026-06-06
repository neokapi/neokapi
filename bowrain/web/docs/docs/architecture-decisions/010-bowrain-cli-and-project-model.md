---
id: 010-bowrain-cli-and-project-model
sidebar_position: 10
title: "AD-010: Bowrain CLI and Project Model"
---

# AD-010: Bowrain CLI and Project Model

## Summary

Bowrain ships as a **manifest-driven kapi plugin**, not as part of the
`kapi` binary. The bowrain commands (`push`, `pull`, `sync`, `status`,
`init`, `auth`, …), the source connector that implements push/pull against
bowrain-server, the bowrain MCP tools, and the recipe-schema decoders that
validate `server:`/`hooks:`/`automations:` all live in `bowrain/plugin/`
and compile into a standalone plugin binary, `kapi-bowrain` (built from
`bowrain/cli/cmd/kapi-bowrain/`).

The `kapi` binary contains **zero bowrain code**. It discovers the
installed plugin at runtime by reading its `manifest.json`, then dispatches
`kapi push`, `kapi pull`, … to `kapi-bowrain` as a subprocess. The
discovery and dispatch mechanics are the framework's unified plugin model —
see [AD-framework-007: Plugin System](https://neokapi.github.io/web/neokapi/docs/architecture/007-plugin-system).
The legacy standalone `bowrain` binary, the build-time blank-import of
`bowrain/plugin` into `kapi`, and the `-tags pure` / `kapi-pure` split are
all retired: there is one user-facing CLI, `kapi`, and bowrain is something
you install into it (`kapi plugins install bowrain`, or
`brew install neokapi/tap/bowrain-cli`).

A bowrain project is just a kapi project with a `server:` block on its
recipe — same `*.kapi` recipe file, same `.kapi/` state directory, same
discovery rules as the framework. Commands walk upward from the current
working directory looking for a `*.kapi` recipe, in the same style as
`git` and `terraform`.

This is the only project model. Bowrain does not maintain a parallel
project schema; the recipe loader, validator, layout discovery, sync
cache, and content iteration all live in the framework's `core/project`
package. Bowrain registers its extension schema, commands, MCP tools,
and source connector via process-global registries at `init()` time inside
the `kapi-bowrain` binary.

## Context

Developer-facing localization pipelines live inside source repositories.
They need a first-class command surface that:

- tracks which files feed a bowrain-server project,
- pushes source changes and pulls translations reliably,
- composes with git, CI, and Makefile-driven workflows, and
- stores its own configuration alongside the code it describes.

The framework's `.kapi` recipe model
([AD-framework-008: Kapi Project Model](https://neokapi.github.io/web/neokapi/docs/architecture/008-project-model))
already provides a portable, gitignore-aware project layout with stateless
recipe + sibling state directory. Bowrain extends it with a `server:`
block and a few top-level lifecycle/governance fields.

## Decision

### The bowrain plugin

Bowrain's behavior lives in Go packages under `bowrain/plugin/`:

```
bowrain/plugin/
├── plugin.go              ← anchor; blank-imports the three sub-packages
├── schema/                ← recipe schema decoders (no CLI deps)
│   ├── server.go, hooks.go, assets.go, brand_voice.go, stream.go
│   └── extension.go       ← init() RegisterExtensionGroup("bowrain", ...)
├── commands/              ← bowrain CLI commands (push, pull, init, auth, ...)
├── connector/             ← BowrainSourceConnector implementation
└── mcp/                   ← bowrain MCP tools
```

Each sub-package's `init()` registers its features with the framework /
shared CLI registries:

- `schema/extension.go` calls `coreproj.RegisterExtensionGroup("bowrain", ...)`
- `commands/*.go` each call `cli.RegisterCommandFactory(...)`
- `commands/register.go` calls `cli.RegisterAppInitializer(...)` to install
  `app.FallbackRunE` (project flow resolution) and `app.ExtraFlows`
  (project flow listing)
- `mcp/tools.go` calls `cli.RegisterMCPToolFactory(...)`

These registries are the framework's in-process plugin mechanism. The only
binary that links them is `kapi-bowrain`, whose `main.go` blank-imports the
anchor:

```go
import _ "github.com/neokapi/neokapi/bowrain/plugin"
```

The `kapi` binary does **not** import `bowrain/plugin`. It loads bowrain at
runtime as a separate process (next section), so the registries above
populate inside `kapi-bowrain`, never inside `kapi`.

### Distribution and discovery

Two binaries are produced from this source tree:

| Binary | Built from | Role |
|---|---|---|
| `kapi` | `kapi/cmd/kapi` | The single user-facing CLI. Framework-only; no bowrain code. |
| `kapi-bowrain` | `bowrain/cli/cmd/kapi-bowrain` | The bowrain plugin binary. Carries a `manifest.json` declaring its commands, MCP tools, schema extensions, and source connector. |

`kapi-bowrain` is installed into a plugin directory rather than placed on
`PATH`. `kapi` discovers it at runtime by scanning, in order,
`$KAPI_PLUGINS_DIR`, `$XDG_DATA_HOME/kapi/plugins` (default
`~/.local/share/kapi/plugins`), and the system plugin roots, looking for
`<dir>/bowrain/{manifest.json, kapi-bowrain}`. Two paths install it there:

```bash
kapi plugins install bowrain          # via the plugin registry
brew install neokapi/tap/bowrain-cli  # Homebrew (depends on the kapi formula)
```

Discovery and dispatch are not bowrain-specific — they are the framework's
unified plugin model, shared with okapi-bridge and any other plugin. See
[AD-framework-007: Plugin System](https://neokapi.github.io/web/neokapi/docs/architecture/007-plugin-system)
for the manifest schema, discovery precedence, and the A/B/C transport
modes referenced below.

The Wails desktop apps (kapi-desktop, bowrain-desktop) blank-import
`bowrain/plugin/schema` directly so they validate bowrain recipes when
opened; push/pull is invoked by spawning `kapi-bowrain`.

### Available commands

With the plugin installed, `kapi` exposes:

- Framework commands (built into `kapi`): `run`, `extract`, `merge`,
  `add`, `rm`, `ls`, `flows`, `tools`, `formats`, `plugins`, `registry`,
  `presets`, `termbase`, `tm`, `credentials`, `mcp`, `version`. The
  local project-content commands `add`/`rm`/`ls` are core — they edit and
  list the `.kapi` recipe's content, which is local configuration, not a
  server concern (a server-connected project just declares `requires: bowrain`).
- Bowrain commands (contributed by the `kapi-bowrain` manifest): `init`,
  `push`, `pull`, `sync`, `status`, `diff`, `config`,
  `auth`, `stream`, `serve`, `ui`, `workspace`. Sync state — including which
  tracked files changed vs the server — is `kapi status` (the home of the
  former `ls --dirty`).

Each bowrain capability is dispatched according to its manifest entry:

- **Commands** run one-shot — `kapi push` forks `kapi-bowrain command push
  …`, inheriting stdin/stdout/stderr and propagating the exit code (Mode A).
- **MCP tools** (`project_status`, `project_pull`, `project_push`) are
  served by a `kapi-bowrain mcp-server` session that the shared `kapi mcp`
  command launches and proxies (Mode B).
- **The source connector** (`bowrain-source`) runs inside a long-lived
  `kapi-bowrain daemon` reached over a Unix-domain socket; `kapi` routes
  push/pull/status through it so recipe parsing and sync state stay in the
  plugin process (Mode C).

Without the plugin, `kapi` is framework-only — a bowrain recipe still
loads, but `kapi push`/`kapi pull` are unavailable and a recipe that
declares `requires: [bowrain]` fails validation.

All bowrain server-sync commands require a `.kapi` recipe with a `server:`
block. Discovery is identical to kapi: walk upward looking for a `*.kapi`
recipe; the recipe must declare `server:` for push/pull/status to be
meaningful.

### Project layout

```
my-app/
├── my-app.kapi         # the recipe (committed) — directory-named
├── .kapi/              # state (gitignored)
│   ├── manifest.yaml
│   ├── tm.db           # authoritative project TM
│   ├── termbase.db     # authoritative project termbase
│   ├── flows/          # optional file-per-flow definitions
│   │   └── pseudo.yaml
│   └── cache/          # all regenerable caches under one roof
│       ├── blocks.db        # block store
│       ├── sync-cache.json  # kapi push/pull state (only with server: block)
│       ├── extractions/
│       └── collections/
└── src/
    └── locales/
        ├── en-US.json
        └── fr-FR.json
```

Ownership:

- **`{name}.kapi`** — hand-edited, committed to git. The single source of
  truth for project configuration.
- **`.kapi/cache/`** — CLI-owned, gitignored. Contains everything that's
  cheaply regenerable: the block store, the kapi sync cache, extraction
  intermediates, overlay layers.
- **`.kapi/tm.db`, `.kapi/termbase.db`, `.kapi/manifest.yaml`** — kapi-owned,
  authoritative. Gitignored by default; opt in to commit the TM/termbase
  when cross-clone reproducibility matters.
- **`.kapi/flows/*.yaml`** — optional file-per-flow definitions, hand-edited,
  committed. Bowrain reads these in addition to inline `flows:` declared
  on the recipe.

### Recipe with server connection

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
    source_language: es     # per-entry source language override
    collection: spanish-ui  # per-entry collection routing override

plugins:
  okapi-bridge: "^1.47.0"   # map form, not list

flows:
  pseudo:
    steps:
      - tool: pseudo-translate
        config: { method: extended }

# Optional bowrain-server connection — presence enables push/pull/sync.
server:
  url: https://bowrain.example.com/my-team/abc123
  stream: $auto             # auto-detect from git branch / CI

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

The recipe schema is owned by the framework. Field shape, validation,
loading, saving, and walk-up discovery all live in `core/project`. Bowrain
imports them; it does not redefine them.

### `server:` block

Only the connection coordinates sit under `server:`:

- **`url`** — compound URL encoding `<server>/<workspace>/<project-id>` for
  workspace projects, or `<server>/projects/<project-id>` for direct
  (anonymous) projects.
- **`stream`** — which server-side content stream to sync against. The
  sentinel `$auto` (default when unset) auto-detects the stream name from
  CI environment variables (GitHub Actions, GitLab CI, CircleCI, Azure
  DevOps, Jenkins, Travis CI, Buildkite) or from the local git branch.
  `master` normalizes to `main`. The chain `--stream` flag →
  `BOWRAIN_STREAM` env var → recipe field → auto-detect → `main` decides
  the active stream per command.

Lifecycle (`hooks`, `automations`) and content/governance (`assets`,
`brand_voice`) are top-level on the recipe — they describe project-owned
policy, not server identity.

### Sync cache

`.kapi/cache/sync-cache.json` tracks the last known server state for
incremental sync. Per-file block hashes, per-stream cursors, claim tokens
for anonymous projects, and cached server metadata. Stored as JSON, always
gitignored (it lives under `.kapi/cache/` which the user typically
gitignores wholesale).

Deleting the file is safe — push and pull repopulate it from server state.
The cache is regenerable.

### Auth

Bowrain access tokens live in the OS keychain — the same store kapi uses
for LLM provider keys (`keyringService = "kapi"`). Tokens are keyed by
server URL (`bowrain-auth:<server-url>` for the access token,
`bowrain-refresh:<server-url>` for the refresh token), so multiple
bowrain instances coexist without collision.

Non-secret metadata (server URL, user info, expiry) lives at
`~/.config/bowrain/auth.json`. The file format is JSON; the access /
refresh token fields are intentionally absent on disk — they're loaded
from the keychain by `bowrain.auth.LoadAuth()`.

For CI, the `BOWRAIN_AUTH_TOKEN` environment variable bypasses both the
file and the keychain (paired with `BOWRAIN_SERVER_URL`).

Anonymous projects use a claim token stored in `.kapi/cache/sync-cache.json`
(gitignored) rather than the keychain — the token is project-scoped
rather than user-scoped.

### Workflow

```bash
kapi init                # create my-app.kapi + .kapi/, populate server: block
kapi auth login          # OAuth login → tokens to keychain, metadata to ~/.config/bowrain/auth.json
kapi status              # show what's pending push / pull
kapi push [--dry-run]    # scan local files, diff against cache, upload changed blocks
kapi pull [--locale fr]  # fetch translations from server, write to local files
kapi sync                # push → wait_translate → pull (orchestrated)
kapi ls                  # list tracked files (--stats adds block/word counts)
kapi add <path>          # append a content entry to the recipe
kapi rm <path>           # remove or exclude a content entry
kapi mcp                 # stdio MCP server exposing project tools
```

`kapi init` writes a `<dir-name>.kapi` recipe by default. The recipe
lands at the project root; the sibling `.kapi/` state dir is created
empty (caches populate as commands run). No `.bowrain/` directory is
ever created.

## Consequences

**Positive:**

- One project model, one schema, one loader. Bowrain consumes the
  framework directly — no parallel `Config` struct, no duplicate
  validation, no separate walk-up routine.
- Users with both kapi and bowrain workflows on a project see one
  recipe, one state directory, and one keychain prompt for credentials.
- `.kapi/cache/` consolidates all regenerable state under a single
  predictable path that can be safely deleted.

**Negative:**

- Bowrain features that aren't yet expressed on the framework recipe
  (plugin registries, per-flow config maps, `LocalFormatPreset.Description`)
  are deferred or dropped. Future work re-adds them as framework recipe
  fields when needed.
- The unified discovery means a `*.kapi` recipe with a `server:` block
  must be unambiguous in its directory. Multiple recipes at the same
  level require an explicit `-p` flag — same rule as kapi.

## Related

- [AD-framework-007: Plugin System](https://neokapi.github.io/web/neokapi/docs/architecture/007-plugin-system) — the manifest discovery and A/B/C dispatch model that loads `kapi-bowrain`
- [AD-framework-008: Kapi Project Model](https://neokapi.github.io/web/neokapi/docs/architecture/008-project-model) — the recipe schema this AD layers `server:` onto
- [AD-009: Sync Protocol](009-sync-protocol) — the on-the-wire contract used by push/pull
- [AD-011: REST API](011-rest-api) — the bowrain-server endpoints consumed by the source connector
- [AD-013: Automation Engine](013-automation-engine) — server-side automation paired with local `hooks` / `automations`
