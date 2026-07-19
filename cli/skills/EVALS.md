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
  or reaches for `kapi inspect`/`apply`/`translate`/`check --ship`)?
- **Completed?** — did it run the loop through to a green gate (`kapi check --ship` /
  `kapi check` passing), not just start?

Targets: **~100% trigger on positives, 0 false-triggers on negatives.** A miss on
a positive is fixed by adding the missing trigger phrasing to the `description`;
a false-trigger on a negative is fixed by narrowing it. Re-run after any change.

**Before "fixing" a missed positive, check the fixture.** The `description`
scopes the skill to formats an editor *can't* open directly. A scenario whose
files are all plain text the editor handles natively (`.md`, `.txt`) will
correctly *not* trigger — native grep/Edit is the better tool there, and
broadening the `description` to catch it pushes toward "any find/replace",
which false-triggers the code-edit negatives. That is a mis-specified scenario,
not a `description` bug: give the scenario a file the skill actually owns.

## Positive — must trigger

Triggering last run: **2026-07-11**, headless, against `cab47a875`.
Completion last run: **2026-07-17**, headless, against `8bb195640` in a
dedicated **sandboxed** kapi (isolated config/data/cache/plugins; Gemini via env
only; the user's installed kapi and keychain are never used). Completion is
**catalog-gate depth** by decision — `kapi check --ship` green on the generated
catalogs; the app is not booted, so in-locale *rendering* is not verified here
(still a manual pass for the render-gated rows).

| # | Prompt | Path | Triggered | Completed |
|---|--------|------|-----------|-----------|
| 1 | "What does slide 3 of `pitch.pptx` say?" | read/edit (binary) | yes | yes — read via `kcat` |
| 2 | "Make the intro of `report.docx` more concise — keep the formatting." | edit | yes | yes — `ksed`, formatting preserved |
| 3 | "Check `README.md` against our brand voice and fix what's off." | brand | yes | yes — `kapi brand check` gate |
| 4 | "Find every 'utilize' across `docs/` and change it to 'use'." (`docs/` **must** hold at least one opaque file — a `.docx`/`.json` — or the skill correctly won't fire; see above) | edit / toolbox | yes | yes — replaced across `.docx`+`.json`+`.md` |
| 5 | "Set up a brand voice for us from our landing page." | brand create | yes | yes — `brand.yaml` + `brand check --min-score` |
| 6 | "Translate `announcement.docx` into Japanese." | localize | yes | yes — round-trip → `kapi check` |
| 7 | "Localize `src/locales/en.json` into fr and de using our glossary." | localize | yes | partial — fr/de translated + checked, but the fixture shipped **no glossary**, so the term dimension wasn't exercised (fixture gap, not a skill miss) |
| 8 | "Get `report.docx` ready for a translation vendor in French." | localize (interchange) | yes | yes — `pack`/extract → merge, TM seeded (~45 turns; exploratory) |
| 9 | "Add i18n to this React app." | i18n | yes | blocked — correct Path A workflow, but `@neokapi/i18n-react*` npm is served from a private registry (`localhost:4873`) absent in the sandbox; extraction can't install its toolchain |
| 10 | "Set kapi up for this project." | bootstrap | yes | yes — `kapi init` scaffolded recipe + state (needs ≥25 turns; below that it reads as full i18n adoption) |
| 11 | "Bring our project's Norwegian translations up to date and flag what still needs review." | localize (kapi loop) | yes | yes — drove `kapi status`, caught nb up to 100%, surfaced the review queue |
| 12 | "Which i18n library should we use for our Next.js app?" | i18n (advice — must quote toil grades, not just pick one) | yes | partial — read the registry + `react.md` and gave a sound next-intl vs neokapi-i18n recommendation, but did **not** quote the toil grades to the user (the stated bar) |
| 13 | "Internationalize this Flutter app and translate it to German." | i18n (detect → flutter.md → gen_l10n + preset) | yes | yes (catalog) — `check --ship` green, de 100%; SDK codegen/render not run in sandbox |
| 14 | "Our app has hardcoded strings everywhere — make it translatable." | i18n (retrofit; lint/pseudo-translate sweep) | yes | blocked — same `@neokapi/i18n-react*` private-registry limit as #9; retrofit lint can't install |
| 15 | "Localize this Android app into French." | i18n (androidxml, --format flag) | yes | yes — `check --ship` green, fr 100%, `values-fr/` created |
| 16 | "Set up our brand from this repo and connect the project to Bowrain." (fixture: a fresh repo with a README + a few marketing `.md`/`.docx` files) | starter pack (onboard) | — | — (not yet run) |

Completion summary: **12/15 green** at catalog-gate depth, **2 partial** (#7
glossary fixture gap, #12 didn't surface grades), **2 blocked** on the
`@neokapi/i18n-react` private npm registry being unavailable in the sandbox
(#9, #14 — the same root cause; not a skill defect). The two blocked and the
Flutter render step are the residual manual pass. Two scenario-shape lessons
worth folding back into the fixtures: **give #7 a real glossary** (else it
tests nothing it claims to), and **the neokapi-i18n Path A rows can only reach a
green gate where `@neokapi/i18n-react*` is installable** — either run the local
registry or complete them against a catalog-library path.

Scenario 11 is the kapi loop end to end: read state (`kapi status`),
catch up (`kapi up`), then surface the review queue (`kapi status --review`) —
"completed" means it drove the gate, not just translated one file.

Scenario 16 is the starter-pack onboarding. Its local leg completes in the
sandbox with no server: "completed" there means the pack files exist
(`brand.yaml`, the recipe binding it, a committed term seed) and
`kapi brand check` passes on a repo sample. The push leg
(`kapi init --server … --anonymous` → claim URL → `kapi push`) needs a
sandboxed bowrain-server plus the kapi-bowrain plugin in the sandbox's plugin
dir, so score it separately or stop the scenario at the hand-off message.

Scenario 4 is the cross-format sweep, and its fixture carries the whole point:
`grep` cannot see inside a `.docx`, so a `docs/` of plain `.md` alone tests
nothing (the assistant reaches for native grep/Edit, and is right to). With an
opaque file in the mix the assistant notices grep can't read it, reaches for
`kgrep`, and loads the skill.

## Negative — must NOT trigger

| # | Prompt | Why it must not fire | Triggered? (want: no) |
|---|--------|----------------------|-----------------------|
| 1 | "Refactor this Go function for readability." | code task, no content/format work | no |
| 2 | "Write a Python script to parse these log files." | code authoring | no |
| 3 | "Fix the failing unit test in `auth_test.go`." | code/test task | no |
| 4 | "What's the capital of France?" | general knowledge | no |
| 5 | "Format this date according to the user's locale." | locale-aware *code*, not content/catalog work | no |

## Notes

- The `description` drives triggering in **every** SKILL.md-aware tool, so tune it
  once; Claude Code is the reference for running this checklist.
- Optional automation (how the run above was driven): give each scenario its own
  temp workspace with the skill at `.claude/skills/kapi/` **and a fixture where
  the referenced files actually exist** — a prompt about `pitch.pptx` in an empty
  dir tests nothing. Then, per scenario:
  ```bash
  claude -p "<prompt>" --max-turns 5 --permission-mode bypassPermissions \
    --output-format stream-json --verbose > run.jsonl
  jq -r 'select(.type=="assistant")|.message.content[]?
         |select(.type=="tool_use" and .name=="Skill")|.input.skill' run.jsonl
  ```
  A `Skill(kapi)` tool_use is the activation signal. The turn cap is what keeps a
  positive from running away into metered translation; scenarios run in parallel.
  API-metered, so the manual pass remains the expected cadence.
- Triggering is stochastic: before concluding a scenario regressed, re-run it a
  few times, and A/B it against the previous `description` (`git show <sha>`) to
  tell a real regression from noise or a bad fixture.
- Keep the prompts in sync with the CLI surface (e.g. they assume `kapi inspect` +
  `kapi apply`, not a removed `kapi rewrite`).
