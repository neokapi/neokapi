---
title: pull
sidebar_position: 4
---

# brain pull

Fetch changes from Bowrain Server. Uses cursor-based incremental sync to
transfer only blocks that changed since the last pull.

## Usage

```bash
brain pull [flags]
```

## Examples

```bash
# Pull all changes from server
brain pull

# Pull only French translations
brain pull --locale fr-FR

# Pull multiple locales
brain pull --locale fr-FR --locale de-DE

# Show what would be pulled without making changes
brain pull --dry-run

# Force pull from beginning (ignore sync cursor)
brain pull --force

# Example output:
# Pulled 12 blocks for 2 locales
```

## Options

| Flag | Description | Default |
|------|-------------|---------|
| `--locale` | Target locales to pull (repeatable) | all |
| `--force` | Pull from beginning, ignoring sync cursor | `false` |
| `--dry-run` | Show what would be pulled without changing files | `false` |

## What Happens

1. **Read sync cursor** from `.bowrain/.sync-cache`
2. **Query server** via `GET /api/v1/projects/:id/sync/pull?cursor=X&locales=...`
   - Server returns only changes since the cursor (O(changes), not O(total))
   - Paginated: follows `has_more` until all changes are consumed
3. **Update `.bowrain/.sync-cache`** with new cursor

## Locale Scoping

Pull supports locale-scoped queries — fetch translations for specific languages
without downloading everything:

```bash
# Only French
brain pull --locale fr-FR

# French and German
brain pull --locale fr-FR --locale de-DE
```

This is efficient because the server's change log is indexed by locale.

## Exit Codes

- `0` — Success (changes pulled or already up to date)
- `1` — Error (server unavailable, auth failed, etc.)

## Related Commands

- [`brain push`](/docs/bowrain-cli/commands/push) — Send local changes to server
- [`brain status`](/docs/bowrain-cli/commands/status) — Show sync state
- [`brain diff`](/docs/bowrain-cli/commands/diff) — Show detailed changes

## When to Use

Pull from Bowrain Server to:

- **Fetch translations** completed by team members
- **Get AI/MT suggestions** generated on the server
- **Sync terminology** updates from the termbase
- **Update source content** modified in the CMS or design tool

Think of it as `git pull` for localization content.
