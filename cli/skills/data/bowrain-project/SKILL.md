---
name: bowrain-project
description: Sync a localization project with a Bowrain server — push source content, pull translations, and check status (like git for content). Use when a project has a .kapi recipe with a server block and the user wants to send content for translation or retrieve completed translations. Triggers on "push content", "pull translations", "sync the project", "bowrain status", "send for translation".
---

# bowrain-project

Drives the project sync lifecycle against a Bowrain server using the `kapi-bowrain` plugin commands. Bowrain is the integration platform: connectors, automation, shared TM/termbase/brand, and a content store.

## When to use

The repo has a `<dir>.kapi` recipe with a `server:` block (a Bowrain project), and the user wants to send content for translation, retrieve translations, or see what's out of sync.

## Prerequisites

- `kapi-bowrain` plugin installed (`kapi plugins install bowrain-cli`).
- Authenticated: `kapi login` (device flow) or `BOWRAIN_AUTH_TOKEN` + `BOWRAIN_SERVER_URL` in CI.
- A project: `kapi init` creates `<dir>.kapi` + a `.kapi/` state dir.

## Commands

```bash
kapi status              # pending push/pull, server connection (like git status)
kapi push [paths...]     # upload local source changes to the server
kapi pull                # download translations and update local files
kapi sync                # push, wait for translations, then pull
kapi ls --stats          # list tracked files with block/word counts
kapi diff                # local vs remote differences
```

Add `--json` for machine-readable output; `--dry-run` to preview; `--locales fr,de` to scope.

## MCP equivalents

When the Bowrain MCP server is configured, the same operations are available as `project_status`, `project_push`, `project_pull`, `project_ls`, `project_config`.

## How to apply

1. `kapi status` to see what's out of sync.
2. `kapi push` after the assistant updates source content.
3. `kapi sync` (or `kapi pull` later) to bring translations back into the repo.
4. Combine with `bowrain-brand-governance` so pushed content is scored against the org brand voice, and with `bowrain-terminology` for shared glossary enforcement.
