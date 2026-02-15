---
title: status
sidebar_position: 2
---

# kapi status

Show the sync state between local files and Bowrain Server. Displays modified files,
last pull/push timestamps, and server connection status.

## Usage

```bash
kapi status
```

## Examples

```bash
# Show current project status
kapi status

# Example output:
# Project: my-app
# Root: /Users/me/my-project
#
# Last pull: 2026-02-14T10:30:00Z
# Last push: 2026-02-14T09:15:00Z
#
# Modified local files:
#   M src/locales/en/messages.json
#   M src/locales/fr/messages.json
#
# Remote: https://bowrain.example.com
# Project ID: abc123
```

## What It Shows

### Local State

- **Last pull**: When you last fetched from the server
- **Last push**: When you last sent changes to the server
- **Modified files**: Files changed since last sync (based on mtime vs `.kapi/.state.json`)

### Server State

- **Remote URL**: Configured Bowrain Server endpoint
- **Project ID**: Server-side project identifier
- **Remote changes**: (Requires server API — coming soon)

## How It Works

`kapi status` compares:

1. **File modification times** vs. `.kapi/.state.json` timestamps
2. **Local content hashes** vs. last recorded hashes (pending full implementation)
3. **Server state** via `GET /api/v1/.../status` (pending server API)

Currently, only local change detection is implemented. Full remote diffing requires
server API endpoints.

## Sync State File

Status is tracked in `.kapi/.state.json` (auto-gitignored):

```json
{
  "last_pull": "2026-02-14T10:30:00Z",
  "last_push": "2026-02-14T09:15:00Z",
  "files": {
    "src/locales/en/messages.json": {
      "hash": "sha256:abc123...",
      "modified": "2026-02-14T09:15:00Z"
    }
  },
  "remote_items": {
    "ui/strings/messages": {
      "hash": "sha256:def456...",
      "modified": "2026-02-14T09:15:00Z"
    }
  }
}
```

This file is updated by `kapi pull` and `kapi push`.

## Exit Codes

- `0` — Success (no errors)
- `1` — Error (project not found, state file corrupt, etc.)

## Related Commands

- [`kapi diff`](/docs/kapi-cli/commands/diff) — Show detailed line-by-line changes
- [`kapi pull`](/docs/kapi-cli/commands/pull) — Fetch changes from server
- [`kapi push`](/docs/kapi-cli/commands/push) — Send local changes to server

## When to Use

Run `kapi status` to:

- **Check before push** to see what will be uploaded
- **Check after pull** to verify sync succeeded
- **Troubleshoot** sync conflicts or unexpected state

Think of it as `git status` for translation files.
