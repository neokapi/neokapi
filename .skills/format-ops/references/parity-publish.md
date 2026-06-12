# parity-publish — refresh the parity dashboard data

## Purpose

Re-run the head-to-head parity suite against okapi-bridge and publish fresh
dashboard JSON. The parity report is a cache over the code; this ritual owns
its freshness contract.

## Due when

- 30-day cadence elapsed; or
- parity-affecting paths changed since the report's `generated_at`
  (`git log -1 --since=<generated_at> --format=%H -- core/formats cli/parity`
  non-empty); or
- `watermarks.report_generated_at` ≠ the report file's `generated_at`
  (orphan — adopt per the reconcile preamble instead of re-running).

## Inputs

- `make parity-sandbox` — builds the sandbox (kapi + okapi-bridge plugin).
  Prerequisite; needs Java + the pinned Okapi version.
- `make parity-publish` — runs the parity suite and publishes
  `web/static/data/parity-report.json`.
- Environment: `fts5` build tag wiring and icu4c on `PKG_CONFIG_PATH` are
  handled by the make targets; do not hand-roll `go test` invocations here.

## Steps (low freedom — exact commands)

```bash
make parity-sandbox
make parity-publish
```

1. Run the two targets above. `make parity-publish` regenerates
   `web/static/data/parity-report.json` with a fresh `generated_at`.
2. Diff the new report against the previous one (`git diff -- web/static/data/parity-report.json`).
   Tier movements are findings, not noise: a bridge that stayed byte-stable
   while a native tier moved means **native regressed** — file it as a
   followup for `remediate` (or fix immediately if within budget).
   Remember: native round-trip defaults to `TierDivergent` unless a
   per-format `MinTier` is set — absence of red is not proof of parity.
3. Any newly-passing `expected_fail` the runner logs ("assertions pass") is a
   stale xfail — note it as a followup for `xfail-hygiene`.

## Verification (→ `runs[].evidence`)

- `{check: "make parity-publish", exit, output_sha: sha256(command output)}`.
- The report parses and `generated_at` is today.

## Ledger updates

- `last_run` = today.
- `watermarks.report_generated_at` = the new report's `generated_at`.
- `watermarks.main_sha` = `git rev-parse HEAD`.
- `runs[]` entry; commit the regenerated JSON + ledger together.

## Outputs

Fresh `web/static/data/parity-report.json`; followups for regressions and
stale xfails.

## Failure → blocked

Sandbox build failure (Java/Okapi missing) or suite crash →
`outcome: "blocked"` with the failing target's output sha. Never publish a
report from a partial run; never edit the JSON by hand.
