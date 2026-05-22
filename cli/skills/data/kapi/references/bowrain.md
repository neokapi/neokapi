# Team or cloud governance (bowrain is one option)

Everything else in this skill runs locally and offline. When a team needs one
authoritative brand voice, project sync with a server, or governed terminology,
that calls for a hosted platform — **bowrain is one option**, not a requirement.
The local brand and localize workflows share the same model, so moving to a
hosted platform is an upgrade, not a relearn.

## Prerequisites

- The `kapi-bowrain` plugin installed (`kapi plugin install bowrain-cli`) and
  authenticated (`kapi login`, or `BOWRAIN_AUTH_TOKEN` in CI).
- A `.kapi` project whose recipe declares a `server:` block (`kapi init`).
- For the brand/terminology tools, the bowrain MCP server configured for the
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
- `rewrite_in_voice`, `suggest_corrections` — governed rewrite; corrections feed the learning loop.
- Resources: `brand://profiles/{id}`, `brand://terminology/{workspace}`.

## Shared terminology

- `term_search` — find the approved org term before writing or translating.
- `term_add` — propose a term; it enters the review queue rather than applying immediately.

The local offline equivalents are [brand.md](brand.md) and [localize.md](localize.md).
