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

Workspace owner opens the workspace settings page to confirm the workspace's name and slug, check their role, and toggle Pulse Dashboard sharing. The settings page is the same surface team members use to view what they have access to in this workspace.

## Scene 1 — settings (web)

User navigates to `/{workspace}/settings`. The General card renders with workspace details — Name, Slug, Description, Your Role — and the Pulse Dashboard section appears below for owners. The recording holds on the General card so the reader can see the layout.

## Closing

For the broader workflow that includes pushing translatable files into this workspace, see the [getting-started walkthrough](/walkthroughs/bowrain-getting-started).
