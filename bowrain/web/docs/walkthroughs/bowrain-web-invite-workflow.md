---
id: bowrain-web-invite-workflow
audience: translator
target_doc: docs/walkthroughs/bowrain-web-invite-workflow.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: invite
    kind: web
    duration_budget_seconds: 40
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Workspace owner sends an email invite to a teammate. The invite arrives via email; the recipient clicks the link, signs in, and is added to the workspace.

## Scene 1 — invite (web)

User clicks Invite member in the workspace settings, types an email, picks a role, and sends. A toast confirms the invite was sent.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
