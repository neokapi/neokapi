---
id: bowrain-auth
audience: developer
target_doc: docs/walkthroughs/bowrain-auth.mdx
scenes:
  - id: auth
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 15
    fixtures: []
    smoke_contract:
      - bowrain auth --help
      - bowrain auth status
---

## Story

`bowrain auth` is how you sign into Bowrain Server from the CLI. Quick
status check; help for the subcommands.

## Scene 1 — auth (terminal)

`bowrain auth --help` lists login/logout/status. `bowrain auth status`
prints the active session (or that you're signed out).

## Closing

For the full device-OAuth flow, see [`bowrain auth login`](/cli/commands/auth).
