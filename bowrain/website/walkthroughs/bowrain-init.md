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
      - bowrain init --help
---

## Story

`bowrain init` scaffolds a `.bowrain/` project in your repo (the bowrain
equivalent of `git init`). Single-command bootstrap of project config,
flow folder, and sync cache.

## Scene 1 — init (terminal)

`bowrain init --help` shows the available flags for project bootstrap.

## Closing

For the full project model and config schema, see [Project model](/cli/project-model).
