---
id: bowrain-desktop-translation-editor-workflow
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-translation-editor-workflow.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: editor
    kind: desktop
    duration_budget_seconds: 70
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator goes through the everyday core editing surface: split view, type translations, save, navigate to next block.

## Scene 1 — editor (desktop)

User opens a project, navigates into a file, edits 3 blocks back to back. Save toast confirms after each. Last block triggers an unsaved-changes prompt when navigating away.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
