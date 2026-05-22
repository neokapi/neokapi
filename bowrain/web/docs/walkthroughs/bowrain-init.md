---
id: bowrain-init
audience: developer
target_doc: docs/walkthroughs/bowrain-init.mdx
scenes:
  - id: init
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 10
    fixtures: []
    smoke_contract:
      - kapi init --help
---

## Story

`kapi init` scaffolds a `.kapi` project in your repo (the bowrain
equivalent of `git init`): a `<dir-name>.kapi` recipe at the project
root with a `server:` block, plus a sibling `.kapi/` state directory
that holds the block store, sync cache, and TM/termbase.

## Scene 1 — init (terminal)

`kapi init --help` shows the available flags for project bootstrap.

## Closing

For the full project model and config schema, see [Project model](/cli/project-model).
