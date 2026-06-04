---
id: kapi-klz-workspace
audience: developer
target_doc: docs/kapi/recipes/resumable-workspace.mdx
scenes:
  - id: klz-workspace
    kind: terminal
    binary: kapi
    duration_budget_seconds: 40
    fixtures:
      - messages.json
    smoke_contract:
      - kapi extract messages.json -o work.klz --target-lang qps
      - kapi pseudo-translate work.klz
      - kapi info work.klz
      - kapi pack work.klz
      - kapi merge work.klz -o out/
---

## Story

A `.klz` is a single-file, serverless localization workspace — the portable
twin of a `.kapi` project's working state. You build it up and emit from it with
three pipeline-stage verbs: `extract` (ingest), a transform run in place (here
`pseudo-translate`), and `merge` (emit). The `.klz` itself is a stable bundle
written only when you `pack`; in between, work accumulates in a fast shadow
cache, and `info` tells you whether it is dirty.

## Scene 1 — klz-workspace (terminal)

Extract a JSON catalog into a `.klz`, pseudo-translate it in place, check its
dirty state, pack the working cache into the file, and merge out the localized
result — the full lifecycle on one portable file, no project required.

## Closing

Hand the packed `.klz` to another machine and the first command rebuilds its
cache from the file — pick up exactly where you left off.
