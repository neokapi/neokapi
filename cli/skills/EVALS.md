# kapi skill — triggering evals

A maintainer checklist for verifying that the `kapi` Agent Skill
(`cli/skills/data/kapi/SKILL.md`) **triggers on the right tasks and not the
wrong ones**. Run it after editing the skill's `description` (the only field
loaded at agent startup, and the sole lever on triggering — across every tool
that reads `SKILL.md`: Claude Code, Copilot, Cursor, …).

There is no built-in eval runner; this is a manual checklist. Run it in a fresh
session with the skill installed (Claude Code is the reference client):

- Claude Code: `/plugin install kapi@neokapi-plugins`, or
- any tool: `npx skills add neokapi/agent-skills --skill kapi`

For each scenario, start a clean conversation, paste the prompt, and record:

- **Triggered?** — did the skill load (the assistant reads `SKILL.md`/a reference,
  or reaches for `kapi inspect`/`apply`/`translate`/`verify`)?
- **Completed?** — did it run the loop through to a green gate (`kapi verify` /
  `kapi check` passing), not just start?

Targets: **~100% trigger on positives, 0 false-triggers on negatives.** A miss on
a positive is fixed by adding the missing trigger phrasing to the `description`;
a false-trigger on a negative is fixed by narrowing it. Re-run after any change.

## Positive — must trigger

| # | Prompt | Path | Triggered | Completed |
|---|--------|------|-----------|-----------|
| 1 | "What does slide 3 of `pitch.pptx` say?" | read/edit (binary) | | |
| 2 | "Make the intro of `report.docx` more concise — keep the formatting." | edit | | |
| 3 | "Check `README.md` against our brand voice and fix what's off." | brand | | |
| 4 | "Find every 'utilize' across `docs/` and change it to 'use'." | edit / toolbox | | |
| 5 | "Set up a brand voice for us from our landing page." | brand create | | |
| 6 | "Translate `announcement.docx` into Japanese." | localize | | |
| 7 | "Localize `src/locales/en.json` into fr and de using our glossary." | localize | | |
| 8 | "Get `report.docx` ready for a translation vendor in French." | localize (interchange) | | |
| 9 | "Add i18n to this React app." | i18n | | |
| 10 | "Set kapi up for this project." | bootstrap | | |

## Negative — must NOT trigger

| # | Prompt | Why it must not fire | Triggered? (want: no) |
|---|--------|----------------------|-----------------------|
| 1 | "Refactor this Go function for readability." | code task, no content/format work | |
| 2 | "Write a Python script to parse these log files." | code authoring | |
| 3 | "Fix the failing unit test in `auth_test.go`." | code/test task | |
| 4 | "What's the capital of France?" | general knowledge | |

## Notes

- The `description` drives triggering in **every** SKILL.md-aware tool, so tune it
  once; Claude Code is the reference for running this checklist.
- Optional automation: drive each prompt headless with `claude -p "<prompt>"` in an
  isolated temp project (skill installed) and grep the transcript for skill
  activation / `kapi` invocations. Bespoke and API-metered — the manual pass above
  is the expected cadence.
- Keep the prompts in sync with the CLI surface (e.g. they assume `kapi inspect` +
  `kapi apply`, not a removed `kapi rewrite`).
