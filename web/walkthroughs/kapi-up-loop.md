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

This walkthrough follows a project from translation through review to a passing
ship gate. The gate requires 100% translated and 100% reviewed content.

`kapi up --plan` previews pending units, exact content-memory matches, remaining
AI work and estimated tokens. `kapi up` fills the translations from content
memory, then parks the locale awaiting human review. It exits zero;
`kapi status --review` lists the remaining work.

`kapi check --ship` exits 3 while required reviews are missing. `kapi apply`
records each review decision, bound to the translation's content hash. The same
ship check then passes without retranslating the content.

## Scene 1 — up-loop (terminal)

Seed content memory, preview with `kapi up --plan`, and produce translations with
`kapi up`. List pending reviews with `kapi status --review`, show the failing
`kapi check --ship`, record decisions with `kapi apply review.jsonl`, then repeat
the ship check. The closing `kapi status` shows `fr` as shippable.

## Closing

Use the project's context export or snapshot workflow to preserve review
decisions. For a server-connected project, `up` pushes changes, streams progress
and pulls results; reviewers use the shared review queue.
