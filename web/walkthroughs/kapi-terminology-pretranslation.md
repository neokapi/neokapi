---
id: kapi-terminology-pretranslation
audience: developer
target_doc: docs/walkthroughs/kapi-terminology-pretranslation.mdx
scenes:
  - id: terms-pretranslation
    kind: terminal
    binary: kapi
    duration_budget_seconds: 75
    fixtures:
      - terms.json
      - project.memory.json
      - messages_en.json
    smoke_contract:
      - kapi memory import project.memory.json
      - kapi exec recycle messages_en.json -o step1_tm.json --source-lang en --target-lang fr
---

## Story

Pre-translation is the cheap, deterministic phase that runs before any
machine or human translator sees the content. Leverage existing content memory, run
pseudo-translation on the rest, and pre-flag any terminology violations
— all in seconds, no API key required. It is the front half of the default
flow `kapi up` loops over a project, run here one move at a time.

## Scene 1 — terms-pretranslation (terminal)

Set up language assets (terms + content memory), then run the three-step pipeline:
content-memory leverage → pseudo-translate the misses → check against the terms store.
The output of each step is the input to the next.

## Closing

In a project, `kapi up` runs this sequence for you — content-memory leverage first,
AI translation where pseudo-translation stands in here, and the bound
checks after each pass. Compose the same steps into a named flow for
`kapi run <flow>` when CI needs exactly one pass.
