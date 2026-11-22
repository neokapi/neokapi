# Scene script review: kapi up output (2026-11-22)

`kapi up` prints different output since the changes merged on 2026-11-22. This
document lists the videos whose screen or narration no longer matches, and
proposes the script changes. Nothing was captured, narrated or rendered. The
scripts are reviewed before any recording.

## For the founder

Recordings needed, light and dark:

- `s1-compass-converge`, `s1-compass-ship-gate`, `s2-tidewatch-build-output`
  and `s2-tidewatch-ci`: re-capture the whole take. The `kapi up` lines change
  in every one.
- `kapi-up-loop` and `kapi-project-workflow` (walkthroughs): record the tape.
- `bowrain-cli-getting-started`: re-capture, with `bin/kapi-bowrain` built from
  the same commit as `bin/kapi`.
- `07-global-launch-many-languages`: its `plan` and `status` artifacts change.
  Re-capture it when it is next recorded. It needs an AI key.

Narration lines that change, with a recommendation for each:

- `s1-compass-ship-gate`, `review`. The second `kapi up` now shows
  `produced 19`, and the caption says "the loop drafts nothing". Recommend the
  new line on this branch: "Running the loop again writes the nineteen Dutch
  drafts it held back, and asks the provider for nothing."
- `kapi-up-loop`, the `kapi up` step. The run shows `produced 0` and
  `waiting for review 3`, and the narration says "fills all French targets".
  Recommend the new line on this branch: "up recycles all three French units
  from content memory. The gate holds them for review, so up writes no target
  file yet."
- `kapi-up-loop`, a new closing `kapi up` step. The screen is now honest
  without it, because the waiting count is shown. The take still ends with no
  French file on disk, so I recommend keeping it. It shows
  `materialized 1 target file(s) · produced 3`.

Every other narration line still fits what the screen shows.

## The changes behind it

- #3693 and #3747: a pass line says what the pass did. Under gated delivery it
  reads `drafted N · recycled M · waiting for review K`. `produced N` appears
  where files are written, on the `materialized` line and in the summary. The
  summary gains `produced` and `waiting for review` lines. Units a gate holds
  count as waiting for review whether they were drafted or recycled. A block
  with nothing to translate is not counted. `kapi status --json` gains an
  `at_least` object per language.
- #3702 and #3724: `kapi up` accepts only the flags it acts on. `--target-lang`,
  `-i` and `-o` are refused with a message naming what to use instead, and
  `--format` and `--engine` name the recipe key. The `kapi up` plan line ends
  with the provider (`· demo`), and `kapi up --plan` names the provider per
  group of target languages. A server run refuses a plugin that does not
  declare its flags.
- #3696, #3705, #3726, #3718: errors for a missing key, a `-p` that is not a
  project, a recipe YAML error and a target outside the project.
- #3711, #3688, #3690: store import counts, same-format `kconv`, and store
  lookups.

## How this was checked

`bin/kapi` was built with `make build` from this branch, merged with main at
`44598323a`. A second binary was built from `0b9854ab0~1`, before the first of
these changes. Every scripted scene was run with both binaries in an isolated
sandbox that follows the isolation contract in `CLAUDE.md`, with recipe
discovery set the way the harness sets it for that demo. The two outputs were
compared line by line. Every AI step used the built-in `demo` provider. The
outputs below are copied from those runs.

The Bowrain demo was read from the code and not run, because it needs the
server stack.

## What changed per scene

| Demo | Scene | What changed on screen | Narration change |
| --- | --- | --- | --- |
| `s1-compass-converge` | `converge` | plan line ends `· demo`; pass lines; `produced 9`, `waiting for review 38` | no |
| `s1-compass-ship-gate` | `parked` | the same as `s1-compass-converge` | no |
| `s1-compass-ship-gate` | `review` | second `kapi up` shows `from stored drafts 19` and `produced 19` | yes |
| `s2-tidewatch-build-output` | `converge` | plan line; pass lines; `produced 60`, `waiting for review 60` | no |
| `s2-tidewatch-ci` | `coverage-run` | the same as `s2-tidewatch-build-output` | no |
| `s2-tidewatch-ci` | `coverage` artifact | `at_least` per language (53 to 63 lines) | no |
| `kapi-up-loop` | `kapi up` | pass lines; `produced 0`, `waiting for review 3` | yes |
| `kapi-up-loop` | new closing `kapi up` | proposed step, `produced 3` | new line |
| `kapi-project-workflow` | `kapi up` | pass line; `produced 3` in the summary | no |
| `bowrain-cli-getting-started` | `up` | pass lines (from the code) | no |
| `07-global-launch-many-languages` | `plan`, `status` artifacts | provider fields; `at_least` per language | no |

## s1-compass-converge

Published as `/video/kapi/multilingual-converge-{light,dark}.webm`. The asset
map in `regenerating-docs-assets.md` names `kapi/get-started/add-languages.mdx`.
No page on main embeds the video.

**Scene `converge` (`kapi up`).** Before:

```
plan: 26 block(s) missing · 5 recycled · 21 AI · ≈237 tokens
pass 1 done · produced 114 (+28) · still pending: nl
pass 2 done · produced 114 (+0) · still pending: nl
materialized 2 target file(s)
```

After:

```
plan: 26 block(s) missing · 5 recycled · 21 AI · ≈237 tokens · demo
pass 1 done · drafted 23 · recycled 91 · waiting for review 38 · still pending: nl
pass 2 done · recycled 19 · from stored drafts 19 · waiting for review 38 · still pending: nl
materialized 2 target file(s) · produced 9

  produced            9
  waiting for review  38
```

The nine are the four Tide window strings in Norwegian, four in German, and one
German string the stub drafted again. The 38 are every Dutch unit. The
per-language lines (`recycled 37 · AI 1` for Norwegian) are unchanged, so the
narration ("Thirty-seven of Norwegian's thirty-eight units come back out of the
project's own content memory") still fits. The `gated` narration ("Its drafts
are safe in the store; what the gate withholds is delivery") now has a count on
screen beside it. No narration change.

## s1-compass-ship-gate

Published as `/video/kapi/multilingual-ship-states-{light,dark}.webm`. The
asset map names `kapi/recipes/machine-ship-strategy.mdx`. No page on main embeds
the video.

**Scene `parked` (first `kapi up`).** The output changes as in
`s1-compass-converge`. The highlight on `waiting for review` now also lands on
the pass lines and the summary line, which supports the narration. No
narration change.

**Scene `review` (second `kapi up`, after the Dutch approvals).** Before:

```
pass 1 done · produced 114 (+18)
materialized 1 target file(s)
```

After:

```
plan: 18 block(s) missing · ... · 19 drafts · 0 AI · ≈0 tokens · demo
pass 1 done · recycled 19 · from stored drafts 19
materialized 1 target file(s) · produced 19

  produced            19
```

Current caption: "Twenty decisions, on the record, and the loop drafts
nothing." Current narration: "... Twenty approvals land in the record as they
are made. Running the loop again asks the provider for nothing."

A viewer now reads `produced 19` beside "the loop drafts nothing". The nineteen
are the Dutch drafts the first run held back, which the pass line names as
`from stored drafts 19`. Proposed caption: "Twenty decisions on the record. The
held Dutch is written, and nothing new is drafted." Proposed narration: "...
Twenty approvals land in the record as they are made. Running the loop again
writes the nineteen Dutch drafts it held back, and asks the provider for
nothing."

Both edits are on this branch. The Norwegian sidecar falls back to English for
`review` until the dogfood loop translates the new line. Re-dub `review`.

## s2-tidewatch-build-output

Published as `/video/kapi/docs-site-convergence-{light,dark}.webm`. The asset
map names `kapi/convergence.mdx`. No page on main embeds the video.

**Scene `converge` (`kapi up`).** Before:

```
pass 1 done · produced 120 (+120) · still pending: nl
pass 2 done · produced 120 (+0) · still pending: nl
materialized 4 target file(s)
```

After:

```
pass 1 done · drafted 97 · recycled 23 · waiting for review 60 · still pending: nl
pass 2 done · from stored drafts 60 · waiting for review 60 · still pending: nl
materialized 4 target file(s) · produced 60

  produced            60
  waiting for review  60
```

The narration ("Norwegian clears the ship gate on translated work. Dutch waits
for review ...") matches the new lines better than the old ones: sixty
Norwegian units written, sixty Dutch held. No narration change. The `i18n/`
tree the later scenes show is the same as before.

## s2-tidewatch-ci

Published as `/video/kapi/docs-coverage-in-ci-{light,dark}.webm`. The asset map
names `kapi/convergence-in-ci.mdx`. No page on main embeds the video.

**Scene `coverage-run` (`kapi up`).** The output changes as in
`s2-tidewatch-build-output`. The `--jq` line and `kapi check` print the same as
before. No narration change.

**Artifact `coverage` (`kapi status -p kapi.yaml --json`).** Each language gains
an object, so the report grows from 53 to 63 lines:

```
"at_least": { "draft": 60, "established": 46, "translated": 60 },
```

The narration does not name fields. Check that the 1200 by 900 frame still
holds the whole report.

## kapi-up-loop (walkthrough)

A video-mode walkthrough, `web/walkthroughs/kapi-up-loop.scene.yaml`. No docs
page embeds it on main, and it has no published capture.

**Step `kapi up`.** Before:

```
pass 1 done · produced 3 (+3) · still pending: fr
pass 2 done · produced 3 (+0) · still pending: fr
Ran flow "memory-recycle" over 1 locale(s) in 2 passes.
```

After:

```
pass 1 done · recycled 3 · waiting for review 3 · still pending: fr
pass 2 done · recycled 3 · waiting for review 3 · still pending: fr
Ran flow "memory-recycle" over 1 locale(s) in 2 passes.

  produced            0
  waiting for review  3
```

Current narration: "up fills all French targets from content memory, then
holds the locale back for review by a person. The command exits zero." The run
writes no target file, so "fills all French targets" contradicts `produced 0`.
Proposed: "up recycles all three French units from content memory. The gate
holds them for review, so up writes no target file yet. The command exits
zero."

**Proposed closing step `kapi up`.** The scene ends on `kapi status` showing
French as shippable, and no French file exists in the take at that point. A
closing `kapi up` writes it:

```
plan: every language is up to date, so this run drafts nothing and verifies gates
materialized 1 target file(s) · produced 3

  produced            3

  fr      100%     100%        ✓ shippable  —
```

Proposed narration: "The decisions released French, so up writes
messages.fr.json and reports produced 3." The step is `offline: false`, so the
embed does not change. The scene's 60-second budget may need a few more
seconds. Drop the step if the walkthrough should end on the gate.

The narration edit, the new step, the header comment and the story in
`kapi-up-loop.md` are on this branch. `gen.ts --check` passes.

## kapi-project-workflow (walkthrough)

`web/walkthroughs/kapi-project-workflow.scene.yaml`. No docs page embeds it on
main. The recipe has no ship gate, so files are written in the pass.

**Step `kapi up`.** Before: `pass 1 done · produced 3 (+3)`. After:
`pass 1 done · produced 3`, and a summary line `produced 3`. The narration
("Here content memory supplies all French translations in one pass") still
fits. No narration change.

## bowrain-cli-getting-started

Published as `/video/bowrain-cli/bowrain-cli-getting-started-{light,dark}.webm`
on the Bowrain pages `getting-started/the-loop.mdx` and
`walkthroughs/bowrain-getting-started.mdx`.

**Scene `up` (`kapi up` on the server).** Read from the code, not run. The pass
line comes from `host/upprogress.go` and drops the running total:
`produced 120 (+120)` becomes `produced 120` when the server reports what the
pass wrote. The kapi-bowrain summary prints no `produced` line, by design. The
narration speaks of the venue and the order of the work, which do not change.
No narration change.

A kapi from main refuses a server run when the kapi-bowrain plugin does not
declare its flags. The harness installs `bin/kapi-bowrain`, so build it with
`make build-bowrain-plugin` from the same commit as `bin/kapi` before the take.

## 07-global-launch-many-languages

Not embedded on any page on main. The take is a live assistant session, so the
terminal shows whatever commands the assistant chooses.

**Artifact `plan` (`kapi up --plan --json`).** The JSON gains a `provider`
field. When the run would stop for a missing key it also gains a `credential`
field with the error, and the text plan reads `AI provider: anthropic. The run
would stop before drafting: no saved credentials found ...`. The artifact runs
in a copy of the fixtures, so check that it sees the take's key. Otherwise the
artifact shows the error. The narration does not name fields.

**Artifact `status` (`kapi status -p kapi.yaml --json`).** Each of the four
languages gains an `at_least` object, and the report is about 100 lines. Check
that it fits the 1120 by 760 frame.

The prompt names no flag. If the assistant types a flag `kapi up` refuses, such
as `--target-lang`, the take shows the refusal, which names
`defaults.target_languages` and `kapi translate`. The kapi skill names no
refused flag on `kapi up`.

## What would fail if recorded today

No scripted scene fails. No demo or walkthrough passes a flag `kapi up`
refuses, and none points a recipe target outside its project. Every exit code
matches the old binary, including the `kapi check` beats that exit 3 on purpose.

Two preconditions:

- `bowrain-cli-getting-started` fails if `bin/kapi-bowrain` is older than
  `bin/kapi`.
- The assistant-driven demos (`01`, `02`, `03`, `04`, `06`, `07`, `08`) stop
  before writing anything when the take has no provider key (#3696). The
  harness supplies one when `needsAi: true`.

## Checked and unaffected

Output was identical on both binaries, apart from paths, timestamps, timings,
commit hashes, batch ids and decision ids:

- Harness demos: `s0-northsea-checks`, `s0-northsea-context` (no target
  languages, so no pass lines), `05-ai-checks-guardrail`,
  `09-toolbox-find-replace`, `10-cli-points-and-voice`, `11-cli-terms-and-queue`
  (its `kapi run converge --target-lang fr` is still accepted on `kapi run`),
  `audience-constraints`, `kapi-bilingual-workflow` (`kapi memory import` still
  reads `Imported 5 entries ... (total: 5)`), `kapi-cli-first-project`, and the
  setup of `e2-assistant-run` (`kapi terms import --name`).
- Walkthroughs: `kapi-bilingual-workflow`, `kapi-under-the-hood`,
  `kapi-terminology-checks` (`terms lookup -s en -t fr` prints the same rows),
  `kapi-terminology-pretranslation`, `kapi-explain-prompts`
  (`kapi translate --provider demo`), `kapi-pseudo-translate`, `kapi-word-count`,
  `kapi-kpz-workspace`, `kapi-review-and-approve`, `kapi-rewrite-content`,
  `kapi-keep-source-on-brand`, and `kapi-overview`. In `kapi-overview` the
  `kapi tools` list holds the same tools, in an order that is now stable from
  run to run.

Not run, and unaffected because they show no CLI output: the desktop demos
(`kapi-desktop-*`, `bowrain-desktop-*`, `bowrain-sizzle`), the Bowrain web demos
(`bowrain-web-*`), `bowrain-cli-auth-and-workspaces` (no `kapi up`) and the
explainer `e2-communication-is-context`. The `kapi-desktop-*` demos run the
recording app with its sample data, which has no `kapi up` summary.

## Edits on this branch

- `harness/demos/s1-compass-ship-gate/demo.yaml`: the `review` caption and
  narration.
- `web/walkthroughs/kapi-up-loop.scene.yaml`: the `kapi up` narration, a
  closing `kapi up` step, and the comments `kapi check` flagged.
- `web/walkthroughs/kapi-up-loop.md`: the story and the scene description.
