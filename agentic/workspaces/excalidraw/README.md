# Excalidraw Localization Experiment

The first workspace-centric agentic testing experiment. AI agents collaborate to
localize [Excalidraw](https://github.com/excalidraw/excalidraw), an open-source
collaborative whiteboard tool.

## What We're Testing

- **Workspace isolation:** Each open-source project gets its own Bowrain workspace
  with context-specific agent personas (not generic roles)
- **End-to-end localization flow:** From upstream release detection through
  translation, review, QA, and PR creation
- **Agent collaboration:** L10N Engineer, language experts, and reviewer working
  together through the Bowrain platform
- **Translation quality:** Measuring accept/edit/reject rates across languages

## Agents

| Name | Role | Languages | Schedule |
|------|------|-----------|----------|
| Alex Chen | L10N Engineer | — | Weekdays 9am / 5pm |
| Sophie Martin | French Language Expert | en-US -> fr-FR | Weekdays 2pm |
| Thomas Weber | German Language Expert | en-US -> de-DE | Weekdays 2pm |
| Mei Zhang | Reviewer / QA | All | Every 2 hours |

## Target Languages

- **fr-FR** (French) — Sophie handles translation and review
- **de-DE** (German) — Thomas handles translation and review

## Content

- `src/locales/en.json` — UI strings (JSON key-value)
- `docs/**/*.md` — Documentation (Markdown)

## Running

```bash
# From the agentic/ directory:
make up-excalidraw          # Start only Excalidraw workspace agents
make logs-excalidraw        # Follow Excalidraw agent logs
```
