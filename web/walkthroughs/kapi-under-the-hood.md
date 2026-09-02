---
id: kapi-under-the-hood
audience: developer
target_doc: docs/kapi/direct-execution-layer.mdx
scenes:
  - id: under-the-hood
    kind: terminal
    binary: kapi
    duration_budget_seconds: 55
    fixtures:
      - messages.json
    smoke_contract:
      - kapi memory import project.memory.json
      - kapi exec recycle messages.json -o step1.json --source-lang en --target-lang fr
      - kapi extract --target-lang fr
---

## Story

`kapi up` is porcelain: one verb that catches a project up to its ship
gates. This walkthrough is the plumbing track — the direct execution layer a
localization engineer reaches for when one move at a time is the point. It
pairs with the docs page [Understanding the CLI
layers](/kapi/direct-execution-layer).

Three layers, three verbs. `kapi exec <tool>` runs exactly one registry tool
with nothing around it — here `recycle`, the content memory step of `up`'s default flow.
`kapi run <flow>` executes one pass of one composed pipeline — here
`leverage-check`, recycle followed by the deterministic rule-based checks; inside a
project the pass commits to the project store, not to files. And
`kapi extract` / `kapi merge` are the interchange doors: a bilingual XLIFF
pre-filled from the content memory goes out to a human translator, and the returned file
merges back onto the source with content memory write-back.

Each beat is something `up` does automatically — re-extract on source drift,
produce a pass, materialize the results. The plumbing stays addressable.

## Scene 1 — under-the-hood (terminal)

Seed the content memory with `kapi memory import`, run one tool with `kapi exec recycle`, run
one composed pass with `kapi run leverage-check`, then walk the interchange:
`kapi extract --target-lang fr` emits `out/messages.en-to-fr.xliff`, and
`kapi merge -i` applies it back, writing `messages.fr.json`. Fully offline —
content-memory leverage and deterministic checks only, no provider calls.

## Closing

Day to day, `kapi up` loops all of this for you. Reach down a layer when you
need one tool's exact behavior, one flow pass, or a translator handoff — the
porcelain and the plumbing share one engine.
