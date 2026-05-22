---
id: bowrain-create-project
audience: developer
target_doc: docs/walkthroughs/bowrain-create-project.mdx
scenes:
  - id: create-project
    kind: terminal
    binary: kapi
    duration_budget_seconds: 25
    fixtures:
      - landing-page.html
    smoke_contract:
      - kapi pseudo-translate fixtures/landing-page.html --target-lang fr
---

## Story

A small example showing how to take an existing HTML page, run a
pseudo-translation pass with `kapi`, and inspect the result. Useful as a
"can I see what the platform does" exercise before you commit to setting
up a full Bowrain project.

## Scene 1 — create-project (terminal)

Inspect a landing-page HTML, run `kapi pseudo-translate` against it,
read back the output. The pseudo-translation expands strings with
diacritics so UI bugs are visible immediately.

## Closing

For an actual project workflow, run `kapi init` and see [Getting started](/getting-started/walkthrough).
