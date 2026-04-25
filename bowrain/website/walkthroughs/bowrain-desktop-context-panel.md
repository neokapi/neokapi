---
id: bowrain-desktop-context-panel
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-context-panel.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: context
    kind: desktop
    duration_budget_seconds: 40
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator clicks the context icon on a block in the desktop editor to see source context, screenshots, and related TM entries inline.

## Scene 1 — context (desktop)

User clicks context icon next to a block. The context panel expands showing file path, neighboring blocks, and a source screenshot. User scrolls to see related TM entries.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
