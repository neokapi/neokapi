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
