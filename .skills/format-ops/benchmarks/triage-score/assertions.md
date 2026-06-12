# triage-score benchmark — named assertions

Execute `references/triage-score.md` steps 3–5 (score → anchor → publish)
against `input/` as the only reality (skip the live-floor and docs-block
steps — there is no live repo in a benchmark). Grade each assertion pass/fail
on the *published dataset the run proposes*.

## A1 — floor-forced-demotion (planted: po lost its malformed_test)

The prior (`po` engine `L2`) outranks the floor ceiling (`L1` in
`audit.json`). The published `po` engine level MUST be `L1`, recorded with
`delta.why: "FLOOR-FORCED demotion"`, and MUST publish **without requiring a
citation** (floor-forced demotions are citation-free). Publishing `L2`
because the score agent's justification "prior holds" cites a green parity
test is a FAIL — sticky may never preserve a prior above the floor ceiling.

## A2 — uncited-promotion-suppressed (planted: csv proposes L3 with empty justification)

`csv`'s proposed engine `L3` exceeds the prior `L2` and carries an empty
`delta_justification.engine`. The promotion MUST be suppressed: published
`csv` engine level is `L2` (the prior stands). Publishing `L3` is a FAIL;
inventing a justification on the agent's behalf is a FAIL.

## A3 — control-unchanged (html)

`html` proposes exactly its prior on every axis and the floor agrees. It
MUST publish unchanged (`L3` engine, all axes equal to prior) with no delta
entry and no fabricated movement.

## A4 — engine-mirror-and-summary

The published rows keep `level`/`next_level` mirroring the **engine** axis
(`po` row `level: "L1"`, `csv` row `level: "L2"`), and `summary.by_level` is
the engine distribution over the 3 fixture formats:
`{"L1": 1, "L2": 1, "L3": 1}` (other keys zero).

## A5 — history-append-idempotent

The proposed history mutation is remove-today's-entry-then-append, keyed on
the run's `generated_at` date; no prior history entry is rewritten.
