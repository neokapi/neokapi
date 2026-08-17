---
id: kapi-terminology-qa
audience: developer
target_doc: docs/walkthroughs/kapi-terminology-qa.mdx
scenes:
  - id: terms-qa
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

A terms store makes terminology consistent across your translations and across
locales. `kapi terms` compiles the committed terms bundle and exposes
lookup/search; `kapi exec term-check` flags terminology drift in target files
before they ship — the same check `kapi up` binds after every pass, so a
violating unit cannot lift its locale over the ship gate.

## Scene 1 — terms-qa (terminal)

Inspect the pre-seeded terms's stats, look up a
specific term, search for related ones, then run `kapi pseudo-translate`
followed by `kapi exec term-check ...` to see violations flagged in
the output.

## Closing

In a project, `kapi up` runs this check for you: terminology is a bound
check, and a unit with findings holds its locale out of shipping until it
is fixed.
