---
id: kapi-overview
audience: developer
target_doc: docs/walkthroughs/kapi-overview.mdx
scenes:
  - id: overview
    kind: terminal
    binary: kapi
    duration_budget_seconds: 25
    fixtures: []
    smoke_contract:
      - kapi --help
      - kapi formats
      - kapi tools
---

## Story

A 30-second tour of what kapi is and what it can do. Useful as the very first
thing someone sees on the kapi docs landing page.

## Scene 1 — overview (terminal)

Run `kapi --help` to see the top-level commands, then `kapi formats` to enumerate
the formats it can read and write, then `kapi tools` for the processing tools registry.
The recording paces through each command with enough sleep to read the output.

## Closing

Drill into individual subcommands via the [Kapi CLI overview](/docs/kapi-cli/overview).
