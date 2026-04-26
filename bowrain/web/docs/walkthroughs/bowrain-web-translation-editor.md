---
id: bowrain-web-translation-editor
audience: translator
target_doc: docs/walkthroughs/bowrain-web-translation-editor.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: editor
    kind: web
    duration_budget_seconds: 60
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator opens a project, navigates into a file, and uses the split-view editor to translate strings. Demonstrates source/target columns, save, and inline TM/term hints.

## Scene 1 — editor (web)

User picks a file from the project, clicks into the editor, types a translation in the target column, saves, and sees the toast confirmation. The TM panel suggests a near-match for one of the strings.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
