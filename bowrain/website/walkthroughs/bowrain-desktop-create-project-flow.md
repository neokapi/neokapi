---
id: bowrain-desktop-create-project-flow
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-create-project-flow.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: create
    kind: desktop
    duration_budget_seconds: 50
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator creates a new project from the desktop, picks the source language and targets, drops in a JSON file, and lands in the editor.

## Scene 1 — create (desktop)

User clicks New project from the workspace dashboard, fills in name, source/target locales, drags a messages.json into the file drop zone, and watches the project bootstrap and open in the editor.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
