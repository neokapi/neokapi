---
id: bowrain-desktop-focus-view-editing
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-focus-view-editing.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: focus
    kind: desktop
    duration_budget_seconds: 50
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator switches to focus view to edit a single complex block with all panels (TM, term, context) visible at full size.

## Scene 1 — focus (desktop)

User toggles focus view in the editor. The block expands to fill the column. TM panel suggests a 92% match; user accepts it; small corrections are made; save.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
