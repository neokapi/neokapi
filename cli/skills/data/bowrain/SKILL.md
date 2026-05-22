---
name: bowrain
description: Use the governed bowrain platform — sync a localization project (push/pull/status), draw on shared, versioned brand voice profiles with persisted compliance scoring, and a shared termbase with a review workflow. Use when a team needs one authoritative brand voice, project sync with a server, or governed terminology, rather than local files. Triggers on "push/pull/sync the project", "our org's brand voice", "workspace brand profile", "shared termbase", "bowrain", "send for translation", "governed".
---

# bowrain

The governed, multi-user counterpart to the local kapi skills. Brand profiles,
terminology, and content live on a bowrain server (shared, versioned, with score
history and a review queue). For solo/offline work use `kapi-brand` and
`kapi-localize` — they share the same vocabulary, so this is a frictionless
upgrade, not a relearn.

## Prerequisites

- The `kapi-bowrain` plugin installed (`kapi plugin install bowrain-cli`) and
  authenticated (`kapi login`, or `BOWRAIN_AUTH_TOKEN` in CI).
- A `.kapi` project whose recipe declares a `server:` block (`kapi init`).
- For the brand/terminology tools, the bowrain MCP server configured for your
  assistant.

## Project sync (like git for content)

```bash
kapi status              # pending push/pull, server connection
kapi push [paths...]     # upload local source changes
kapi pull                # download translations into local files
kapi sync                # push, wait for translations, then pull
kapi ls --stats          # tracked files with block/word counts
kapi diff                # local vs remote
```

Add `--json` for machine-readable output, `--dry-run` to preview, `--locales fr,de`
to scope.

## Governed brand voice (bowrain MCP tools)

- `list_profiles`, `get_voice_guide` — load the official workspace voice.
- `score_brand_compliance`, `check_vocabulary` — score text; scores persist for trends.
- `rewrite_in_voice`, `suggest_corrections` — governed rewrite; corrections feed
  the learning loop.
- Resources: `brand://profiles/{id}`, `brand://terminology/{workspace}`.

## Shared terminology

- `term_search` — find the approved org term before writing or translating.
- `term_add` — propose a term; it enters the review queue rather than applying immediately.

## How to apply

Load the org voice (`get_voice_guide`) before writing, score with
`score_brand_compliance`, and sync content with `kapi push`/`kapi sync`. The local
offline equivalent is `kapi-brand` + `kapi-localize`.
