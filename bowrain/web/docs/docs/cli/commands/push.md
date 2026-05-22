---
title: push
sidebar_position: 5
---

# kapi push

Send local file changes to Bowrain Server. Only transfers modified blocks
(incremental sync using content hashing).

## Usage

```bash
kapi push [paths...] [flags]
```

## Examples

```bash
# Push all local changes to server
kapi push

# Push specific files
kapi push src/locales/en/

# Show what would be pushed without uploading
kapi push --dry-run

# Force re-push all blocks (ignoring sync cache)
kapi push --force

# Example output:
# Pushed 47 blocks (scanned 12 files)
#   (sent in 1 batches)
```

## Options

| Flag        | Description                               | Default |
| ----------- | ----------------------------------------- | ------- |
| `--force`   | Push all blocks, ignoring sync cache      | `false` |
| `--dry-run` | Show what would be pushed without sending | `false` |

## What Happens

1. **Read local files** via FormatRegistry (using the recipe's `content:` collections)
2. **Extract blocks** from each file (streaming Parts -> Blocks)
3. **Compute block hashes** using `BlockIdentity` (SHA-256)
4. **Compare with `.kapi/cache/sync-cache.json`** to identify changed blocks
5. **Send changed blocks** to server via `POST /api/v1/projects/:id/sync/push`
   - Batched at 1000 blocks per request
   - Server enforces batch limits and body size (50MB)
6. **Update `.kapi/cache/sync-cache.json`** with new hashes and sync cursor

## Content Hashing

Bowrain CLI uses content-addressed blocks for efficient sync:

```
content_hash = sha256(normalized_source_text)
```

Only blocks with changed hashes are transferred. A project with 10,000 blocks
where 5 changed will only transfer those 5 blocks.

## Sync Cache

Push state is tracked in `.kapi/cache/sync-cache.json` (auto-gitignored):

```json
{
  "server_url": "https://bowrain.example.com",
  "project_id": "abc123",
  "sync_cursor": 4821,
  "last_sync": "2026-02-15T10:30:00Z",
  "files": {
    "_blocks": {
      "blocks": {
        "greeting": "a1b2c3d4...",
        "farewell": "e5f6a7b8..."
      }
    }
  }
}
```

The sync cache can be safely deleted — it will be regenerated on the next push
(which will re-scan and re-push all blocks). The server is the source of truth.

## Exit Codes

- `0` — Success (changes pushed or already up to date)
- `1` — Error (server rejected, network error, etc.)

## Related Commands

- [`kapi pull`](/cli/commands/pull) — Fetch changes from server
- [`kapi status`](/cli/commands/status) — Show what will be pushed
- [`kapi diff`](/cli/commands/diff) — Show detailed changes

## When to Use

Push to Bowrain Server to:

- **Share translations** with your team
- **Trigger workflows** (AI translation, QA, terminology extraction)
- **Backup content** to the server
- **Integrate with CI/CD** pipelines

Think of it as `git push` for localization content.

## Best Practices

1. **Run `kapi status`** before pushing to see what changed
2. **Pull first** if working with a team to avoid conflicts
3. **Use `--dry-run`** when unsure about what will be uploaded
