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

This walkthrough covers direct execution of the steps used by `kapi up`. It
accompanies [Understanding the CLI layers](/kapi/direct-execution-layer).

`kapi exec <tool>` runs one registered tool, here `recycle` for content-memory
reuse. `kapi run <flow>` executes one pass of a composed flow, here
`leverage-check`, which combines reuse with deterministic checks. In a project,
that pass stores results for `kapi merge` to write to files.

`kapi extract` and `kapi merge` also support translator handoffs: extract emits
XLIFF pre-filled from content memory, and merge applies the returned translations
and records them in content memory.

## Scene 1 — under-the-hood (terminal)

Import the content-memory bundle, run `kapi exec recycle`, then run
`kapi run leverage-check`. Use `kapi extract --target-lang fr` to emit
`out/messages.en-to-fr.xliff` and `kapi merge -i` to write `messages.fr.json`.
The example uses content-memory reuse and deterministic checks, with no provider
calls.

## Closing

Use `kapi up` for repeated convergence. Use direct commands when you need a
single tool, one flow pass or a translator handoff.
