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

This walkthrough records human approval of an existing translation. The project
starts with translated content awaiting review. An approval is bound to the
exact translation's content hash, so a later edit requires another review.

The sequence is `status` → `status --review` → `apply` → `status`.

## Scene 1 — review-and-approve (terminal)

Start from an already-translated project. `kapi status` shows `fr` translated
100% and established 0%. `kapi status --review` lists the units awaiting approval,
addressed by file, id and locale. `kapi apply review.jsonl` records a
`kind:"review"` decision. The closing `kapi status` shows the resulting increase
in established coverage, which counts toward an `{ established: … }` gate.

## Closing

Review decisions are authored records, separate from derived caches. Use the
project's context export or snapshot workflow to preserve them.

`kapi check --ship` exits 3 while required reviews are missing and passes once
the gate's requirements are met. The kapi-up-loop walkthrough shows this
transition without retranslating the approved content.
