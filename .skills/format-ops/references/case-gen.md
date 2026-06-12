# case-gen — spec-anchored test-case generation

> **BLOCKED ON [#847](https://github.com/neokapi/neokapi/issues/847) — the
> multi-view spec-case runner.** Until that issue closes: **do nothing.**
> Report `outcome: "blocked"` with the issue URL as evidence and move on.
> Generated cases without the multi-view runner and the differential oracle
> have no acceptance path — do not stage them speculatively. The ritual
> unblocks itself when the issue lands (`due.mjs` re-checks
> `gh issue view 847 --json state`).

## Purpose (once unblocked)

Generate schema-validated candidate spec cases (positive **and** negative)
from section-anchored spec clauses, classify them with the neokapi ×
okapi-bridge differential oracle, and queue **only disagreements** for human
review. Design contract: `docs/internals/format-spec-cases.md` (case
grammar, neutral block-event oracle, accept-mode guard rails, AI generation
loop) — read it before generating anything.

## Due when

- 60-day cadence elapsed (stays due while blocked); or
- `upstream-watch` landed clause changes for a cited spec; or
- a new format was accepted through the radar funnel; or
- per-section coverage fell below the floor
  (`watermarks.per_section_coverage`).

## Inputs

- Retrieval substrate: `specs/sections/<spec>/<version>/<anchor>.md`
  (per-clause units) + `specs/catalog.yaml` pins.
- Context pack: `scripts/format-ops/context-pack.mjs <id>` (dossier +
  spec.yaml + vocabulary.yaml + corpus.yaml + relevant section files, one
  schema-checked artifact — the standard input to this ritual).
- The multi-view runner + differential oracle from #847.
- Existing cases: `core/formats/<id>/spec.yaml`.

## Steps

1. Pick targets: formats/sections flagged by upstream-watch clause changes,
   newly-accepted formats, and sections below the coverage floor.
2. Generate the context pack; generate candidate cases per clause —
   positive (conforming input → expected block events) and negative
   (non-conforming input → expected clean rejection), each citing
   `{spec, version, url#fragment, clause, quote ≤1 sentence, quote_sha256}`.
3. Schema-validate every candidate; drop or fix invalid ones.
4. Run the differential oracle: neokapi and okapi-bridge both execute each
   candidate. Agreement → the case lands mechanically. **Disagreement →
   queued for human review only** (a `pending[]`-adjacent review list in the
   run report; the maintainer adjudicates which side is right against the
   cited clause).
5. Landed cases update per-section coverage; cite them from
   `vocabulary.yaml`/`spec.yaml` where they resolve evidence cells.

## Verification (→ `runs[].evidence`)

`{check: "spec-case differential run", exit, output_sha: sha256(oracle
report: n generated / n valid / n agreed / n queued)}` plus the affected
formats' `go test ./core/formats/<id>/...` exit.

## Ledger updates

`last_run`; `watermarks.per_section_coverage = {<spec>/<section>: ratio}`;
`runs[]` entry — same commit as the landed cases. Remove `blocked_on` only
when #847 is closed and the runner exists on HEAD.

## Outputs

Landed agreed cases, the disagreement review queue, coverage map update.

## Failure → blocked

Runner absent (today's state), oracle sandbox unavailable, or `specs/`
sections missing for the target → `outcome: "blocked"` with the missing
prerequisite as evidence.
