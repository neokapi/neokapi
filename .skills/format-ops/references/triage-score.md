# triage-score — fleet-wide multi-axis scoring + publish

## Purpose

Recompute the five-axis maturity vector (Engine/Vocabulary/Editor/Knowledge/
Corpus) for all 49 real formats, publish the dashboard + history snapshot,
regenerate the docs snapshot block **in the same run**, and refresh
`support.yaml` `last_certified` (that field only). Contract:
`docs/internals/format-maturity.md` §3 (scorer v3) and its dataset/history
rules — read §3 before publishing.

## Due when

- 14-day cadence elapsed; or
- `core/formats` HEAD ≠ `watermarks.core_formats_sha`; or
- scorer/rubric changed (dashboard `scorer_version` ≠ watermark, or the
  change-controlled scoring surface moved).

**Blocked when** the session model differs from the calibration watermark
(SKILL.md §1.5) — calibration first, no exceptions.

## Inputs

- Scorer workflow: `.claude/workflows/format-triage.js` (phases: prep → score
  → triage → publish). Run it via the workflow trigger, or follow its phases
  manually with the same contracts.
- Deterministic floor:
  `python3 .skills/refresh-format-maturity/scripts/audit-format.py --all --json`
  (pass `--ledger docs/internals/format-ops-ledger.json` so
  remediation-introduced tests without mutation-check evidence score
  `partial`).
- Variance mirror: pipe the audit JSON into
  `node .skills/refresh-format-maturity/scripts/repro-check.mjs`.
- Prior dashboard: `web/static/data/format-maturity.json` (sticky anchors).
- `core/formats/support.yaml` (tier rows for the additive `tier:{…}` fields).

## Steps

1. Confirm not model-blocked (`due.mjs --model-id …` shows no model-check
   BLOCKING finding).
2. Run the floor + mirror:

   ```bash
   python3 .skills/refresh-format-maturity/scripts/audit-format.py --all --json \
     | node .skills/refresh-format-maturity/scripts/repro-check.mjs
   ```

   Any `>=2-STEP` spread is a scoring leak: **stop, do not publish**, record a
   finding (learnings + followup), end the run `blocked`.
3. Score per format (the workflow's Score phase): evidence-cited dimension
   cells only — the model never free-picks a level. Levels are computed from
   floor base..ceiling bands + demote-only quality dimensions, each demotion
   citing `file:line`/`TestName` or it is dropped. Denominator: 49 real
   formats (`exec`/`jsx`/`memorytest` excluded everywhere).
4. Apply sticky anchors per axis (maturity §3): a move publishes only with a
   cited `delta_justification`; a prior **above the floor ceiling publishes
   the derived level regardless** (`delta.why: "FLOOR-FORCED demotion"`,
   citation-free); first publish of a new axis publishes the computed level
   directly (no synthesized priors).
5. Publish per the dataset contract (maturity §3, verbatim rules): rows keep
   `level`/`next_level` mirroring the engine axis; `levels:{axis→grade}`,
   per-axis grids, `tier:{declared, since, last_certified, gates[]}` are
   additive; `summary.by_level` stays the engine distribution; history is
   remove-today's-entry-then-append, deduped on `generated_at` from
   `date -u +%Y-%m-%d`; both JSON files 2-space indented.
6. Regenerate the maturity-report snapshot block in
   `docs/internals/format-maturity.md` (between the `BEGIN/END:
   gap-analysis report` markers) — same run, never hand-edited.
7. On a passing run, refresh `last_certified` in `core/formats/support.yaml`
   — **that field only**; every other field belongs to `tier-review`.

## Verification (→ `runs[].evidence`)

- The repro-check output: record `{check: "repro-check", exit, output_sha:
  sha256(stdout)}`. PINNED or single-step everywhere is the pass condition.
- `node scripts/format-ops/validate-ledger.mjs` green after the ledger edit.
- The published JSON parses and `node -e` spot-checks `generated_at` == today.

## Ledger updates

- `last_run` = today.
- `watermarks.core_formats_sha` = `git log -1 --format=%H -- core/formats`.
- `watermarks.audit_sha` = `python3 .skills/refresh-format-maturity/scripts/audit-format.py --all --json | shasum -a 256`.
- `watermarks.scorer_version` = the published `scorer_version` (3).
- `watermarks.model_id` = session model; `watermarks.prompt_sha` = sha256 of
  this reference file; `watermarks.axes_published` = the axes in the dataset.
- Append the `runs[]` entry; commit dashboard + history + docs block +
  support.yaml `last_certified` + ledger **together**.

## Outputs

Refreshed `web/static/data/format-maturity{,-history}.json`, regenerated docs
snapshot block, refreshed `last_certified`, ledger entry. Tier-change
*suggestions* the vector surfaces go to `tier-review` (note them in
`followups[]`) — this ritual never edits tier fields.

## Failure → blocked

Audit script errors, a `>=2-STEP` spread, or a missing prior dataset →
`outcome: "blocked"` with the failing command + output sha as evidence and a
learnings entry. Do not publish a partial dataset.
