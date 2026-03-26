# Excalidraw Localization Experiment

AI agents collaborate to localize [Excalidraw](https://github.com/excalidraw/excalidraw),
an open-source collaborative whiteboard tool.

## Repos

| Repo | Role |
|------|------|
| `excalidraw/excalidraw` | Upstream project |
| `neokapi/agentic-excalidraw` | Fork used by the walker and agents |
| `neokapi/agentic-fleet` | Fleet repo with plan.yaml + status.yaml |

## Release Strategy

The coordinator walks through Excalidraw releases sequentially, pushing
source-only content to Bowrain. Target language files from the upstream repo
are stripped — Bowrain and the agents produce all translations.

| Tag | Status |
|-----|--------|
| v0.14.0 | Done |
| v0.14.2 | Next |
| v0.15.0 | Pending |
| v0.16.0 | Pending |
| v0.17.0 | Pending |
| v0.18.0 | Pending |

Content discovery is prompt-guided: the walker uses `**/locales/en.json` to
find the source file at each tag, handling the path change from `src/locales/`
(pre-v0.18) to `packages/excalidraw/locales/` (v0.18+) automatically.

## Agents

| Name | Role | Languages | Schedule |
|------|------|-----------|----------|
| Maria Dubois | French Language Expert | en-US → fr-FR | Mon/Wed/Fri 10:00 UTC |
| Katrin Weber | German Language Expert | en-US → de-DE | Tue/Thu 10:00 UTC |
| Yuki Tanaka | Japanese Language Expert | en-US → ja-JP | Mon/Wed/Fri 11:00 UTC |
| Alex Chen | Quality Reviewer | All | Tue/Thu/Sat 14:00 UTC |

The fleet coordinator runs separately (every 30min) and manages release
advancement and agent dispatch across all workspaces.

## Dashboard

https://agents.dev.bowrain.cloud
