---
title: pull
sidebar_position: 4
---

# kapi pull

Fetch changes from Bowrain Server. Uses cursor-based incremental sync to
transfer only blocks that changed since the last pull.

When the project is claimed into a workspace, pull also snapshots the
workspace's governed concepts and their relations into the project's bound
termbase (`.kapi/termbase.db`) and records a baseline, so a later
[`kapi push`](/cli/commands/push) can diff local terminology edits against it
and [`kapi verify --terms`](/cli/use-cases/brand-terminology-ci) gates offline
against the same governed vocabulary. See
[Gate brand terminology in CI](/cli/use-cases/brand-terminology-ci).

## Usage

```bash
kapi pull [flags]
```

## Examples

```bash
# Pull all changes from server
kapi pull

# Pull only French translations
kapi pull --locale fr-FR

# Pull multiple locales
kapi pull --locale fr-FR --locale de-DE

# Show what would be pulled without making changes
kapi pull --dry-run

# Force pull from beginning (ignore sync cursor)
kapi pull --force

# Example output:
# Pulled 12 blocks for 2 locales
```

## Options

| Flag        | Description                                      | Default |
| ----------- | ------------------------------------------------ | ------- |
| `--locale`  | Target locales to pull (repeatable)              | all     |
| `--force`   | Pull from beginning, ignoring sync cursor        | `false` |
| `--dry-run` | Show what would be pulled without changing files | `false` |

## What Happens

1. **Read sync cursor** from `.kapi/cache/sync-cache.json`
2. **Query server** via `GET /api/v1/projects/:id/sync/pull?cursor=X&locales=...`
   - Server returns only changes since the cursor (O(changes), not O(total))
   - Paginated: follows `has_more` until all changes are consumed
3. **Update `.kapi/cache/sync-cache.json`** with new cursor

## Locale Scoping

Pull supports locale-scoped queries — fetch translations for specific languages
without downloading everything:

```bash
# Only French
kapi pull --locale fr-FR

# French and German
kapi pull --locale fr-FR --locale de-DE
```

This is efficient because the server's change log is indexed by locale.

## Exit Codes

- `0` — Success (changes pulled or already up to date)
- `1` — Error (server unavailable, auth failed, etc.)

## Related Commands

- [`kapi push`](/cli/commands/push) — Send local changes to server
- [`kapi status`](/cli/commands/status) — Show sync state
- [`kapi diff`](/cli/commands/diff) — Show detailed changes

## When to Use

Pull from Bowrain Server to:

- **Fetch translations** completed by team members
- **Get AI/MT suggestions** generated on the server
- **Sync governed terminology** — the workspace's concepts and relations into
  the local termbase, so [`kapi verify --terms`](/cli/use-cases/brand-terminology-ci)
  can gate offline
- **Update source content** that entered Bowrain through another connector

Source content can originate from a server-side connector — a CMS, a design tool, or a git host — not only from your local files. `kapi pull` brings those upstream changes down, so kapi is the local mirror of content that may have entered Bowrain elsewhere. Think of it as `git pull` for localization content.
