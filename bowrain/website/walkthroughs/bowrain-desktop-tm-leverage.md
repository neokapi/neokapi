---
id: bowrain-desktop-tm-leverage
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-tm-leverage.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: leverage
    kind: desktop
    duration_budget_seconds: 60
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator runs TM leverage on a fresh source file in the desktop editor and watches existing matches pre-fill target blocks.

## Scene 1 — leverage (desktop)

User opens a freshly uploaded file, clicks Run TM leverage. Status bar shows progress; 60% of blocks pre-fill with high-confidence matches; rest are flagged for translation.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
