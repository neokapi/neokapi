# parity-publish — read the okapi-bridge parity report

## Purpose

The head-to-head parity suite lives in the okapi-bridge repository
(`parity/`) and runs there on a weekly schedule against the latest kapi
release. This ritual reads its latest report, compares it with the previous
one, and files what moved. It owns the freshness contract of the local copy;
it does not run the suite.

## Due when

- 30-day cadence elapsed; or
- parity-affecting paths changed since the report's `generated_at`
  (`git log -1 --since=<generated_at> --format=%H -- core/formats`
  non-empty); or
- `watermarks.report_generated_at` ≠ the report file's `generated_at`
  (orphan — adopt per the reconcile preamble instead of re-fetching).

## Inputs

- The latest successful run of the `Parity` workflow in
  `neokapi/okapi-bridge` and its `parity-report` artifact
  (`test-comparison.json` + `parity-report.json`).
- `gh` authenticated for the `neokapi` organisation.
- The previous local copy under `.parity/` (gitignored), if any.

## Steps (low freedom — exact commands)

```bash
mkdir -p .parity
[ -f .parity/parity-report.json ] && cp .parity/parity-report.json /tmp/parity-prev.json
run=$(gh run list -R neokapi/okapi-bridge --workflow parity.yml --status success --limit 1 --json databaseId --jq '.[0].databaseId')
gh run download -R neokapi/okapi-bridge "$run" -n parity-report -D .parity
```

1. Download the latest report as above. A run that is older than the latest
   kapi release measured the release before it; note the version in the run
   log (`kapi version` is printed by setup-kapi) in the ledger entry.
2. Diff `.parity/parity-report.json` against `/tmp/parity-prev.json`. Tier
   movements are findings, not noise: a bridge that stayed byte-stable while a
   native tier moved means **native regressed** — file it as a followup for
   `remediate` (or fix immediately if within budget). Remember: native
   round-trip defaults to `TierDivergent` unless a per-format `MinTier` is set
   — absence of red is not proof of parity.
3. Any newly-passing `expected_fail` the runner logged ("assertions pass") is a
   stale xfail — note it as a followup for `xfail-hygiene`.
4. A spec change in this repo reaches the suite only once a release carries
   it. To measure a change before release, run the suite locally in the bridge
   checkout with `KAPI_PARITY_NEOKAPI_DIR=<this checkout>`.

## Verification (→ `runs[].evidence`)

- `{check: "gh run download parity-report", exit, run_id, output_sha:
  sha256(parity-report.json)}`.
- The report parses and `generated_at` is the run's date.

## Ledger updates

- `last_run` = today.
- `watermarks.report_generated_at` = the report's `generated_at`.
- `watermarks.main_sha` = `git rev-parse HEAD`.
- `runs[]` entry; commit the ledger (the report itself is gitignored).

## Outputs

A fresh `.parity/parity-report.json`; followups for regressions and stale
xfails. Nothing is committed — the report lives under the gitignored
`.parity/`.

## Failure → blocked

No successful run in the last two weeks, or an artifact that does not parse →
`outcome: "blocked"` with the run id and the failing step. Never edit the JSON
by hand.
