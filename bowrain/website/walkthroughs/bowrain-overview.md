---
id: bowrain-overview
audience: developer
target_doc: docs/walkthroughs/bowrain-overview.mdx
scenes:
  - id: overview
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 25
    fixtures: []
    smoke_contract:
      - bowrain --help
      - bowrain init --help
      - bowrain status --help
---

## Story

A 30-second tour of the Bowrain CLI. Top-level commands, the
project-init help text, and the project-status help text — enough to
orient someone before they pick which command to drill into.

## Scene 1 — overview (terminal)

`bowrain --help` shows the top-level commands. `bowrain init --help`
shows the project init flags. `bowrain status --help` shows the
status command (the bowrain equivalent of `git status`).

## Closing

For the full sync workflow, see [`bowrain sync`](/cli/commands/sync) and
the [getting-started walkthrough](/getting-started/walkthrough).
