---
id: bowrain-getting-started
audience: developer
target_doc: docs/walkthroughs/bowrain-getting-started.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: init
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 25
    fixtures: []
    smoke_contract:
      - kapi init --help
  - id: push
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 20
    seed: from-init
    smoke_contract:
      - kapi push
      - kapi status
  - id: pull
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 20
    seed: from-push
    smoke_contract:
      - kapi pull
      - kapi status
  - id: sync
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 30
    seed: from-pull
    smoke_contract:
      - kapi sync --timeout 2m
---

## Story

The full Bowrain CLI workflow in four scenes: initialize a project,
push source content to the server, pull translated content back, and
finally use `kapi sync` to do all three in one command. This is the
canonical "first 5 minutes" experience — by the end, you have a CI-ready
workflow.

## Scene 1 — init (terminal)

Run `kapi init` in your project directory. The command writes a
`<dir-name>.kapi` recipe at the root and creates a sibling `.kapi/`
state directory. `ls -la` shows the recipe alongside the state dir;
`ls .kapi/` shows the layout (manifest, TM/termbase, `cache/` for
regenerable state).

## Scene 2 — push (terminal)

`kapi push` sends your local source content to Bowrain Server.
`kapi status` shows what got pushed and which keys are pending
translation.

## Scene 3 — pull (terminal)

`kapi pull` brings translated content back from the server into
your local files. `kapi status` confirms the new state.

## Scene 4 — sync (terminal)

`kapi sync` does push + wait-for-translations + pull in a single
command. Use this in CI; the timeout flag bounds how long it'll wait
for AI / human translation to complete before exiting.

## Closing

For the deeper sync protocol and conflict handling, see
[Sync protocol](/notes/sync-protocol). For automation rules that
trigger on push, see the [`bowrain-automation` walkthrough](/walkthroughs/bowrain-automation).
