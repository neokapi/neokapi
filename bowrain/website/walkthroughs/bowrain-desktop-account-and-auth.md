---
id: bowrain-desktop-account-and-auth
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-account-and-auth.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: auth
    kind: desktop
    duration_budget_seconds: 40
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

First-time user signs into the desktop app via SSO, lands on workspace selector, picks a workspace.

## Scene 1 — auth (desktop)

User clicks Sign in. The system browser opens for SSO. After authentication, the desktop app shows the workspace selector. User picks a workspace and proceeds to the dashboard.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
