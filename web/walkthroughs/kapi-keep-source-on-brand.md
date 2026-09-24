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

kapi checks source-language content before translation. A voice profile defines
vocabulary, forbidden and competitor terms, and tone rules. `kapi check` reports
violations with their locations and stable rule identifiers. `kapi voice rewrite`
applies deterministic term replacements offline. An assistant can use the voice
guide to draft tone and style edits, then write them with `kapi apply`.

## Scene 1 — keep-source-on-brand (terminal)

Survey `product-page.md` with `kapi stats`, check it against `voice.yaml` with
`kapi check --profile-file` (human table, then `--json`), and substitute the
forbidden and competitor terms with `kapi voice rewrite`. The closing beat
notes that `--max-major 0` turns the score into a gate — the same loop
`kapi check --ship` enforces in a project.

## Closing

Share the profile so local checks, CI and assistants can apply the same rules.
