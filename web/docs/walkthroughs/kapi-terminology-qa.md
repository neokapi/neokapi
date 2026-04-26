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
      - kapi termbase import fixtures/glossary.csv --name product-terms --format csv -s en -t fr --header
      - kapi termbase stats --name product-terms
      - kapi termbase lookup password --name product-terms -s en -t fr
---

## Story

A glossary makes terminology consistent across your translations and across
locales. `kapi termbase` ingests CSVs, exposes lookup/search, and feeds
into `kapi qa-check` to flag terminology drift in target files before
they ship.

## Scene 1 — termbase-qa (terminal)

Import a CSV glossary into a named termbase, inspect stats, look up a
specific term, search for related ones, then run `kapi pseudo-translate`
followed by `kapi qa-check --termbase ...` to see violations flagged in
the output.

## Closing

Hook the same termbase into a flow with `kapi run` so every push runs the
QA check automatically.
