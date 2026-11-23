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
ship gate. The gate requires 100% translated and 100% approved content.

`kapi up --plan` previews pending units, exact content-memory matches, remaining
AI work and estimated tokens. `kapi up` takes the French translations from
content memory and holds them for review by a person, so `messages.fr.json` is
not written yet. The command exits 0, and `kapi status --review` lists the units
waiting for approval.

`kapi check --ship` exits 3 while required reviews are missing. `kapi apply`
records each review decision, bound to the translation's content hash. The same
ship check then passes without translating anything again, and a second
`kapi up` writes the three French translations to `messages.fr.json`.

## Scene 1 — up-loop (terminal)

Seed content memory, preview with `kapi up --plan`, and produce translations with
`kapi up`. List pending reviews with `kapi status --review`, show the failing
`kapi check --ship`, record decisions with `kapi apply review.jsonl`, then repeat
the ship check. `kapi status` shows `fr` as shippable, and a closing `kapi up`
creates `messages.fr.json` with the approved French.

## Closing

Share review decisions with the rest of the team with `kapi context sync`.
For a server-connected project, `up` pushes changes, streams progress
and pulls results; reviewers use the shared review queue.
