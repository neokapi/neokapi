# contract-audit — watch the CI-owned contract audit

**Watch-only (`ci_owned: true`).** The skill never executes this ritual; it
only compares the latest CI run state against the watermark and raises a
finding when CI is red or stale. It has no cadence term at all.

## Purpose

The contract audit (config-key drift between native formats and the
okapi-bridge schemas, `okapi_refs` resolution against the pinned Surefire
output) runs in CI (`.github/workflows/contract-audit.yml`) and publishes
`web/static/data/contract-audit.json`. This ritual's only job is detecting
that the CI loop broke or went red — the fix is delegated to `remediate`.

## Due when

Never "due" for execution. A **finding** is raised when:

- the latest CI run is red; or
- the latest CI run is stale relative to the watermark
  (`generatedAt`/`okapiTag` in the artifact moved without a watermark
  update, or the workflow has not run on its schedule).

## Inputs (watermark-source commands, ops §4)

```bash
gh run list --workflow=contract-audit.yml -L1 --json status,conclusion,headSha,updatedAt
node -e "const j=require('./web/static/data/contract-audit.json'); console.log(j.generatedAt, j.okapiTag)"
```

Watermarks: `contract-audit.watermarks.generatedAt` and `.okapiTag`.

## Steps

1. Run the two commands above (the `gh` one is network; if offline, record
   `needs-network` and compare only the artifact vs the watermark).
2. Classify:
   - CI green + artifact matches watermark → nothing to do; optionally
     fast-forward `last_run`/watermarks with an `adopted-orphan`-style
     `runs[]` entry if the artifact moved.
   - CI **red** → raise a finding: summarize the failing filters from the run
     log, append a `remediate.carryover[]` entry per affected format
     (`axis: "engine"` or `"knowledge"` as appropriate, evidence = run URL),
     and a learnings line.
   - CI **stale** (no recent run / artifact older than the workflow cadence)
     → raise a process finding; check the workflow file still exists and its
     path filters still match (a renamed workflow is path drift).
3. Never run `make contract-audit` from this ritual — that is the
   remediator's tool when fixing the finding.

## Verification (→ `runs[].evidence`)

`{check: "gh run list --workflow=contract-audit.yml -L1", exit,
output_sha: sha256(json output)}`.

## Ledger updates

- `watermarks.generatedAt` / `watermarks.okapiTag` = current artifact values
  (only after recording the CI state).
- `last_run` = today; `runs[]` entry with outcome `ok` (green) or
  `finding-raised`.

## Outputs

Either nothing (green) or carryover entries + a learnings line delegating the
fix to `remediate`.

## Failure → blocked

`gh` unavailable/offline → record the offline comparison only, outcome
`blocked` with the reason; the watch stays due.
