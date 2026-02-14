---
title: pull
sidebar_position: 4
---

# kapi pull

Fetch changes from Bowrain Server and update local files. Only transfers modified
blocks (incremental sync using content hashing).

## Usage

```bash
kapi pull [paths...] [flags]
```

## Examples

```bash
# Pull all changes from server
kapi pull

# Pull specific files
kapi pull src/locales/fr/

# Show what would be pulled without making changes
kapi pull --dry-run

# Force overwrite local changes
kapi pull --force

# Example output:
# Pulling from: https://bowrain.example.com
# Project: abc123
#
# Fetching changes...
# ✓ src/locales/fr/messages.json: 3 blocks updated
# ✓ src/locales/de/messages.json: 1 block updated
#
# Running post-pull hooks: [segmentation]
# ✓ Hooks completed
#
# Pull complete: 4 blocks updated in 2 files
```

## Options

| Flag | Description | Default |
|------|-------------|---------|
| `--force` | Overwrite local changes without prompting | `false` |
| `--dry-run` | Show what would be pulled without changing files | `false` |

## What Happens

1. **Verify auth** token (`.kapi/.server-token` or environment variable)
2. **Send local state** to server (`POST /api/v1/workspaces/:ws/projects/:id/pull`)
   - Local file hashes and timestamps
   - Server responds with only changed blocks
3. **Check for conflicts** (both local and remote modified since last sync)
   - If conflicts exist and `--force` not set, prompt user
4. **Write blocks** to local files via FormatRegistry
   - Only modified blocks are updated
   - File structure preserved (whitespace, comments, formatting)
5. **Run post-pull hooks** if configured in `.kapi/config.yaml`
6. **Update `.kapi/.state.json`** with new sync state

## Content Hashing

Kapi uses content-addressed blocks for efficient sync:

```
block_hash = sha256(block_id + source_text + context)
```

Only blocks with changed hashes are transferred. This minimizes network traffic
and allows parallel sync of large projects.

## Conflict Resolution

If both local and remote versions changed since last sync:

```
Conflict in src/locales/fr/messages.json:

Block: welcome_message
Local:  Bienvenue dans notre application
Remote: Bienvenue sur notre plateforme

Options:
  1. Keep local (default)
  2. Use remote
  3. Abort pull
Choice:
```

Use `--force` to automatically take remote version for all conflicts.

## Hooks

Post-pull hooks run after files are updated. Configure in `.kapi/config.yaml`:

```yaml
hooks:
  post-pull:
    - segmentation      # Re-segment updated content
    - term-lookup       # Extract terminology
```

Hooks are tool chains that run on pulled content. Skip with `--no-hooks` (future).

## Authentication

`kapi pull` requires a valid server auth token:

1. **Environment variable**: `KAPI_SERVER_TOKEN`
2. **Token file**: `.kapi/.server-token` (auto-gitignored)
3. **Interactive login**: `kapi auth login` (stores token in file)

If no token is found, you'll be prompted to authenticate.

## Implementation Status

:::warning Work in Progress

`kapi pull` is currently a **placeholder**. Full implementation requires:

- Server API endpoint: `POST /api/v1/workspaces/:ws/projects/:id/pull`
- Content hash computation and comparison
- FormatRegistry integration for writing files
- Hook execution framework
- Conflict resolution UI

Current behavior: prints a message indicating the feature is not yet implemented.

:::

## Exit Codes

- `0` — Success (no errors, changes pulled)
- `1` — Conflicts exist and `--force` not set
- `2` — Error (auth failed, server unavailable, etc.)

## Related Commands

- [`kapi push`](/docs/user-guide/cli/push) — Send local changes to server
- [`kapi status`](/docs/user-guide/cli/status) — Show sync state
- [`kapi diff`](/docs/user-guide/cli/diff) — Show detailed changes

## When to Use

Pull from Bowrain Server to:

- **Fetch translations** completed by team members
- **Get AI/MT suggestions** generated on the server
- **Sync terminology** updates from the termbase
- **Update source content** modified in the CMS or design tool

Think of it as `git pull` for localization content.
