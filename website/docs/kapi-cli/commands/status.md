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
# Config:       /Users/me/my-project/.kapi/config.yaml
#
# Local blocks: 142
# Pending push: 3 blocks changed locally
# Last sync:    2026-02-15 10:30:00 UTC

# Example output (no server configured):
# Project root: /Users/me/my-project
# Config:       /Users/me/my-project/.kapi/config.yaml
#
# Sync status requires a Bowrain server connection.
#   Configure server in /Users/me/my-project/.kapi/config.yaml
```

## What It Shows

### Local State

- **Local blocks**: Total number of translatable blocks found in local files
- **Pending push**: Blocks that changed locally since last push (based on content hash diff against `.kapi/.sync-cache`)
- **Pending pull**: Remote changes available on the server since last pull

### Server Connection

- Requires `server.url` and `server.project_id` in `.kapi/config.yaml`
- If not configured, shows a message directing you to configure the server

### Sync Cache

Status is tracked in `.kapi/.sync-cache` (auto-gitignored):

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

This cache can be safely deleted — status will report all blocks as pending push
until the next sync re-establishes the baseline.

## How It Works

`kapi status` performs:

1. **Scan local files** via FormatRegistry (using config mappings)
2. **Extract blocks** and compute content hashes
3. **Diff hashes** against `.kapi/.sync-cache` → count changed blocks (pending push)
4. **Query server** for changes since last sync cursor → count pending pull (if cursor > 0)

## Exit Codes

- `0` — Success (status displayed)
- `1` — Error (project not found, etc.)

## Related Commands

- [`kapi diff`](/docs/kapi-cli/commands/diff) — Show detailed line-by-line changes
- [`kapi pull`](/docs/kapi-cli/commands/pull) — Fetch changes from server
- [`kapi push`](/docs/kapi-cli/commands/push) — Send local changes to server

## When to Use

Run `kapi status` to:

- **Check before push** to see what will be uploaded
- **Check after pull** to verify sync succeeded
- **Troubleshoot** sync issues or unexpected state

Think of it as `git status` for translation files.
