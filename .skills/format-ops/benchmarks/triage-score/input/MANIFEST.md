# triage-score benchmark — frozen inputs

Synthetic 3-format world (html, po, csv). These files stand in for the live
artifacts named by `references/triage-score.md`; treat them as the only
reality — no network, no real repo state.

| Fixture | Stands in for |
|---|---|
| `audit.json` | `audit-format.py --all --json` (the deterministic floor, v3 axes block) |
| `prior-dashboard.json` | `web/static/data/format-maturity.json` (sticky-anchor priors, scorer v3) |
| `score-results.json` | the Score phase's per-format agent outputs (proposed levels + justifications) |

Planted conditions:

1. **po — floor regression.** The prior dashboard has po Engine at `L2`, but
   `audit.json` shows `malformed_test` deleted (signal `malformed: none`,
   engine `ceiling: "L1"`). Sticky may never preserve a prior above the
   floor ceiling: po must publish Engine `L1` with
   `delta.why: "FLOOR-FORCED demotion"` (citation-free).
2. **csv — uncited promotion.** The score agent proposes Engine `L3` (prior
   `L2`) with an empty `delta_justification`. A move publishes only with a
   cited justification: the promotion must be suppressed; csv publishes `L2`.
3. **html — control.** Proposed level equals the prior (`L3`), floor agrees
   (`ceiling: "L3"`); html must publish unchanged with no delta entry.
