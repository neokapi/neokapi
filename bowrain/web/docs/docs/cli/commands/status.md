---
title: status
sidebar_position: 2
---

# kapi status

Show the sync state between local files and Bowrain Server. Displays local block
count, pending changes, and last sync timestamp.

## Usage

```bash
kapi status
```

## Examples

```bash
# Show current project status
kapi status

# Example output (connected to server):
# Project root: /Users/me/my-project
# Recipe:       /Users/me/my-project/kapi.yaml
#
# Local blocks: 142
# Pending push: 3 blocks changed locally
# Last sync:    2026-02-15 10:30:00 UTC

# Example output (no server configured):
# Project root: /Users/me/my-project
# Recipe:       /Users/me/my-project/kapi.yaml
#
# Sync status requires a Bowrain server connection.
#   Add a server: block to /Users/me/my-project/kapi.yaml
```

## What It Shows

### Local State

- **Local blocks**: Total number of translatable blocks found in local files
- **Pending push**: Blocks that changed locally since last push (based on content hash diff against `.kapi/cache/sync-cache.json`)
- **Pending pull**: Remote changes available on the server since last pull

### Server Connection

- Requires a `server.url` field on the recipe (the compound URL encodes the project ID)
- If the `server:` block is missing, shows a message directing you to add one

### Terminology and Venue

- **terms**: when the workspace's governed terminology was last snapshotted
  into the local terms store (concept/relation counts), or `never synced` when
  no concept pull has run — refresh it with `kapi pull --concepts`
- **venue**: where `kapi up` would run the kapi loop (`server` or
  `local`) and the recipe's `server.converge` policy

### Sync Cache

Status is tracked in `.kapi/cache/sync-cache.json` (auto-gitignored):

```json
{
  "server_url": "https://app.bowrain.cloud",
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

This cache can be safely deleted — status will report all blocks as pending push
until the next sync re-establishes the baseline.

## How It Works

`kapi status` performs:

1. **Scan local files** via FormatRegistry (using the recipe's `content:` collections)
2. **Extract blocks** and compute content hashes
3. **Diff hashes** against `.kapi/cache/sync-cache.json` -> count changed blocks (pending push)
4. **Query server** for changes since last sync cursor -> count pending pull (if cursor > 0)

## Exit Codes

- `0` — Success (status displayed)
- `1` — Error (project not found, etc.)

## Related Commands

- [`kapi diff`](/cli/commands/diff) — Show detailed line-by-line changes
- [`kapi pull`](/cli/commands/pull) — Fetch changes from server
- [`kapi push`](/cli/commands/push) — Send local changes to server

## When to Use

Run `kapi status` to:

- **Check before push** to see what will be uploaded
- **Check after pull** to verify sync succeeded
- **Troubleshoot** sync issues or unexpected state

Think of it as `git status` for translation files.
