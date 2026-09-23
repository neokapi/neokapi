---
title: init
sidebar_position: 1
---

# kapi init

Initialize a new project in the current directory. Creates a `kapi.yaml` recipe at the project root with collections proposed from the files in the tree, and wires the project for the coding agents that work in it. The project's context (terms, content memory, voice profile and the decision record) lives in your workspace; `.kapi/` beside the recipe is a cache for this checkout, kept out of git. With the bowrain plugin installed, `kapi init` can also connect the project to a workspace.

## Usage

```bash
kapi init [flags]
```

## Interactive mode

When run in a terminal without flags, `kapi init` presents an interactive wizard.

**If you are already signed in** (via `kapi auth login`), the wizard goes straight
to workspace selection, then project name and source locale.

**If you are not signed in**, the wizard offers four paths:

| Option                          | Description                                                                    |
| ------------------------------- | ------------------------------------------------------------------------------ |
| **Sign in to Bowrain**          | Authenticate via browser (OAuth device flow), select workspace, create project |
| **Email me a claim link**       | Create anonymous project, receive claim email                                  |
| **Continue without signing in** | Create anonymous project, print claim URL                                      |
| **Local only**                  | No server connection; a pure local project                                     |

All interactive paths include a **BCP-47 locale selector** with type-ahead
filtering (press `/` to search) for the source locale.

Authenticated paths include a **workspace selector** where you can choose an
existing workspace or create a new one.

## Examples

```bash
# Interactive mode (recommended)
kapi init

# Non-interactive: local project with locales
kapi init --name "My App" --source en --targets fr,de,ja

# Non-interactive: a source-only project (no target language yet)
kapi init --name "Docs" --source en

# Non-interactive: anonymous project (prints claim URL)
kapi init --anonymous --name "My App" --source en

# Non-interactive: anonymous project with email claim
kapi init --name "My App" --email alex@example.com

# Non-interactive: connect to an existing server project
kapi init --server https://app.bowrain.cloud --project abc123

# Non-interactive: create the project in a named workspace
kapi init --server https://app.bowrain.cloud --workspace acme
```

## What happens

1. Checks that no `kapi.yaml` recipe already exists (fails fast if one does)
2. Proposes a collection for each kind of content it finds in the tree, unless a preset already named them
3. Writes the `kapi.yaml` recipe at the project root
4. Creates the `.kapi/` cache directory with an ignore rule that keeps all of it out of git
5. Writes the MCP entry that starts `kapi mcp` for this project and the short kapi skill, as `kapi init` does
6. Optionally creates a project on the Bowrain Server and writes the `bowrain:` block to the recipe

After initialization, you can run `kapi status`, `kapi up`, `kapi run <flow>`,
and other commands from anywhere within the project tree.

## Options

| Flag          | Description                                 | Default                        |
| ------------- | ------------------------------------------- | ------------------------------ |
| `--name`      | Project name                                | Directory name                 |
| `--source`    | Source locale code (BCP 47)                 | `en`                           |
| `--targets`   | Comma-separated target locale codes         | (none)                         |
| `--preset`    | Apply a framework preset (for example `nextjs`, `react-intl`, `angular`) | (none) |
| `--server`    | Bowrain Server URL                          | `BOWRAIN_SERVER_URL` or config |
| `--workspace` | Create the project in this workspace (slug) | your only or first workspace   |
| `--project`   | Server project ID (connect to existing)     | (none)                         |
| `--anonymous` | Create anonymous project (prints claim URL) | `false`                        |
| `--email`     | Create anonymous project, send claim email  | (none)                         |
| `--json`      | Output in JSON format                       | `false`                        |
| `--text`      | Output in text format (default)             | `true`                         |

`--server`, `--workspace`, `--project`, `--anonymous` and `--email` are
contributed by the bowrain plugin.

## JSON output

Use `--json` for machine-readable output (useful in CI/CD):

```bash
kapi init --anonymous --name "My App" --source en --json
```

```json
{
  "root": "/path/to/my-app",
  "recipe": "/path/to/my-app/kapi.yaml",
  "state_dir": "/path/to/my-app/.kapi",
  "project_id": "proj_abc123",
  "server": "https://app.bowrain.cloud",
  "claim_token": "clm_def456",
  "claim_url": "https://app.bowrain.cloud/claim/clm_def456"
}
```

## Recipe file

`kapi init` creates `kapi.yaml` at the project root with this structure. The
`name:` field carries the project's human-readable label; it defaults to the
current directory name and is the only place that name lives:

```yaml
version: v1
name: my-app

defaults:
  source_language: en
  target_languages: [fr, de, ja]

collections:
  - path: src/locales/**/*.json
    format: json
  - path: content/*.md
    format: markdown

# Optional: connect to Bowrain Server (compound URL)
bowrain:
  url: https://app.bowrain.cloud/my-team/abc123
  stream: $auto

# Hooks: flows to run at lifecycle points (schema only; see /cli/flows/hooks)
hooks:
  pre-push: [qa, term-check]
  post-pull: [segmentation]
```

See [Project Model](/cli/project-model) for the full recipe schema.

## Server URL resolution

The server URL is resolved from (first match wins):

1. `--server` flag
2. `BOWRAIN_SERVER_URL` environment variable
3. `server.url` in the per-machine [bowrain config](/cli/commands/config)
4. Existing auth state (from `kapi auth login`)
5. The hosted service (`https://app.bowrain.cloud`), used only when init
   contacts a server (sign-in, `--anonymous`, `--email`, `--project`); a plain
   init with nothing configured writes a recipe with no `bowrain:` block

Set it once globally with:

```bash
kapi config set bowrain.server.url https://app.bowrain.cloud
```

## Project discovery

Once initialized, kapi searches for a `kapi.yaml` recipe by walking up the directory tree
(like git). You can run commands from any subdirectory:

```bash
cd my-project/src/locales/
kapi status  # finds kapi.yaml up the tree
```

## Version control

**Commit to git**:

- `kapi.yaml`: the recipe (single source of truth)
- the agent wiring `kapi init` wrote: `.mcp.json` and `.claude/skills/kapi/SKILL.md`, and the other hosts' files where it wrote them

**Do NOT commit** `.kapi/`: it is this checkout's cache (`work/store.db`, the caches, the redaction vault, your personal saved filters), and `kapi init` writes an ignore rule that keeps all of it out.

Auth tokens are never written to the project. They live in the OS keychain (keys `bowrain-auth:<server-url>` and `bowrain-refresh:<server-url>`); non-secret metadata sits in `auth.json` in the bowrain config directory (`~/.config/bowrain` on Linux, `~/Library/Application Support/bowrain` on macOS).

## Next steps

After initialization:

1. **Edit content collections** in `kapi.yaml` to match your file structure
2. **Bind the context**: read the voice profile and terms in with `kapi context import`, which binds the voice under `defaults.voice` by name; declare coordinates under `defaults.coordinates`
3. **Catch the project up**: `kapi up`
4. **Connect to a server**: `kapi init --server …` on an existing project connects it and leaves an already-connected project untouched
