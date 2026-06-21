---
id: kapi-terminology-pretranslation
audience: developer
target_doc: docs/walkthroughs/kapi-terminology-pretranslation.mdx
scenes:
  - id: termbase-pretranslation
    kind: terminal
    binary: kapi
    duration_budget_seconds: 75
    fixtures:
      - glossary.csv
      - project.tmx
      - messages_en.json
    smoke_contract:
      - kapi tm import project.tmx -s en -t fr
      - kapi recycle messages_en.json -o step1_tm.json --source-lang en --target-lang fr
---

## Story

Pre-translation is the cheap, deterministic phase that runs before any
machine or human translator sees the content. Leverage existing TM, run
pseudo-translation on the rest, and pre-flag any terminology violations
— all in seconds, no API key required.

## Scene 1 — termbase-pretranslation (terminal)

Set up language assets (termbase + TM), then run the three-step pipeline:
TM leverage → pseudo-translate the misses → QA check against the termbase.
The output of each step is the input to the next.

## Closing

The same three steps fit into a `kapi run` flow so this becomes one
command in CI.
