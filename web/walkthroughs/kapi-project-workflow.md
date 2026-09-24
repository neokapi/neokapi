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
      - kapi memory import project.memory.json
      - kapi status
      - kapi extract --target-lang fr
---

## Story

A project recipe declares languages, content paths and flows so commands can
reuse those settings. The checkout's derived working data sits under `.kapi/`;
content memory and other authored context are held in the workspace.

`kapi up` runs the project's flow across target locales until the ship gates are
met or remaining work requires a person. `kapi status` reports progress. Missing
target translations are pending work and do not fail an ordinary build.

The individual steps are also available directly. `kapi run <flow>` executes one
pass and stores its results; `kapi merge` writes the target files. For a human
translator, `kapi extract` emits a bilingual file pre-filled from content memory,
and `merge` applies the returned translations.

## Scene 1 — project-workflow (terminal)

Create a project with `kapi init`, list its content with `kapi ls`, import the
content-memory bundle and inspect `kapi status`. Run `kapi up`; the example's
content-memory-only flow fills the French targets without a model call. Then
show the individual `kapi run` and `kapi merge` steps, inspect the resulting
coverage, and use `kapi extract` to prepare a translator handoff.

## Closing

Commit `kapi.yaml` to share the workflow settings. Export or snapshot authored
context when another checkout needs the same terms, memory and review decisions.
