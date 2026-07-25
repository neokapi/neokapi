---
id: kapi-kpz-workspace
audience: developer
target_doc: docs/kapi/recipes/resumable-workspace.mdx
scenes:
  - id: kpz-workspace
    kind: terminal
    binary: kapi
    duration_budget_seconds: 40
    fixtures:
      - messages.json
    smoke_contract:
      - kapi extract messages.json -o work.kpz --target-lang qps
      - kapi pseudo-translate work.kpz
      - kapi info work.kpz
      - kapi pack work.kpz
      - kapi merge work.kpz -o out/
---

## Story

A `.kpz` is a single-file, serverless localization workspace — the portable
twin of a `.kapi` project's working state. You build it up and emit from it with
three pipeline-stage verbs: `extract` (ingest), a transform run in place (here
`pseudo-translate`), and `merge` (emit). The `.kpz` itself is a stable bundle
written only when you `pack`; in between, work accumulates in a fast shadow
cache, and `info` tells you whether it is dirty.

## Scene 1 — kpz-workspace (terminal)

Extract a JSON catalog into a `.kpz`, pseudo-translate it in place, check its
dirty state, pack the working cache into the file, and merge out the localized
result — the full lifecycle on one portable file, no project required.

## Closing

Hand the packed `.kpz` to another machine and the first command rebuilds its
cache from the file — pick up exactly where you left off.
