---
id: kapi-project-workflow
audience: developer
target_doc: docs/kapi/get-started/first-project.mdx
scenes:
  - id: project-workflow
    kind: terminal
    binary: kapi
    duration_budget_seconds: 55
    fixtures:
      - messages.json
    smoke_contract:
      - kapi init --name demo --source-locale en --target-locale fr
      - kapi ls
      - kapi memory import project.tmx
      - kapi status
      - kapi extract --target-lang fr
---

## Story

A `.kapi` project is the day-to-day working model: capture the languages, the
content globs, and the flows once in a committed recipe, then drive the project
without repeating flags. The recipe sits beside a `.kapi/` state directory —
the project store that accumulates block overlays and content memory as
you work.

The verb that drives it is `kapi up`. It treats the recipe as the desired
state: it runs the project's flow across every target locale, pass after pass,
until each ship gate is met or the remainder parks for a person. `kapi status`
shows the derived standing before and after — a locale that is behind is
pending work, never a build failure.

Under the hood, `up` loops plumbing you can run by hand. `kapi run <flow>`
executes one pass of one named flow; inside a project a pass is
**process-only** — it commits results to the project store rather than writing
files. `kapi merge` materializes the localized files from the store. And when
a person does the translating, `kapi extract` emits a bilingual file
pre-filled from the content memory, and `merge` applies the return.

## Scene 1 — project-workflow (terminal)

Scaffold a project with `kapi init`, list the tracked content with `kapi ls`,
seed the project content memory with `kapi memory import`, and read the
before-grid with `kapi status`. Then `kapi up` brings the project up to date —
the recipe's content memory-only flow fills real `fr` targets, no LLM, fully offline. The
closing beats run the plumbing by hand: one `kapi run` pass into the store,
`kapi merge` to write `messages.fr.json`, the after-grid at 100%, and
`kapi extract` as the translator handoff.

## Closing

Commit the `kapi.yaml` recipe and anyone who clones the repository brings the
same project up to date with one command — the recipe is the portable contract,
and `kapi up` is the verb that reconciles reality to it.
