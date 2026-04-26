---
id: bowrain-web-login-and-workspace
audience: translator
target_doc: docs/walkthroughs/bowrain-web-login-and-workspace.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: login-workspace
    kind: web
    duration_budget_seconds: 30
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

First-time user signs in via SSO and lands on their workspace dashboard. Workspace rail shows their teams; the main pane shows recent projects and quick actions.

## Scene 1 — login-workspace (web)

User opens the bowrain web app, clicks Sign in with SSO, returns from the IdP, and lands on the workspace dashboard. The workspace rail shows their workspaces and the main pane lists projects.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
