---
id: bowrain-pseudo-translate
audience: developer
target_doc: docs/walkthroughs/bowrain-pseudo-translate.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: pseudo
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 20
    fixtures: []
    smoke_contract:
      - kapi pseudo-translate
      - kapi status
---

## Story

`kapi pseudo-translate` runs the pseudo-translation flow on your
project — the same flow you'd use to catch UI truncation early, but
integrated with the project's source content and `.kapi` recipe.

## Scene 1 — pseudo (terminal)

Run `kapi pseudo-translate` from inside the project directory.
`kapi status` shows the resulting state.

## Closing

For the standalone-file pseudo-translate workflow, see the
[kapi-pseudo-translate walkthrough](https://neokapi.github.io/web/neokapi/docs/walkthroughs/kapi-pseudo-translate).
