# process-health — calibration + benchmark-gated self-improvement

## Purpose

Phase 1 **calibration**: prove the scoring system still agrees with the
human-adjudicated golden set. Phase 2 **improvement** (only if phase 1 found
drift or the learnings demand it): versioned, benchmark-gated edits to the
prompts/skill/rubric. This ritual is the only writer of the runbook skill and
the only path for rubric/prompt edits.

## Due when

- 90-day cadence elapsed; or
- scorer/rubric/prompt/skill files changed (`prompt_sha`/`rubric_sha`
  watermarks); or
- `model_id` changed — then **due-and-blocking** (§2.1): `triage-score`,
  `remediate`, and prompt edits stay blocked until calibration passes; or
- the learnings file grew (`learnings_sha`).

## Inputs (watermark-source commands)

```bash
shasum -a 256 docs/internals/format-maturity.md                 # rubric_sha
cat .skills/format-ops/SKILL.md .skills/format-ops/references/*.md | shasum -a 256  # prompt_sha
shasum -a 256 docs/internals/format-ops-learnings.md            # learnings_sha
```

- Golden set (ledger `process-health.golden_set`):
  `html, json, po, xcstrings, xliff2, properties` — **po, not mo** (`mo` is
  the standing retirement candidate).
- Adjudicated grades: `process-health.adjudicated` `{rubric_sha, grades}` —
  human-graded axis levels, **versioned by the rubric they were graded
  under**.
- Benchmarks: `.skills/format-ops/benchmarks/<ritual>/` (see its README).

## Phase 1 — calibration

1. **Re-score the golden set anchor-free**: run the triage scoring procedure
   on the six golden formats with sticky anchors OFF (no priors), same floor
   inputs as a normal run.
2. **Compare against `adjudicated.grades` for the same `rubric_sha`.**
   Calibration **refuses cross-rubric comparison**: if the current rubric
   sha ≠ `adjudicated.rubric_sha`, do not compare — queue a `pending[]` item
   `type: "adjudication"` scoped to the axes the rubric change touched, and
   end `blocked` (the maintainer must re-grade before calibration can pass).
3. **Agreement report**: per axis, the disagreement rate.
   **Disagreement >20–25% on any axis means the rubric or the prompts
   drifted — stop and fix before the next fleet sweep** (a blocking finding,
   not a note).
4. **Spot-replay one recorded check**: pick one mutation-check or
   citation-check entry from `runs[].evidence` and re-execute it; confirm
   the recorded exit/output still reproduces. A non-reproducing record is a
   process finding.
5. Passing: update `watermarks.model_id` (this is what unblocks
   scoring/remediation after a model change), `prompt_sha`, `rubric_sha`,
   `learnings_sha`.

## Phase 2 — improvement (only on drift or learnings pressure)

1. Read `docs/internals/format-ops-learnings.md`; cluster recurring
   friction.
2. For each proposed prompt/skill/rubric edit:
   - **No prompt edit without a benchmark.** If the surface has no benchmark
     under `benchmarks/<ritual>/`, the edit **begins by freezing current
     behavior as its benchmark** (inputs + named assertions), committed
     before the edit.
   - Run the benchmark against the **old** prompt, then the **new** prompt;
     both paired runs are recorded in `runs[]` with their assertion
     outcomes. The edit lands only if the new run passes every named
     assertion the old one passed (plus whatever it set out to fix).
   - **Scoring-prompt edits additionally require golden-set calibration**
     (re-run phase 1 against the edited prompt before landing).
   - Rubric edits are `pending[]` proposals (`type: "rubric-edit"`) — the
     maintainer approves; a gate/floor change lands across scorer + audit +
     mirror + rubric in one commit and re-triggers calibration.
3. **Prune the learnings file**: collapse absorbed entries; pruned entries
   are **archived at the bottom, not deleted**. Prune resolved open
   questions from the rubric doc (an editorial decision recorded here).
4. **Constraints migrate downward**: anything a prompt repeats forever
   (format lists, JSON contracts, citation regex shapes) moves into a
   validator/schema/script — prompts hold judgment, not facts the repo
   already knows.

## Model-upgrade runbook (fixed sequence)

1. Inventory the prompt surfaces (SKILL.md, references/, the scorer
   workflow prompts).
2. Freeze them (commit shas recorded).
3. Swap the model.
4. Run calibration to isolate the delta.
5. **Only then** remove stale model-specific workarounds — one at a time,
   each with its paired benchmark run.

## Golden-set membership change

Replacing a golden member is itself a recorded procedure: adjudicate the
substitute (pending `type: "adjudication"`), then note the discontinuity in
`runs[]`.

## Verification (→ `runs[].evidence`)

`{check: "calibration agreement", exit, output_sha: sha256(agreement
report)}` + the spot-replayed check's entry + paired benchmark run entries.

## Ledger updates

`last_run`; all four watermarks; `adjudicated` only via approved pending
items; `runs[]` entries for calibration and each paired benchmark run.

## Failure → blocked

No adjudicated grades yet (bootstrap §9.4), rubric/adjudication sha
mismatch, or agreement >20–25% → `outcome: "blocked"` with the agreement
report as evidence. Scoring and remediation stay blocked until calibration
passes — that is the design.
