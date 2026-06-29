---
id: kapi-review-and-approve
audience: developer
target_doc: docs/kapi/convergence.mdx
scenes:
  - id: review-and-approve
    kind: terminal
    binary: kapi
    duration_budget_seconds: 35
    fixtures:
      - messages.json
    smoke_contract: []
---

## Story

Convergence keeps three verbs separate: a flow **produces** (it drives each unit
as far up its lifecycle as a machine can, typically to *translated*), a person
**reviews** (advancing what a machine can't decide, *translated → reviewed*), and
a gate **releases**. This walkthrough is the middle verb — the human decision —
and where it lands.

A plain target file records that a translation *exists*, not that someone
*blessed* it. So an approval can't live in the file, and it isn't derivable from
anything. It lands in the project's committed **state store** —
`.kapi-state.json` beside the recipe — bound to the content hash of the exact
translation it approves. Delete the caches and a re-run rebuilds them; the state
store is the one thing it can't, so you commit it with your sources.

The loop is `status` → `status --review` → `apply` → `status`.

## Scene 1 — review-and-approve (terminal)

Start from an already-translated project. `kapi status` shows `fr` translated
100%, reviewed 0% — the machine is done, the human review is pending.
`kapi status --review` is the worklist: every translated unit not yet approved,
addressed by file / id / locale. `kapi apply review.jsonl` records a
`kind:"review"` decision in the state store, content-hash bound. The closing
`kapi status` shows reviewed coverage climb — derived straight back from the
committed decision, so a `{ reviewed: … }` gate now counts it.

## Closing

Commit `.kapi-state.json` and the approval travels with the project — the same
loop a server-backed project runs by pushing its state to a remote instead of
committing a file. The decision is the carrier; the caches are just speed.
