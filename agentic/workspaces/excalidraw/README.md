# Excalidraw Localization Experiment

AI agents collaborate to localize [Excalidraw](https://github.com/excalidraw/excalidraw),
an open-source collaborative whiteboard tool.

## Agents

| Name | Role | Languages | Schedule |
|------|------|-----------|----------|
| Maria Dubois | French Language Expert | en-US → fr-FR | Mon/Wed/Fri 10:00 UTC |
| Katrin Weber | German Language Expert | en-US → de-DE | Tue/Thu 10:00 UTC |
| Yuki Tanaka | Japanese Language Expert | en-US → ja-JP | Mon/Wed/Fri 11:00 UTC |
| Alex Chen | Quality Reviewer | All | Tue/Thu/Sat 14:00 UTC |

The fleet coordinator runs separately (weekdays 8:00 UTC) and observes all workspaces.

## Target Languages

- **fr-FR** (French) — Maria
- **de-DE** (German) — Katrin
- **ja-JP** (Japanese) — Yuki

## Content

- `src/locales/en.json` — UI strings (JSON key-value, 605 blocks)

## Dashboard

https://agents.dev.bowrain.cloud
