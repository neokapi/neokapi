---
id: kapi-terminology-qa
audience: developer
target_doc: docs/walkthroughs/kapi-terminology-qa.mdx
scenes:
  - id: termbase-qa
    kind: terminal
    binary: kapi
    duration_budget_seconds: 60
    fixtures:
      - glossary.csv
      - messages_en.json
    smoke_contract:
      - kapi termbase stats
      - kapi termbase lookup password -s en -t fr
      - kapi termbase search encrypt -s en
      - kapi pseudo-translate messages_en.json -o pseudo_fr.json
---

## Story

A glossary makes terminology consistent across your translations and across
locales. `kapi termbase` ingests CSVs and exposes lookup/search;
`kapi exec term-check` flags terminology drift in target files before they
ship — the same check `kapi up` binds after every pass, so a violating unit
cannot lift its locale over the ship gate.

## Scene 1 — termbase-qa (terminal)

Inspect the pre-seeded termbase's stats, look up a
specific term, search for related ones, then run `kapi pseudo-translate`
followed by `kapi exec term-check ...` to see violations flagged in
the output.

## Closing

In a project, `kapi up` runs this check for you: terminology is a bound
check, and a unit with findings counts as drafted, not translated, until it
is fixed.
