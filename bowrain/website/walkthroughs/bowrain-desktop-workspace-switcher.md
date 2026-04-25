---
id: bowrain-desktop-workspace-switcher
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-workspace-switcher.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: switch
    kind: desktop
    duration_budget_seconds: 30
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

User belongs to multiple workspaces and switches between them via the rail.

## Scene 1 — switch (desktop)

User clicks a different workspace icon in the rail. Dashboard transitions, project list refreshes to show that workspace's projects. URL updates.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
