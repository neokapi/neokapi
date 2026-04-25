---
id: bowrain-desktop-tm-explorer
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-tm-explorer.mdx
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

Translator opens the TM explorer to browse and search the workspace translation memory from the desktop app.

## Scene 1 — browse (desktop)

User navigates to TM explorer, scrolls entries, types a search, picks an entry to see provenance, then exports a CSV slice.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
