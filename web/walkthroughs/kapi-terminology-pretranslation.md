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
      - kapi exec recycle messages_en.json -o step1_tm.json --source-lang en --target-lang fr
---

## Story

Pre-translation is the cheap, deterministic phase that runs before any
machine or human translator sees the content. Leverage existing TM, run
pseudo-translation on the rest, and pre-flag any terminology violations
— all in seconds, no API key required. It is the front half of the default
flow `kapi up` loops over a project, run here one move at a time.

## Scene 1 — termbase-pretranslation (terminal)

Set up language assets (termbase + TM), then run the three-step pipeline:
TM leverage → pseudo-translate the misses → QA check against the termbase.
The output of each step is the input to the next.

## Closing

In a project, `kapi up` runs this sequence for you — TM leverage first,
AI translation where pseudo-translation stands in here, and the bound
checks after each pass. Compose the same steps into a named flow for
`kapi run <flow>` when CI needs exactly one pass.
