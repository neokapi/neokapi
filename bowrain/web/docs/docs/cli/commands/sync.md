---
title: sync
sidebar_position: 7
---

# kapi sync

Push local changes, wait for translations, and pull results — all in one command. This is the primary workflow command of the bowrain plugin, combining `kapi push`, server-side processing, and `kapi pull` into a single invocation.

## Usage

```bash
kapi sync [paths...] [flags]
```

## Examples

```bash
# Full sync: push, wait for translations, pull
kapi sync

# Sync specific paths
kapi sync src/locales/en/

# Push only (don't wait or pull)
kapi sync --no-wait

# Set a custom timeout for the wait phase
kapi sync --timeout 10m

# Sync only specific locales
kapi sync --locale fr-FR --locale de-DE

# Example output:
# ── Push ──────────────────────────────────
# Pushed 47 blocks (scanned 12 files)
#   (sent in 1 batches)
#
# ── Wait ──────────────────────────────────
# Waiting for translations... (timeout: 5m)
#   push abc123: 47 blocks → 3 flows triggered
#   flow ai-translate: completed (42s)
#   flow qa-check: completed (8s)
#   flow term-enforce: completed (3s)
#
# ── Pull ──────────────────────────────────
# Pulled 47 blocks across 4 locales
#   fr-FR: 47 blocks (12 files updated)
#   de-DE: 47 blocks (12 files updated)
#   ja-JP: 47 blocks (12 files updated)
#   es-ES: 47 blocks (12 files updated)
```

## Options

| Flag        | Description                                   | Default     |
| ----------- | --------------------------------------------- | ----------- |
| `--no-wait` | Push only — skip wait and pull phases         | `false`     |
| `--timeout` | Maximum time to wait for translations         | `5m`        |
| `--locale`  | Pull only specific locales (repeatable)       | all locales |
| `--force`   | Push all blocks, ignoring sync cache          | `false`     |
| `--dry-run` | Show what would happen without making changes | `false`     |

## Three Phases

### Phase 1: Push

Identical to [`kapi push`](/cli/commands/push). Sends changed blocks to the server using content-addressed incremental sync. Only modified blocks are transferred.

### Phase 2: Wait

Polls the server for completion of all flows triggered by the push. The server tracks which flows were triggered by a specific push via PushID correlation.

The wait phase ends when:

- All triggered flows complete successfully
- The `--timeout` duration is reached (exit code 2)
- A flow fails (exit code 3)
- No flows were triggered (proceeds immediately)

Use `--no-wait` to skip this phase entirely — useful when you want to push and come back later for the pull.

### Phase 3: Pull

Identical to [`kapi pull`](/cli/commands/pull). Fetches translated blocks from the server and writes them to local files. Use `--locale` to pull only specific target locales.

## PushID Tracking

Each push generates a unique PushID that the server uses to correlate triggered automations:

```
Push (PushID: abc123)
  → Event: connector.push.completed
    → Automation: run ai-translate (linked to abc123)
    → Automation: run qa-check (linked to abc123)
```

The wait phase polls `GET /api/v1/projects/:id/sync/push/:pushId/status` until all linked flows complete or the timeout expires. This ensures that `kapi sync` only pulls translations that result from _this_ push, not from unrelated server activity.

## Exit Codes

- `0` — Success (all three phases completed)
- `1` — Error (push failed, network error, auth error)
- `2` — Timeout (wait phase exceeded `--timeout`)
- `3` — Flow failure (a server-side flow failed during wait)

## Related Commands

- [`kapi push`](/cli/commands/push) — Push phase only
- [`kapi pull`](/cli/commands/pull) — Pull phase only
- [`kapi status`](/cli/commands/status) — Show what will be pushed

## When to Use

Use `kapi sync` when you want the complete round-trip workflow in a single command:

- **CI/CD pipelines** — push source strings, wait for AI translation, pull results, commit
- **Developer workflows** — update source content, get translations back immediately
- **Pre-release checks** — ensure all translations are up to date before shipping

For more granular control, use `kapi push` and `kapi pull` separately.
