---
id: bowrain-desktop-term-explorer
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-term-explorer.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: browse
    kind: desktop
    duration_budget_seconds: 45
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator opens the term explorer in the desktop app to browse, search, and edit glossary entries.

## Scene 1 — browse (desktop)

User navigates to term explorer, browses, searches for a term, edits its target translation, saves, and sees a toast confirmation.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
