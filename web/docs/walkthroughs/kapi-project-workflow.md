---
id: kapi-project-workflow
audience: developer
target_doc: docs/kapi/get-started/first-project.mdx
scenes:
  - id: project-workflow
    kind: terminal
    binary: kapi
    duration_budget_seconds: 45
    fixtures:
      - messages.json
    smoke_contract:
      - kapi init --name demo --source-locale en --target-locale fr --target-locale qps
      - kapi extract
---

## Story

A `.kapi` project is the recommended day-to-day working model: capture the
languages, the content globs, and the reusable flows once in a committed recipe,
then drive the project without repeating flags. The recipe sits beside a
`.kapi/` state directory — the project store that accumulates block overlays and
translation memory as you work.

The loop is `init` → read the recipe → run a flow → `merge`. A run inside a
project is **process-only**: it commits results to the store rather than writing
files, so passes stay cheap and the store recycles each pass's work. When you
want the localized files on disk, `merge` replays the store onto each source.

## Scene 1 — project-workflow (terminal)

Scaffold a project with `kapi init`, extract the recipe's content into the
project store, run the declared `pseudo` flow process-only, then `merge` the
localized files out — the full project lifecycle, no flags repeated.

## Closing

Commit the `.kapi` recipe and anyone who clones the repository runs the same
flows with the same configuration — the project is the portable contract.
