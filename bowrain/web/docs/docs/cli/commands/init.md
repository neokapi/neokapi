---
title: init
sidebar_position: 1
---

# kapi init

Initialize a new Bowrain project in the current directory. Creates a `kapi.yaml` recipe at the project root and a sibling `.kapi/` state directory for the block store, sync cache, TM, and termbase.

## Usage

```bash
kapi init [flags]
```

## Interactive Mode

When run in a terminal without flags, `kapi init` presents an interactive wizard.

**If you are already signed in** (via `kapi auth login`), the wizard goes straight
to workspace selection, then project name and source locale.

**If you are not signed in**, the wizard offers four paths:

| Option                          | Description                                                                    |
| ------------------------------- | ------------------------------------------------------------------------------ |
| **Sign in to Bowrain**          | Authenticate via browser (OAuth device flow), select workspace, create project |
| **Email me a claim link**       | Create anonymous project, receive claim email                                  |
| **Continue without signing in** | Create anonymous project, print claim URL                                      |
| **Local only**                  | No server connection — pure local project                                      |

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

# Non-interactive: anonymous project (prints claim URL)
kapi init --anonymous --name "My App" --source en

# Non-interactive: anonymous project with email claim
kapi init --name "My App" --email alex@example.com

# Non-interactive: connect to existing server project
kapi init --server https://app.bowrain.cloud --project abc123
```

## What Happens

1. Checks that no `kapi.yaml` recipe and no `.kapi/` state directory already exist (fails fast if they do)
2. Writes the `kapi.yaml` recipe at the project root
3. Creates `.kapi/` state directory with `flows/`, `manifest.yaml`, and an empty `cache/`
4. Adds the example `pseudo` flow at `.kapi/flows/pseudo.yaml`
5. Adds a `.gitignore` entry to exclude `.kapi/` from version control
6. Optionally creates a project on the Bowrain Server and writes the `server:` block to the recipe

After initialization, the directory becomes a Bowrain project. You can run `kapi status`,
`kapi translate`, `kapi run <flow>`, and other commands from anywhere within the project tree.

## Options

| Flag          | Description                                 | Default                        |
| ------------- | ------------------------------------------- | ------------------------------ |
| `--name`      | Project name                                | Directory name                 |
| `--source`    | Source locale code (BCP 47)                 | `en`                           |
| `--targets`   | Comma-separated target locale codes         | (none)                         |
| `--server`    | Bowrain Server URL                          | `BOWRAIN_SERVER_URL` or config |
| `--project`   | Server project ID (connect to existing)     | (none)                         |
| `--anonymous` | Create anonymous project (prints claim URL) | `false`                        |
| `--email`     | Create anonymous project, send claim email  | (none)                         |
| `--json`      | Output in JSON format                       | `false`                        |
| `--text`      | Output in text format (default)             | `true`                         |

## JSON Output

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

## Recipe File

`kapi init` creates `kapi.yaml` at the project root with this structure. The
`name:` field carries the project's human-readable label; it defaults to the
current directory name and is the only place that name lives:

```yaml
version: v1
name: my-app

defaults:
  source_language: en
  target_languages: [fr, de, ja]

content:
  - path: src/locales/**/*.json
    format: json
  - path: content/*.md
    format: markdown

# Optional: connect to Bowrain Server (compound URL)
server:
  url: https://app.bowrain.cloud/my-team/abc123
  stream: $auto

# Hooks: flows to run at lifecycle points (schema only — not yet executed; see /cli/flows/hooks)
hooks:
  pre-push: [qa, term-enforce]
  post-pull: [segmentation]
```

See [Project Model](/cli/project-model) for the full recipe schema.

## Server URL Resolution

The server URL is resolved from (first match wins):

1. `--server` flag
2. `BOWRAIN_SERVER_URL` environment variable
3. `server.url` in global config (`~/.config/bowrain/bowrain.yaml`)
4. Existing auth state (from `kapi auth login`)
5. The hosted service (`https://app.bowrain.cloud`) — used only when init
   contacts a server (sign-in, `--anonymous`, `--email`, `--project`); a plain
   init with nothing configured writes a recipe with no `server:` block

Set it once globally with:

```bash
kapi config set bowrain.server.url https://app.bowrain.cloud
```

## Project Discovery

Once initialized, kapi searches for a `kapi.yaml` recipe by walking up the directory tree
(like git). You can run commands from any subdirectory:

```bash
cd my-project/src/locales/
kapi status  # finds kapi.yaml up the tree
```

## Version Control

**Commit to git:**

- `kapi.yaml` — the recipe (single source of truth)
- `.kapi/flows/*.yaml` — flow definitions you author

**Do NOT commit:**

- `.kapi/cache/` — block store, sync cache, extraction intermediates (auto-gitignored)
- `.kapi/manifest.yaml` — regenerable bookkeeping
- `.kapi/tm.db`, `.kapi/termbase.db` — local-only by default; opt in to commit when cross-clone reproducibility matters

Auth tokens are never written to the project. They live in the OS keychain (keys `bowrain-auth:<server-url>` and `bowrain-refresh:<server-url>`); non-secret metadata sits at `~/.config/bowrain/auth.json`.

`kapi init` automatically adds `.kapi/` to `.gitignore`.

## Next Steps

After initialization:

1. **Edit content collections** in `kapi.yaml` to match your file structure
2. **Create flows** in `.kapi/flows/` for your translation workflows
3. **Run tools and flows**: `kapi tools`, `kapi flows`, `kapi translate`, `kapi run <flow-name>`
4. **Connect to server**: `kapi pull` and `kapi push` (if `server:` block is set)
