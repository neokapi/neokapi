---
id: bowrain-desktop-translation-editor
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-translation-editor.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: open-and-translate
    kind: desktop
    duration_budget_seconds: 90
    seed:
      workspace: fresh
      project:
        name: "Acme Marketing"
        source_lang: en
        target_langs: [fr]
        files:
          - path: i18n/en.json
            content:
              welcome: "Welcome"
              save: "Save"
              cancel: "Cancel"
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

A translator opens the Bowrain desktop app, picks a project from their
workspace, navigates the translation editor's split-view, types a
translation in one of the target columns, and saves it. Demonstrates
the everyday core editing surface.

## Scene 1 — open-and-translate (desktop)

User launches the desktop app (Wails dev mode for recording). Workspace
rail shows the seeded "Acme Marketing" project. Click into the project,
land on the file list, click into i18n/en.json. The translation editor
opens with three blocks (welcome / save / cancel). User clicks the
target cell of "Welcome", types "Bienvenue", clicks save. Toast
confirms.

## Closing

For the focus-view single-block deep edit, see the
`bowrain-desktop-focus-view` walkthrough (scaffold pending).
