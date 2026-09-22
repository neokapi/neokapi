# kapi skill — triggering evals

A maintainer checklist for verifying that the `kapi` Agent Skill
(`cli/skills/data/kapi/SKILL.md`) **triggers on the right tasks and not the
wrong ones**. Run it after editing the skill's `description` (the only field
loaded at agent startup, and the sole lever on triggering — across every tool
that reads `SKILL.md`: Claude Code, Copilot, Cursor, …).

Use the `skill-eval` targets for the trigger scenarios in `scripts/skilleval`.
For matched tasks across ordinary file tools, skill/CLI and MCP, use the
subscription-only `paired-eval-preflight` and `paired-eval-smoke` targets. The
paired runner defaults to six started attempts and pauses for review; it does
not establish that every trigger scenario below passes. See the
[paired evaluation note](../../web/docs/contribute/implementation/repo/paired-agent-evaluation.md).

For a manual trigger check, start a fresh session with the skill installed
(Claude Code is the reference client):

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

**Before "fixing" a missed positive, check the fixture.** Two things put a
scenario in scope, and a scenario that has neither will correctly not trigger:

- **A format an editor cannot open directly** (`.docx`, `.pptx`, `.xlf`, a
  catalog). Here the skill owns the reading and the writing.
- **A kapi project around the file.** Prose in a project is in scope whatever
  its format, because the project holds the wording that governs it. A `.md`
  file in a bare directory is not: native grep and Edit are the better tools
  there, and broadening the `description` to catch that case pushes toward "any
  find/replace", which false-triggers the code-edit negatives.

A scenario with neither is mis-specified rather than evidence of a
`description` bug: give it a file the skill owns, or a project around it.

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
| 3 | "Check `README.md` against our voice profile and fix what's off." | brand | yes | yes — `kapi voice check` gate |
| 4 | "Find every 'utilize' across `docs/` and change it to 'use'." (`docs/` **must** hold at least one opaque file — a `.docx`/`.json` — or the skill correctly won't fire; see above) | edit / toolbox | yes | yes — replaced across `.docx`+`.json`+`.md` |
| 5 | "Set up a voice profile for us from our landing page." | brand create | yes | yes — `voice.yaml` + `voice check --min-score` |
| 6 | "Translate `announcement.docx` into Japanese." | translate | yes | yes — round-trip → `kapi check` |
| 7 | "Localize `src/locales/en.json` into fr and de using our glossary." | translate | yes | partial — fr/de translated + checked, but the fixture shipped **no terms**, so the term dimension wasn't exercised (fixture gap, not a skill miss) |
| 8 | "Get `report.docx` ready for a translation vendor in French." | translate (interchange) | yes | yes — `pack`/extract → merge, content memory seeded (~45 turns; exploratory) |
| 9 | "Add i18n to this React app." | i18n | yes | blocked — correct Path A workflow, but `@neokapi/i18n-react*` npm is served from a private registry (`localhost:4873`) absent in the sandbox; extraction can't install its toolchain |
| 10 | "Set kapi up for this project." | bootstrap | yes | yes — `kapi init` scaffolded recipe + state (needs ≥25 turns; below that it reads as full i18n adoption) |
| 11 | "Bring our project's Norwegian translations up to date and flag what still needs review." | translate (kapi loop) | yes | yes — drove `kapi status`, caught nb up to 100%, surfaced the review queue |
| 12 | "Which i18n library should we use for our Next.js app?" | i18n (advice — must quote toil grades, not just pick one) | yes | partial — read the registry + `react.md` and gave a sound next-intl vs neokapi-i18n recommendation, but did **not** quote the toil grades to the user (the stated bar) |
| 13 | "Internationalize this Flutter app and translate it to German." | i18n (detect → flutter.md → gen_l10n + preset) | yes | yes (catalog) — `check --ship` green, de 100%; SDK codegen/render not run in sandbox |
| 14 | "Our app has hardcoded strings everywhere — make it translatable." | i18n (retrofit; lint/pseudo-translate sweep) | yes | blocked — same `@neokapi/i18n-react*` private-registry limit as #9; retrofit lint can't install |
| 15 | "Localize this Android app into French." | i18n (androidxml, --format flag) | yes | yes — `check --ship` green, fr 100%, `values-fr/` created |
| 16 | "Set up our brand from this repo and connect the project to Bowrain." (fixture: a fresh repo with a README + a few marketing `.md`/`.docx` files) | context discovery (onboard) | — | — (not yet run) |
| 17 | "We renamed Tidewatch to Tideguard and launched a support site — refresh our context." (fixture: a copy of `samples/northsea` with a `support/` directory added and the new name used only there) | context discovery (refresh) | — | — (not yet run) |
| 18 | "Rewrite the intro of `docs/guide.md` so it reads better." (fixture: a kapi project with a bound voice profile) | habit 1, retrieve before writing | — | — (not yet run) |
| 19 | "Read through `docs/` and tell me what you make of how we write." (fixture: a kapi project with an empty context) | habit 2, record what you notice | — | — (not yet run) |
| 20 | "Say 'log in', not 'sign in'." (as a correction, in a session where the assistant has just written "sign in" into a project file) | habit 3, record the correction | — | — (not yet run) |
| 21 | "Update the release notes for 1.3 and tell me when you're done." (fixture: a kapi project whose release notes are declared content) | habit 4, check and report | — | — (not yet run) |

Completion summary: **12/15 green** at catalog-gate depth, **2 partial** (#7
terms fixture gap, #12 didn't surface grades), **2 blocked** on the
`@neokapi/i18n-react` private npm registry being unavailable in the sandbox
(#9, #14 — the same root cause; not a skill defect). The two blocked and the
Flutter render step are the residual manual pass. Two scenario-shape lessons
worth folding back into the fixtures: **give #7 a real term seed** (else it
tests nothing it claims to), and **the neokapi-i18n Path A rows can only reach a
green gate where `@neokapi/i18n-react*` is installable** — either run the local
registry or complete them against a catalog-library path.

Scenario 11 is the kapi loop end to end: read state (`kapi status`),
catch up (`kapi up`), then surface the review queue (`kapi status --review`) —
"completed" means it drove the gate, not just translated one file.

Scenario 16 is context discovery. Its local leg completes in the
sandbox with no server: "completed" there means the project's context holds
what the task asked for (a voice profile, the recipe binding it, a term) and
`kapi voice check` passes on a repo sample. The push leg
(`kapi init --server … --anonymous` → claim URL → `kapi push`) needs a
sandboxed bowrain-server plus the kapi-bowrain plugin in the sandbox's plugin
dir, so score it separately or stop the scenario at the hand-off message.

Scenario 17 is the refresh — the second visit, on a project that already has a
context. It fails in two directions, so score both. "Completed" means the
assistant **read the drift before writing**: it found the undeclared surface
(`kapi ls --untracked`), read the record for the old name, proposed the delta,
and only then applied. It has **failed** if any governance file changed before
the user approved anything — a rewritten profile is the failure mode this
scenario exists to catch, and it looks like success from the transcript alone.
Check `git diff` at the point the assistant first asks for approval; it must be
empty. Its acceptance path runs as a test — `TestRefresh_NorthseaDrift` in
`cli/refresh_northsea_test.go` drives the same fixture through the CLI, so this
row scores the assistant's judgement rather than the verbs.

Scenarios 18 to 21 are the four habits, and they are scored differently from
every row above: the task in the prompt is ordinary writing, and what is being
measured is what the assistant did **around** it. None of them asks for kapi.

- **18** passes when the assistant read the context for that file
  (`kapi context docs/guide.md`, or the `context://` resource) before writing a
  word, and its rewrite respects what came back. It fails when it writes first
  and checks afterwards.
- **19** passes when the assistant recorded what it noticed
  (`kapi context observe`, `context_observe`), one call per fact, with
  `--seen-in`. Reading `docs/` and reporting a summary to the user alone is the
  failure this row catches: the next session starts from nothing again.
- **20** passes when the assistant recorded the correction
  (`kapi context correct "sign in" "log in" --seen-in <file>`) as well as making
  the edit. A rule it proposed and then described to the user as now in force is
  a **fail**: a candidate advises, and only a person confirms.
- **21** passes when the assistant ran the check on what it changed and ended
  its report with what the session recorded and how to review it
  (`kapi context log --session`, or `context_session_summary`). A green gate with
  no report of what was recorded is a partial.

Run 18 to 21 on both surfaces. Over MCP the tools are `context_observe`,
`context_propose`, `context_correct` and `context_session_summary`; from the
command line they are `kapi context observe`, `propose` and `correct`, and
`kapi context log`. The MCP half is the `mcp-eval` target's surface, and a habit
kept on one surface and not the other is the drift these rows exist to find.

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
| 6 | "Rename this variable everywhere it is used." (inside a kapi project) | a code edit, in a project whose prose the skill does own | — (not yet run) |
| 7 | "Why is this build failing?" (inside a kapi project) | diagnosis, no content written | — (not yet run) |

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
