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

A `.kpz` is a portable content workspace. `extract` reads content into it, a
transform such as `pseudo-translate` changes that content, and `merge` writes the
result. Changes accumulate in a working cache until `pack` writes them to the
bundle. `info` reports whether the cache contains unpacked changes.

## Scene 1 — kpz-workspace (terminal)

Extract a JSON catalog into a `.kpz`, pseudo-translate it, inspect its dirty
state, pack the changes, and merge the result to a target file.

## Closing

Transfer the packed `.kpz` to another machine to resume work. The first command
rebuilds its working cache from the bundle.
