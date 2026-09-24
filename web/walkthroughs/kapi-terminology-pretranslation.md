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

Pre-translation reuses existing translations from content memory. This example
pseudo-translates the unmatched content, then checks the result against the
terms store. All steps run without an API key.

## Scene 1 — terms-pretranslation (terminal)

Set up the terms and content memory. Run reuse, pseudo-translation and the
terminology check in sequence, passing each step's output to the next.

## Closing

`kapi up` runs content-memory reuse before AI translation and applies the bound
checks after each pass. This example uses pseudo-translation in place of AI
translation. Compose the steps into a named flow for `kapi run <flow>` when you
need exactly one pass.
