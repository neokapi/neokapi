---
id: bowrain-web-term-explorer
audience: translator
target_doc: docs/walkthroughs/bowrain-web-term-explorer.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: browse
    kind: web
    duration_budget_seconds: 45
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator opens the term explorer to browse the workspace glossary, edit a translation, and see where the term is used across projects.

## Scene 1 — browse (web)

User navigates to the term explorer, finds a term, edits its target translation, saves, and the affected projects panel updates to show the propagation.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
