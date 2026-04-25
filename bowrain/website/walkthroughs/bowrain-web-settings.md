---
id: bowrain-web-settings
audience: translator
target_doc: docs/walkthroughs/bowrain-web-settings.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: settings
    kind: web
    duration_budget_seconds: 40
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Workspace owner navigates to settings to update the workspace name, view billing/usage, and manage team members.

## Scene 1 — settings (web)

User clicks Settings in the sidebar. The settings page loads with tabs for general, billing, and members. Owner edits the workspace name, saves, and the rail updates.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
