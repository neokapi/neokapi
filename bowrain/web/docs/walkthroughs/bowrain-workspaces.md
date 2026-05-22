---
id: bowrain-workspaces
audience: developer
target_doc: docs/walkthroughs/bowrain-workspaces.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: workspaces
    kind: terminal
    binary: bowrain
    duration_budget_seconds: 30
    fixtures: []
    smoke_contract:
      - kapi workspace list
---

## Story

Workspaces are the multi-tenant boundary in Bowrain Server — a workspace
holds projects, users, settings, and billing. The CLI exposes a thin
wrapper around the REST API for scripting workspace lifecycle.

## Scene 1 — workspaces (terminal)

List existing workspaces, create a new one, list again to confirm.

## Closing

The full REST endpoints are documented in the [server REST API docs](/server/overview).
