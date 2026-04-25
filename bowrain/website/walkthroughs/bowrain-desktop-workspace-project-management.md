---
id: bowrain-desktop-workspace-project-management
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-workspace-project-management.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: manage
    kind: desktop
    duration_budget_seconds: 60
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Workspace owner manages projects: archive an old project, rename one, change source/target languages on another.

## Scene 1 — manage (desktop)

User opens the workspace projects view, right-clicks a project to archive, renames another via the project menu, edits target languages on a third. Each action shows a toast confirmation.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
