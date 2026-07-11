---
title: diff
sidebar_position: 3
---

# kapi diff

Show the translation blocks that changed locally relative to the last sync —
like `git diff --stat`, but for content. For each file it reports how many
blocks were added, changed, or removed locally versus the last-synced server
state, and (when a server is configured) the number of remote changes available
to pull.

Without a server, `kapi diff` still works: it compares against the local sync
cache, so it stays useful offline.

## Usage

```bash
kapi diff [paths...] [flags]
```

## Examples

```bash
# Summarize all local changes since the last sync
kapi diff

# Limit to a directory
kapi diff src/locales/

# List the changed block ids/keys with a source preview
kapi diff --verbose
```

Example output:

```text
  src/locales/fr/messages.json   +1 ~2 -1

1 file(s) changed: +1 ~2 -1
Remote: 3 change(s) available to pull

Use --verbose to see changed block ids/keys.
```

With `--verbose`, each changed block is listed under its file with a change
sigil (`+` added, `~` changed, `-` removed) and a source preview.

## Options

| Flag        | Description                                       |
| ----------- | ------------------------------------------------- |
| `--verbose` | List changed block ids/keys with a source preview |

Output format and color come from the shared global flags:

| Flag                            | Description                                |
| ------------------------------- | ------------------------------------------ |
| `--json`                        | Emit machine-readable JSON instead of text |
| `--output-format <json\|text>`  | Select the output format                   |
| `--jq <expr>`                   | Filter JSON output through a jq expression |
| `--color <auto\|always\|never>` | Colorize JSON output                       |

## How it works

1. Resolve the project by walking up from the current directory to the `.kapi`
   recipe (run `kapi init` first if none is found).
2. Read local files via the format registry, respecting the recipe's `content:`
   collections.
3. Compare block-level content against the local sync cache to compute added,
   changed, and removed blocks.
4. When a server is configured, query it for the count of pending remote
   changes.

Block identity is derived from the block's id/key and source content, so only
genuinely changed blocks are reported.

## Exit codes

- `0` — the diff was produced (whether or not changes were found).
- `1` — an error occurred (no project, server unavailable, etc.).

## Related commands

- [`kapi status`](/cli/commands/status) — higher-level sync summary.
- [`kapi pull`](/cli/commands/pull) — fetch remote changes.
- [`kapi push`](/cli/commands/push) — send local changes.
