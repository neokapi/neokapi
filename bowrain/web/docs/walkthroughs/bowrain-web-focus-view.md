---
id: bowrain-web-focus-view
audience: translator
target_doc: docs/walkthroughs/bowrain-web-focus-view.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: focus-view
    kind: web
    duration_budget_seconds: 45
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator switches from split view to focus view to deep-edit a single block with full TM, term, and context panels visible.

## Scene 1 — focus-view (web)

User toggles the focus-view button. The editor switches from split view to a single-block focus mode showing source on top, target below, and TM/term/context panels expanded on the right.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
