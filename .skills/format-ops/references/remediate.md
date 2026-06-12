# remediate — close the top gating-axis gaps

## Purpose

Land at most `max_fixes_per_run` (ledger field, default 3) verified fixes
that move formats toward their declared tiers — the smallest artifact/test
that raises a gating-axis floor. Update `carryover[]` so failed attempts stop
evaporating.

## Due when

- 30-day cadence elapsed; or
- `carryover[]` non-empty; or
- the dashboard shows gating-axis gaps versus declared tiers
  (`dashboard_generated_at` watermark moved).

**Blocked when** the session model differs from the calibration watermark.

## Inputs

- Dashboard: `web/static/data/format-maturity.json` (gaps per axis).
- Tiers: `core/formats/support.yaml` (gating: Supported = Engine≥L3 ∧
  Corpus≥C2 ∧ Knowledge≥K2; Maintained = Engine≥L2).
- `remediate.carryover[]` in the ledger (prior attempts, ranked first).
- Floor detail per format:
  `python3 .skills/refresh-format-maturity/scripts/audit-format.py <id>`.
- File floors quick reference: `docs/internals/format-maturity.md` §5.

## Steps

1. **Rank the work**: carryover entries first (skip `blocked` ones unless
   something changed; note why), then Supported-set formats under-running a
   gating axis, then Maintained, then the long tail. Within a format, pick
   the highest-leverage missing artifact from the audit's missing list.
2. For each fix, up to `max_fixes_per_run`:
   - Implement the smallest change that satisfies the floor (a missing
     `malformed_test`, a `dossier.yaml`, a `corpus.yaml` entry, a resolved
     `vocabulary.yaml` cell, …). Follow the relevant axis section of the
     rubric — floors are parsed from the named artifacts, not proxies.
   - **Scorer/worker separation (hard rule):** the change may not touch the
     scorer, the rubric, the audit script, `constructs.yaml`,
     `integrations.yaml`, or relax an assertion. If the right fix requires a
     gate change, stop and queue a `rubric-edit` pending item instead.
   - **Verify with `make test`** — package tests alone are insufficient for
     shared surfaces (registries, detection, shared helpers). Targeted
     `go test ./core/formats/<id>/...` first for speed, `make test` before
     commit.
   - **Mutation check (mandatory for every added test):** deliberately break
     the reader/writer under test (e.g. invert a condition, drop a branch),
     run the new test, confirm it goes red, revert the break. Record in
     `runs[].evidence`: the breaking change description/command, and
     `{check: "mutation:<pkg.TestName>", exit: 1, output_sha:
     sha256(red test output)}`. The floor scores the new test `partial`
     until this evidence exists (`audit-format.py --ledger`).
   - **One commit per fix**, including its `carryover[]` update.
3. Update `carryover[]` for every attempt — landed or not:
   `{format, axis, gap, attempt_date, outcome:
   "test_failed|landed|skipped|blocked", evidence}`.
4. If scores moved, refresh the dashboard + docs snapshot block in the same
   run (the triage-score publish contract applies; do not leave a stale
   cache).

## Verification (→ `runs[].evidence`)

- `make test` exit + output sha for the final state.
- One mutation-check evidence entry per added test (shape above).

## Ledger updates

- `last_run`, `watermarks.dashboard_generated_at` (from the dashboard JSON),
  `carryover[]` rewritten, `runs[]` entry appended in the final commit.

## Outputs

≤ `max_fixes_per_run` fix commits, updated carryover, refreshed dashboard +
docs block if scores moved.

## Failure → blocked

A fix whose test cannot pass, or an environment gap (icu4c off
`PKG_CONFIG_PATH`, missing parity sandbox), ends that attempt
`outcome: "test_failed"|"blocked"` in `carryover[]` with evidence — never a
silent skip, never a relaxed assertion. Footgun: golangci-lint silently
under-reports without icu4c on `PKG_CONFIG_PATH`; parity tests need the
`fts5` tag + sandbox.
