---
id: kapi-keep-source-on-brand
audience: developer
target_doc: docs/kapi/recipes/keep-source-on-brand.mdx
scenes:
  - id: keep-source-on-brand
    kind: terminal
    binary: kapi
    duration_budget_seconds: 45
    fixtures: []
    smoke_contract:
      - kapi stats product-page.md
---

## Story

kapi's guardrails run on the source language, before any translation exists.
A voice profile — a git-shareable YAML — carries the vocabulary,
forbidden and competitor terms, and tone rules. `kapi check` scores a file
against it and returns one located finding per off-brand block, each with a
stable rule id an AI fix-loop can track. `kapi voice rewrite` fixes the term
findings deterministically, offline; tone and style fixes go through
`kapi apply` with the voice guide as context.

## Scene 1 — keep-source-on-brand (terminal)

Survey `product-page.md` with `kapi stats`, check it against `voice.yaml` with
`kapi check --profile-file` (human table, then `--json`), and substitute the
forbidden and competitor terms with `kapi voice rewrite`. The closing beat
notes that `--max-major 0` turns the score into a gate — the same loop
`kapi check --ship` enforces in a project.

## Closing

The profile is the contract: commit it, and every check — local, CI, or an
assistant's fix-loop — scores against the same rules.
