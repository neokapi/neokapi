---
id: bowrain-web-pseudo-translation
audience: translator
target_doc: docs/walkthroughs/bowrain-web-pseudo-translation.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: pseudo
    kind: web
    duration_budget_seconds: 35
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Developer triggers a pseudo-translation flow on a project from the web app to verify UI rendering before sending to a real translator.

## Scene 1 — pseudo (web)

User opens the project menu, selects Run flow → Pseudo translate, picks the target locale, and watches the flow status update. After completion, target file shows expanded diacritic-padded strings.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
