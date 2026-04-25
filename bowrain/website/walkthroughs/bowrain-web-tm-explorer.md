---
id: bowrain-web-tm-explorer
audience: translator
target_doc: docs/walkthroughs/bowrain-web-tm-explorer.mdx
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

Translator opens the TM explorer to browse, search, and filter translation memory entries scoped to the workspace.

## Scene 1 — browse (web)

User navigates to the TM explorer, scrolls through entries, types a search query, applies a language filter, and clicks an entry to see its provenance and usage history.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
