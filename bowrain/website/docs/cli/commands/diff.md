---
title: diff
sidebar_position: 3
---

# bowrain diff

Show detailed differences between local files and Bowrain Server content. Displays
block-level changes with source and target text diffs.

## Usage

```bash
bowrain diff [paths...] [flags]
```

## Examples

```bash
# Show all differences in the project
bowrain diff

# Show differences for specific files
bowrain diff src/locales/en/messages.json

# Show differences for a directory
bowrain diff src/locales/

# Show only added/removed blocks (no modified)
bowrain diff --status added,removed

# Use unified diff format (like git diff)
bowrain diff --format unified

# Example output:
# diff --bowrain a/ui/strings/messages b/ui/strings/messages
# --- a/src/locales/en/messages.json (remote)
# +++ b/src/locales/en/messages.json (local)
#
# Block: welcome_message
# - Welcome to our app
# + Welcome to our application
#
# Block: logout_button (added)
# + Log Out
```

## Options

| Flag         | Description                                             | Default   |
| ------------ | ------------------------------------------------------- | --------- |
| `--format`   | Output format: `unified`, `json`, `table`               | `unified` |
| `--status`   | Filter by change status: `added`, `removed`, `modified` | (all)     |
| `--context`  | Lines of context in unified diff                        | `3`       |
| `--no-color` | Disable colored output                                  | `false`   |

## Diff Formats

### Unified Format

```diff
diff --bowrain a/ui/strings/buttons b/ui/strings/buttons
--- a/src/locales/en/buttons.json (remote: sha256:abc123)
+++ b/src/locales/en/buttons.json (local: sha256:def456)

Block: save_button
- Save
+ Save Changes

Block: cancel_button (removed)
- Cancel

Block: close_button (added)
+ Close
```

### JSON Format

```json
{
  "files": [
    {
      "local": "src/locales/en/buttons.json",
      "remote": "ui/strings/buttons",
      "blocks": [
        {
          "id": "save_button",
          "status": "modified",
          "remote_source": "Save",
          "local_source": "Save Changes"
        },
        {
          "id": "cancel_button",
          "status": "removed",
          "remote_source": "Cancel"
        },
        {
          "id": "close_button",
          "status": "added",
          "local_source": "Close"
        }
      ]
    }
  ]
}
```

### Table Format

```
FILE: src/locales/en/buttons.json <-> ui/strings/buttons
+----------------+----------+-----------------+-----------------+
| Block ID       | Status   | Remote Source   | Local Source    |
+----------------+----------+-----------------+-----------------+
| save_button    | modified | Save            | Save Changes    |
| cancel_button  | removed  | Cancel          |                 |
| close_button   | added    |                 | Close           |
+----------------+----------+-----------------+-----------------+
```

## How It Works

`bowrain diff` compares block-level content between local files and server state:

1. **Read local files** via FormatRegistry (respecting `.bowrain/config.yaml` mappings)
2. **Fetch remote content** via `POST /api/v1/.../diff` server endpoint
3. **Compute block hashes** using `BlockIdentity` (source text + metadata)
4. **Match blocks** by ID and hash across local/remote
5. **Display differences** in the requested format

## Content Hashing

Block identity is computed as:

```
hash = sha256(block_id + source_text + context_metadata)
```

This enables efficient incremental sync — only changed blocks transfer over the network.

## Exit Codes

- `0` — No differences found
- `1` — Differences exist (exit code mimics `diff` command)
- `2` — Error (project not found, server unavailable, etc.)

## Implementation Status

:::warning Work in Progress

`bowrain diff` is currently a **placeholder**. Full implementation requires:

- Server API endpoint: `POST /api/v1/workspaces/:ws/projects/:id/diff`
- Block-level content comparison
- FormatRegistry integration for reading local files
- Diff formatting and colorization

Current behavior: prints a message indicating the feature is not yet implemented.

:::

## Related Commands

- [`bowrain status`](/cli/commands/status) — Show which files changed (summary)
- [`bowrain pull`](/cli/commands/pull) — Fetch remote changes
- [`bowrain push`](/cli/commands/push) — Send local changes

## When to Use

Use `bowrain diff` to:

- **Review changes** before pushing to the server
- **Understand conflicts** when both local and remote changed
- **Generate reports** for translation review (JSON output)
- **Debug sync issues** when `bowrain status` shows unexpected state
