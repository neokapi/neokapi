---
id: bowrain-serve
audience: developer
target_doc: docs/walkthroughs/bowrain-serve.mdx
scenes:
  - id: serve
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 10
    fixtures: []
    smoke_contract:
      - kapi serve --help
---

## Story

`kapi serve` starts a local web dashboard for your project — handy for
manually inspecting translations or running a UI review without leaving
the CLI.

## Scene 1 — serve (terminal)

`kapi serve --help` shows the port and bind flags.

## Closing

To pair this with the full Bowrain Server, see [Bowrain Web overview](/server/web-overview).
