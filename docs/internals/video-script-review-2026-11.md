# Scene script review: kapi up output (2026-11-22)

`kapi up` and several store commands print different output since the changes
merged on 2026-11-22. This document lists the videos whose screen or narration
no longer matches, and proposes the script changes. Nothing was captured,
narrated or rendered. The scripts are reviewed before any recording.

The changes behind it:

- #3693: the `kapi up` pass line reads `produced N` (what the pass wrote to
  target files), with `waiting for review M` for drafts a gate holds back. The
  summary gains `produced` and `waiting for review` lines. `kapi status --json`
  gains an `at_least` object per language.
- #3702: `kapi up` accepts only the flags it acts on. `-i`, `-o`,
  `--target-lang`, `--source-lang`, `--format`, `--engine`, `-j`,
  `--parallel-blocks`, `--explain`, `--stats` and `--pack` are refused as
  unknown flags. #3724 is still open: it names the provider in `kapi up --plan`
  and refuses a server run with a plugin older than kapi.
- #3696, #3705, #3726, #3718: errors for a missing key, a `-p` that is not a
  project, a recipe YAML error and a target outside the project.
- #3711, #3688, #3690: store import counts, same-format `kconv`, and store
  lookups.

## How this was checked

`bin/kapi` was built from `99ed21a37` with `make build`. A second binary was
built from `0b9854ab0~1`, before the first of these changes. Every scripted
scene was run with both binaries in an isolated sandbox that follows the
isolation contract in `CLAUDE.md`, with recipe discovery set the way the
harness sets it for that demo. The two outputs were compared line by line.
Every AI step used the built-in `demo` provider. The outputs below are copied
from those runs.

The Bowrain demo was read from the code and not run, because it needs the
server stack.

## Summary

| Demo | Scene | What changed on screen | Narration change |
| --- | --- | --- | --- |
| `s1-compass-converge` | `converge` | pass lines and two summary lines | no |
| `s1-compass-ship-gate` | `parked` | pass lines and two summary lines | no |
| `s1-compass-ship-gate` | `review` | second `kapi up` shows `produced 19` | yes |
| `s2-tidewatch-build-output` | `converge` | pass lines and two summary lines | no |
| `s2-tidewatch-ci` | `coverage-run` | pass lines and two summary lines | no |
| `s2-tidewatch-ci` | `coverage` artifact | `at_least` per language in the JSON | no |
| `kapi-up-loop` (walkthrough) | `kapi up` step | `produced 0`, one pass instead of two | yes, and one step added |
| `kapi-project-workflow` (walkthrough) | `kapi up` step | pass line and a summary line | no |
| `bowrain-cli-getting-started` | `up` | pass lines (from the code) | no |
| `07-global-launch-many-languages` | `status` artifact | `at_least` per language in the JSON | no |

## s1-compass-converge

Published as `/video/kapi/multilingual-converge-{light,dark}.webm`. The asset
map in `regenerating-docs-assets.md` names `kapi/get-started/add-languages.mdx`.
No page on main embeds the video.

**Scene `converge` (`kapi up`).** Before:

```
pass 1 done · produced 114 (+28) · still pending: nl
pass 2 done · produced 114 (+0) · still pending: nl
```

After:

```
pass 1 done · produced 0 · waiting for review 19 · still pending: nl
pass 2 done · produced 0 · waiting for review 19 · still pending: nl
...
  produced            9
  waiting for review  19
```

The nine are the four Tide window strings in Norwegian, four in German, and one
German string the stub drafted again. The narration speaks of recycled and
drafted units ("Thirty-seven of Norwegian's thirty-eight units come back out of
the project's own content memory"), which the per-language lines still show.
No narration change.

The scene still makes its point. Re-capture the scene, light and dark.

## s1-compass-ship-gate

Published as `/video/kapi/multilingual-ship-states-{light,dark}.webm`. The
asset map names `kapi/recipes/machine-ship-strategy.mdx`. No page on main embeds
the video.

**Scene `parked` (first `kapi up`).** The output changes as in
`s1-compass-converge`. The highlight on `waiting for review` now also lands on
the new summary line, which supports the narration. No narration change.

**Scene `review` (second `kapi up`, after the Dutch approvals).** Before:

```
pass 1 done · produced 114 (+18)
```

After:

```
pass 1 done · produced 0
...
  produced            19
```

Current caption: "Twenty decisions, on the record, and the loop drafts
nothing." Current narration: "... Twenty approvals land in the record as they
are made. Running the loop again asks the provider for nothing."

A viewer now reads `produced 19` beside "the loop drafts nothing". The nineteen
are the Dutch drafts the first run held back, written now that the language is
released. Proposed caption: "Twenty decisions on the record. The held Dutch is
written, and nothing new is drafted." Proposed narration: "... Twenty approvals
land in the record as they are made. Running the loop again writes the nineteen
Dutch drafts it held back, and asks the provider for nothing."

Both edits are on this branch. The Norwegian sidecar falls back to English for
`review` until the dogfood loop translates the new line.

Re-capture the whole take, light and dark, and re-dub `review`.

## s2-tidewatch-build-output

Published as `/video/kapi/docs-site-convergence-{light,dark}.webm`. The asset
map names `kapi/convergence.mdx`. No page on main embeds the video.

**Scene `converge` (`kapi up`).** Before:

```
pass 1 done · produced 120 (+120) · still pending: nl
pass 2 done · produced 120 (+0) · still pending: nl
```

After:

```
pass 1 done · produced 0 · waiting for review 60 · still pending: nl
pass 2 done · produced 0 · waiting for review 60 · still pending: nl
...
  produced            61
  waiting for review  60
```

The narration ("Norwegian clears the ship gate on translated work. Dutch waits
for review ...") matches the new lines better than the old ones. No narration
change. The `i18n/` tree the later scenes show is byte for byte the same.

Re-capture the scene, light and dark.

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

Re-capture the take, light and dark.

## kapi-up-loop (walkthrough)

A video-mode walkthrough, `web/walkthroughs/kapi-up-loop.scene.yaml`. No docs
page embeds it on main, and it has no published capture.

**Step `kapi up`.** Before:

```
pass 1 done · produced 3 (+3) · still pending: fr
pass 2/5 · catching up fr
pass 2 done · produced 3 (+0) · still pending: fr
Ran flow "memory-recycle" over 1 locale(s) in 2 passes.
```

After:

```
pass 1 done · produced 0 · still pending: fr
Ran flow "memory-recycle" over 1 locale(s) in 1 pass.

  produced            0
```

Current narration: "up fills all French targets from content memory, then
holds the locale back for review by a person. The command exits zero." The run
writes no target file, so "fills all French targets" contradicts `produced 0`.
Proposed: "up recycles all three French units from content memory and holds
them back for review, so it writes no target file yet. The command exits
zero."

The scene ends on `kapi status` showing French as shippable, and no French
file exists in the take at that point. A closing `kapi up` writes it:

```
plan: every language is up to date, so this run drafts nothing and verifies gates
materialized 1 target file(s)

  produced            3

  fr      100%     100%        ✓ shippable  —
```

Proposed new last step, `kapi up`, with the narration "The decisions released
French, so up writes messages.fr.json and reports produced 3." It is marked
`offline: false`, so the embed does not change. The scene's 60-second budget
may need a few more seconds.

The narration edit, the new step, the header comment and the story in
`kapi-up-loop.md` are on this branch. `gen.ts --check` passes. Drop the new
step if the walkthrough should end on the gate.

Record the walkthrough, light and dark.

## kapi-project-workflow (walkthrough)

`web/walkthroughs/kapi-project-workflow.scene.yaml`. No docs page embeds it on
main.

**Step `kapi up`.** Before: `pass 1 done · produced 3 (+3)`. After:
`pass 1 done · produced 3`, and a summary line `produced 3`. The narration
("Here content memory supplies all French translations in one pass") still
fits. No narration change. Re-capture the step, light and dark.

## bowrain-cli-getting-started

Published as `/video/bowrain-cli/bowrain-cli-getting-started-{light,dark}.webm`
on the Bowrain pages `getting-started/the-loop.mdx` and
`walkthroughs/bowrain-getting-started.mdx`.

**Scene `up` (`kapi up` on the server).** Read from the code, not run. The pass
line drops the running total: `produced 120 (+120)` becomes `produced 120`, from
the server's own count. The kapi-bowrain summary prints no `produced` line, by
design. The narration speaks of the venue and the order of the work, which do
not change. No narration change. Re-capture the scene, light and dark.

When #3724 merges, `kapi up` refuses a server run if the kapi-bowrain plugin is
older than kapi. The harness installs `bin/kapi-bowrain`, so rebuild it
(`make build-bowrain-plugin`) together with `bin/kapi` before the take.

## 07-global-launch-many-languages

Not embedded on any page on main. The take is a live assistant session, so the
terminal shows whatever commands the assistant chooses.

**Artifact `plan` (`kapi up --plan --json`).** The same on both binaries. With
#3724 the text plan names the provider. The JSON may gain a `providers` field,
which #3724 adds only when languages use different providers. Check the
artifact again after #3724 merges.

**Artifact `status` (`kapi status -p kapi.yaml --json`).** Each of the four
languages gains an `at_least` object, which makes the report about 100 lines.
Check that it fits the 1120 by 760 frame. The narration does not name fields.

The prompt names no flag. If the assistant types a flag `kapi up` now refuses,
such as `--target-lang`, the take shows `Error: unknown flag` and the assistant
retries. The kapi skill names no refused flag on `kapi up`.

## What would fail if recorded today

No scripted scene fails. No demo or walkthrough passes a flag that `kapi up` now
refuses, and none points a recipe target outside its project. Every exit code
matches the old binary, including the `kapi check` beats that exit 3 on purpose.

Two preconditions:

- `bowrain-cli-getting-started` fails once #3724 merges if `bin/kapi-bowrain`
  is older than `bin/kapi`.
- The assistant-driven demos (`01`, `02`, `03`, `04`, `06`, `07`, `08`) stop
  before writing anything when the take has no provider key (#3696). The
  harness supplies one when `needsAi: true`.

## Checked and unaffected

Output was identical on both binaries, apart from paths, batch ids and decision
ids:

- Harness demos: `s0-northsea-checks`, `s0-northsea-context`,
  `05-ai-checks-guardrail`, `09-toolbox-find-replace`, `10-cli-points-and-voice`,
  `11-cli-terms-and-queue` (its `kapi run converge --target-lang fr` is still
  accepted on `kapi run`), `audience-constraints`, `kapi-bilingual-workflow`
  (`kapi memory import` still reads `Imported 5 entries ... (total: 5)`),
  `kapi-cli-first-project`, and the setup of `e2-assistant-run`
  (`kapi terms import --name`).
- Walkthroughs: `kapi-bilingual-workflow`, `kapi-under-the-hood`,
  `kapi-terminology-checks` (`terms lookup -s en -t fr` prints the same rows),
  `kapi-terminology-pretranslation`, `kapi-explain-prompts`
  (`kapi translate --provider demo`), `kapi-pseudo-translate`, `kapi-word-count`,
  `kapi-kpz-workspace`, `kapi-review-and-approve`, `kapi-rewrite-content`,
  `kapi-keep-source-on-brand`, `kapi-overview`.

Not run, and unaffected because they show no CLI output: the desktop demos
(`kapi-desktop-*`, `bowrain-desktop-*`, `bowrain-sizzle`), the Bowrain web demos
(`bowrain-web-*`), `bowrain-cli-auth-and-workspaces` (no `kapi up`) and the
explainer `e2-communication-is-context`. The `kapi-desktop-*` demos run the
recording app with its sample data, which has no `kapi up` summary.

## Leads found on the way

These are product questions, not script changes.

- Under gated delivery every pass line reads `produced 0`, because the files
  are written after the loop. A viewer sees `produced 0` twice and then
  `produced 9`.
- `s2-tidewatch-build-output` reports `produced 61` for a language of 60 units.
  The run extracts 61 blocks, of which 60 are translatable.
- In `kapi-up-loop` French is held for review with every unit recycled. The
  summary shows neither a `produced` nor a `waiting for review` count above
  zero, while the table says `waiting for review`. The count covers AI and
  stored drafts only, by design.
- On main, `kapi up --plan --provider demo` prints `AI provider: none
  configured` although the same run with `--provider demo` drafts. #3724 fixes
  the plan.
- `kapi tools` lists tools in a different order on each run, before and after
  these changes. A `kapi-overview` take shows an arbitrary order.

## Edits on this branch

- `harness/demos/s1-compass-ship-gate/demo.yaml`: the `review` caption and
  narration.
- `web/walkthroughs/kapi-up-loop.scene.yaml`: the `kapi up` narration, a
  closing `kapi up` step, and the header comment.
- `web/walkthroughs/kapi-up-loop.md`: the story and the scene description.
