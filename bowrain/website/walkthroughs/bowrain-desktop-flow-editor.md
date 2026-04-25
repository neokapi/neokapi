---
id: bowrain-desktop-flow-editor
audience: developer
target_doc: docs/walkthroughs/bowrain-desktop-flow-editor.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: flow
    kind: desktop
    duration_budget_seconds: 70
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Developer opens the flow editor to compose a multi-step translation pipeline: TM leverage → AI translate → QA check → terminology check, drag-drop.

## Scene 1 — flow (desktop)

User opens flow editor, drags TM-leverage tool onto canvas, then AI-translate, connects, configures provider/model. Adds QA-check, term-check. Saves the flow with a name.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
