---
id: kapi-terminology-checks
audience: developer
target_doc: docs/walkthroughs/kapi-terminology-checks.mdx
scenes:
  - id: terms-checks
    kind: terminal
    binary: kapi
    duration_budget_seconds: 60
    fixtures:
      - terms.json
      - messages_en.json
    smoke_contract:
      - kapi terms stats
      - kapi terms lookup password -s en -t fr
      - kapi terms search encrypt -s en
      - kapi pseudo-translate messages_en.json -o pseudo_fr.json
---

## Story

A terms store records preferred wording across languages. This walkthrough uses
`kapi terms` to inspect and search its contents, then `kapi exec term-check` to
identify violations in a target file.

## Scene 1: terms-checks (terminal)

Inspect the pre-seeded terms store's statistics, look up a term and search for
related concepts. Run `kapi pseudo-translate`, then `kapi exec term-check ...`
to inspect the resulting findings.

## Closing

In a project, `kapi up` applies the bound terminology check after each pass.
Failing findings prevent the affected content from meeting its ship gate.
