---
id: bowrain-desktop-settings-configuration
audience: translator
target_doc: docs/walkthroughs/bowrain-desktop-settings-configuration.mdx
backend_url: ${BOWRAIN_BACKEND_URL}
scenes:
  - id: settings
    kind: desktop
    duration_budget_seconds: 35
    seed: { "workspace": "fresh" }
    smoke_contract:
      - GET ${BOWRAIN_BACKEND_URL}/api/v1/health
---

## Story

Translator launches the desktop app for the first time, opens settings, points it at the bowrain-server, picks UI language and theme, and confirms the connection works.

## Scene 1 — settings (desktop)

User opens Preferences, types the server URL, switches between light/dark theme, picks UI language. Save shows a green checkmark next to the server URL on successful health check.

## Closing

See the [getting-started walkthrough](/walkthroughs/bowrain-getting-started)
for the broader CLI workflow.
