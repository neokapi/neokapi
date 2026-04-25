---
id: bowrain-desktop-visual-editor
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-visual-editor.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: visual
    kind: desktop
    duration_budget_seconds: 60
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator switches to the visual editor view to see translations rendered live in the source's HTML/UI context.

## Scene 1 — visual (desktop)

User toggles visual editor mode. The right pane shows the source file rendered (HTML). User edits a target string; the rendered preview updates live with the target text.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
