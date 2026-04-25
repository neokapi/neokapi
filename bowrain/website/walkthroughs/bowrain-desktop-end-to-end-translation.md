---
id: bowrain-desktop-end-to-end-translation
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-end-to-end-translation.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: e2e
    kind: desktop
    duration_budget_seconds: 90
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

From an empty workspace to a complete translation: create project, drop in a file, AI-translate, review, save.

## Scene 1 — e2e (desktop)

User creates project, drops file, runs AI-translate flow, watches progress, opens editor with all blocks pre-filled, reviews 5 blocks (1 edit, 4 accepts), saves.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
