---
id: kapi-up-loop
audience: developer
target_doc: docs/kapi/convergence.mdx
scenes:
  - id: up-loop
    kind: terminal
    binary: kapi
    duration_budget_seconds: 60
    fixtures:
      - messages.json
    smoke_contract: []
---

## Story

The kapi loop separates three verbs: a flow **produces** (a machine takes each
unit as far as it can), a person **decides** (translated → reviewed), and a
gate **releases**. This walkthrough shows the whole loop on one small project
whose ship gate demands both translated 100% and reviewed 100% — a bar a
machine alone can never clear.

`kapi up --plan` previews the work before anything is spent: pending units,
exact TM leverage, remaining AI work, a token estimate. `kapi up` produces —
here the TM covers everything — and then parks the locale, because review
needs a person. Parking is the hand-off, not a failure: `up` exits zero, and
`kapi status --review` holds the worklist.

The gate is where trust is earned. `kapi check --ship` fails with exit code 3
while the required review is missing. `kapi apply` records the reviewer's
decisions in the committed state store, bound to each translation's content
hash. The same `check --ship` then passes with nothing re-translated — the
recorded decisions are what cleared it.

## Scene 1 — up-loop (terminal)

Seed the TM, preview with `kapi up --plan`, produce with `kapi up` (fr parks
awaiting review), list the worklist with `kapi status --review`, watch
`kapi check --ship` fail (exit 3), record the decisions with
`kapi apply review.jsonl`, and watch the same gate pass (exit 0). The closing
`kapi status` shows fr shippable.

## Closing

Commit `.kapi-state.json` and the decisions travel with the project. On a
server-connected project the same loop runs on the Bowrain server — `up`
pushes, streams, and pulls; the review queue is the team's instead of a local
worklist. The verbs do not change.
