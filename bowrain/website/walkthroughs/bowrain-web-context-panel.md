---
id: bowrain-web-context-panel
audience: translator
target_doc: docs/walkthroughs/bowrain-web-context-panel.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: context
    kind: web
    duration_budget_seconds: 35
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator clicks the context icon on a block to see surrounding source context — file path, neighboring blocks, screenshots if available, and related TM entries.

## Scene 1 — context (web)

User clicks the context icon next to a block. A side panel slides in showing the file path, two surrounding blocks, and (if available) a screenshot from the source app marking where this block renders.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
