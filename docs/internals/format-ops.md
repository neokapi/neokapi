# Format Ops — The Operating Process for Format Maturity

This document defines *how the format estate is maintained over time*: the
rituals, their cadences, the ledger that makes the process self-evaluating, and
the self-improvement loop that keeps the prompts and rubric current as models
improve. The bar itself (tiers, axes, levels) lives in
[format-maturity.md](./format-maturity.md); the executable spec-case design in
[format-spec-cases.md](./format-spec-cases.md); the engine knowledge base in
[format-engineering.md](./format-engineering.md); the frozen research base
behind the design in `docs/internals/research/format-ops/`.

**The maintainer's whole job** is to point Claude at the runbook skill on a
loose cadence (weekly to fortnightly is plenty; a 4–6 week absence produces
stale badges and a longer next session, never breakage — due work accumulates,
is ranked, and is budgeted):

```
"Run the format-ops runbook."        # → .skills/format-ops/SKILL.md
```

The skill then: (1) runs the reconcile preamble (§2.1), (2) reads the ledger
and live signals and computes what is due, (3) proposes a ranked, budgeted
plan, (4) executes within the budget, (5) ends every ritual with executable
evidence (test output, regenerated data, diffs) committed **in the same commit
as its ledger update**, (6) presents all pending approvals as one batch, and
(7) closes with a reflection note in the learnings file — proposed
improvements go to the pending queue, never silently applied.

## 1. Design principles

- **Signals are authoritative; the ledger stores watermarks.** Everything a
  run needs to know about "what happened since last time" is recomputed from
  durable repo signals (git history, dashboard `generated_at` fields, CI run
  states, upstream trackers). The ledger records only what each ritual last
  consumed. Due-ness is a pure function:

  ```
  due(ritual) = (cadence_days > 0 AND today − last_run > cadence_days)
                OR any(current(signal) ≠ watermark)
  ```

  `cadence_days: 0` means **watermark-only** (never due by time alone).
  `ci_owned: true` rituals are **watch-only**: the skill never executes them,
  it only compares the latest CI run state against the watermark and raises a
  finding when CI is red or stale — they have no cadence term at all.
- **Every artifact is a cache; every cache has a freshness contract.** The
  dashboard, the docs snapshot block, the parity report, and the contract
  audit are caches over the code. Each is owned by a ritual whose job
  includes regenerating it in the same run that invalidates it. (The
  existence proof: in one five-day window the scorer shipped v2, the dashboard
  stayed v1, and the docs snapshot cited a workflow that no longer existed.)
- **The verification loop is the unit of unattended work.** A ritual is not
  done when the agent says so; it is done when its check ran and its machine
  output (exit status + output hash) is recorded in `runs[].evidence`. K3 and
  the anti-gaming rules gate on these recorded outputs, not on bare
  watermarks. Rituals whose checks cannot run (sandbox missing, tool absent)
  end `blocked`, which is a safe, durable state — not a failure to hide.
- **Deterministic oracles make autonomy safe.** Spec assertions, parity,
  external validators (CI conclusions), externally re-verified corpus hashes,
  and snapshot-resolved citations are ground truth the model cannot argue
  with. Anything the process can check deterministically, it must.
- **Scorer/worker separation.** A change that improves a score may not touch
  the scorer, the rubric, the audit script, `constructs.yaml`,
  `integrations.yaml`, or relax an assertion (the change-control surface in
  [format-maturity.md §3](./format-maturity.md)). Tier changes, rubric edits,
  demotions, radar decisions, and adjudications are proposed into the pending
  queue and approved by the maintainer.
- **Demotion, rejection, and decay are normal outcomes**, recorded — never
  re-litigated from scratch (`format-demotions.md`, radar `rejected` entries
  with `revisit_after`, certification decay on the dashboard).

## 2. State channels & run mechanics

Four durable channels, each with a distinct job:

| Channel | File(s) | Holds |
|---|---|---|
| Git history | commits, one per ritual outcome (artifact + ledger together) | What changed, attributable and diffable (`git diff <runs[].commit>..HEAD -- core/formats`) |
| Ops ledger | `docs/internals/format-ops-ledger.json` | Watermarks per ritual, `pending[]` approval queue, append-only `runs[]` log |
| Machine-readable status | `web/static/data/format-maturity{,-history}.json`, `parity-report.json`, `contract-audit.json`, `core/formats/support.yaml` | Current scores, trends, promises |
| Learnings | `docs/internals/format-ops-learnings.md` | Brief, factual, dated lessons (failure modes, fix patterns, model quirks). Pruned by `process-health`; pruned entries are archived at the bottom, not deleted. |

The artifact map (§7) is single-sourced as machine-readable
**`.skills/format-ops/artifacts.yaml`** (id → path → freshness field → owning
ritual), consumed by `due.mjs` and `validate-ledger.mjs`.

### 2.1 Run preamble: reconcile

Every run starts by reconciling reality with the ledger, because half-completed
runs are the system's own documented failure mode:

1. **Path check.** Assert every `artifacts.yaml` path exists. A miss is a
   blocking *path-drift* finding (a repo refactor must degrade to one clear
   error, not a phantom staleness avalanche).
2. **Clean worktree.** Require it, or explicitly adopt/stash leftovers and say
   so in the run record.
3. **Orphan adoption.** For each cache artifact, compare its `generated_at`
   against the ledger watermark: an artifact *newer* than its watermark with
   no `runs[]` entry is adopted by fast-forwarding the watermark with a
   `runs[]` entry `outcome: "adopted-orphan"` — never silently regenerated.
4. **Atomicity.** Each ritual lands its artifact changes and its ledger update
   in the **same commit**; history-snapshot appends are idempotent, keyed on
   `generated_at`.
5. **Model check.** If the session's `model_id` differs from the
   calibration watermark, the calibration phase of `process-health` becomes
   **due-and-blocking**: `triage-score`, `remediate`, and prompt edits are
   marked `blocked` until calibration passes against the adjudicated set.
   (The maintainer saying "new model" is a courtesy, not the mechanism.)

### 2.2 Budget

The ranked plan executes at most **2 heavy rituals per run** by default
(maintainer-overridable in the invocation: "run everything that's due" /
"only the top one"). The remainder stays due — watermarks make deferral safe.
`remediate` additionally respects `max_fixes_per_run` (ledger field, default
3). The 90-day horizon rituals are seeded with staggered `last_run` offsets at
bootstrap (0/−30/−60 days) so quarterly work rotates instead of clumping.

## 3. The ritual catalog

Twelve rituals. Each has a reference file in
`.skills/format-ops/references/<id>.md` containing its full driving prompt
(model-agnostic: context pointers, steps, verification, ledger updates).
Cadences are defaults; watermark triggers override cadence in both directions.
Two rituals are **blocked on engineering prerequisites** (tracked as GitHub
issues, listed in §9) and report `blocked` until those land.

| # | Ritual | Cadence | Due when (besides cadence) | Output |
|---|---|---|---|---|
| 1 | `triage-score` | 14d | `core/formats` HEAD ≠ watermark sha; scorer/rubric changed | Multi-axis scores published to dashboard + history snapshot; docs snapshot block regenerated **in the same run**; `support.yaml` `last_certified` refreshed (that field only) |
| 2 | `remediate` | 30d | carryover non-empty; dashboard shows gating-axis gaps vs declared tiers | ≤ `max_fixes_per_run` top-gap fixes (one commit each), verified by `make test` (package tests alone are insufficient for shared surfaces); mutation-check evidence recorded per added test; carryover updated; dashboard + docs block refreshed if scores moved |
| 3 | `parity-publish` | 30d | parity-affecting paths changed since report `generated_at` | Fresh `make parity-publish` dashboard data |
| 4 | `contract-audit` | watch-only (`ci_owned`) | CI cron red or stale vs watermark | Finding raised; fix delegated to remediate |
| 5 | `upstream-watch` | 30d | any watermark family moved: Okapi issues (`updated_at` > watermark), Okapi release tag ≠ pinned `OKAPI_VERSION`, any dossier spec-source or implementation `watch` feed shows a new version | Per-spec/per-implementation drift notes written into dossiers; un-xfail candidates and backport list; an Okapi **version-bump plan** when the tag moved (all pin locations enumerated: Makefile, parity sandbox, fixtures regen, contract-audit, okapi-bridge matrix) — execution is its own approved follow-up. Annual deep window: Oct–Nov Unicode/CLDR/ICU train; quarterly: MS-* revision pages, OASIS, DTCG, W3C timed-text |
| 6 | `xfail-hygiene` | 60d | any tracked issue in `expected_fail`/annotations changed state (`gh issue` watermarks) | Stale xfails removed; `divergence_kind` coverage raised toward 100%; annotations reconciled; recorded checker output |
| 7 | `corpus-census` | 90d | any `corpus.yaml` sha mismatch; corpus release respun | Manifests verified (sha256); `origin: url` entries **re-fetched and externally verified**; licensing red-lines checked; SOURCES.md regenerated; harvest shortlist for thin formats; corpus release updated |
| 8 | `corpus-sweep` | 60d | **blocked** until the sweep harness ships (issue) | Tier B corpora executed read→write→read, one file one subprocess, wall-clock/RSS caps; per-file `OK/OK_ROUNDTRIP/EXPECTED_REJECT/CRASH/HANG/OOM/ROUNDTRIP_DRIFT`; counts diffed against the previous sweep — due-fail on unexplained deltas; minimized CRASH/HANG/OOM/DRIFT files auto-promote to `testdata/fuzz/` + `origin: bug` corpus entries |
| 9 | `case-gen` | 60d | **blocked** until the multi-view spec-case runner ships (issue); then: spec-watch landed clause changes; a new format accepted; per-section coverage below floor | Schema-validated candidate cases (positive + negative) generated from section-anchored clauses, classified by the neokapi × okapi-bridge differential oracle; only disagreements queued for human review ([format-spec-cases.md](./format-spec-cases.md)) |
| 10 | `format-radar` | 90d | — | Updated `docs/internals/format-radar.yaml`: emergence scan, candidates scored against the adoption-evidence bar, accept/reject **proposals** (with `as: format\|connector\|watch`) into the pending queue |
| 11 | `process-health` | 90d | scorer/rubric/prompt/skill files changed (`prompt_sha`/`rubric_sha`); `model_id` changed (then **blocking**, §2.1); learnings file grew | Phase 1 *calibration*: anchor-free re-score of the adjudicated golden set; agreement report; spot-replay of one recorded mutation/citation check. Phase 2 (only if phase 1 found drift or learnings demand it) *improvement*: versioned prompt/skill/rubric edit proposals, each gated by a paired old-vs-new benchmark run; learnings pruned+archived; resolved open questions pruned from the rubric doc |
| 12 | `tier-review` | watermark-only | vector suggests promotion; certification decayed; countersign requests (`na` claims) pending | Tier-change/demotion/na-countersign **proposals** into the pending queue; approved ones applied to `support.yaml` + `format-demotions.md` |

**Radar scan sources** (ritual 10) span the wider content world, not just
text/code: GitHub Octoverse growth data, standards-body announcements (W3C —
including timed-text/DAPT, OASIS, Unicode, Khronos glTF, AOUSD/USD), TMS
supported-format pages (demand signal), CMS rich-text schemas, subtitle/
caption and dubbing-exchange ecosystems, game-engine l10n table formats,
design-tool format pages, llms.txt/AGENTS.md adoption trackers.

**Corpus harvest sources** (ritual 7 reference): govdocs1 by-type tarballs
(US-gov, redistributable), NapierOne (attribution license), SafeDocs CC-MAIN
PDF shards + UNSAFE-DOCS as the hostile set, bug-tracker attachments
(LibreOffice's `get-bugzilla-attachments-by-mimetype` adapted to GitHub
issues), producer-variance matrices (the same document saved by
Word/LibreOffice/Google Docs → self-created → CC0 → Tier A). Common Crawl
grants no license to crawled content: fetch-on-demand only,
`redistributable: false`, never vendored.

**Ranking when several rituals are due:** blocking items first (§2.1 model
check, path drift), then freshness contracts (triage-score), then correctness
(remediate, xfail-hygiene, upstream-watch), then horizon work (corpus rituals,
format-radar, case-gen), then process work (process-health — except when
blocking). The skill proposes; the maintainer can reorder.

## 4. The ledger

`docs/internals/format-ops-ledger.json` — committed (so `git log` on it is
itself a signal), small enough to hand-audit, schema-checked by
`scripts/format-ops/validate-ledger.mjs`. The validator runs in
`reference-data-drift.yml`, whose path filters **must include**
`docs/internals/format-ops-ledger.json`, `core/formats/support.yaml`, and
`scripts/format-ops/**` (without these path entries the wiring is
decorative — they are part of the implementation, not an aspiration).

```jsonc
{
  "ledger_version": 1,
  "rituals": {
    "triage-score":   { "cadence_days": 14, "last_run": null,
                        "watermarks": { "core_formats_sha": "", "scorer_version": 4,
                                        "audit_sha": "", "model_id": "", "prompt_sha": "",
                                        "axes_published": [] } },
    "remediate":      { "cadence_days": 30, "last_run": null, "max_fixes_per_run": 3,
                        "watermarks": { "dashboard_generated_at": "" },
                        "carryover": [] },
    "parity-publish": { "cadence_days": 30, "last_run": null,
                        "watermarks": { "report_generated_at": "", "main_sha": "" } },
    "contract-audit": { "cadence_days": 0, "ci_owned": true, "last_run": null,
                        "watermarks": { "generatedAt": "", "okapiTag": "" } },
    "upstream-watch": { "cadence_days": 30, "last_run": null,
                        "watermarks": { "okapi_last_issue_iid": 0, "okapi_last_issue_updated_at": "",
                                        "okapi_latest_tag": "", "okapi_pinned": "1.48.0",
                                        "per_spec": {}, "per_implementation": {} },
                        "per_format_last_swept": {} },
    "xfail-hygiene":  { "cadence_days": 60, "last_run": null,
                        "watermarks": { "tracked_issues": {} } },
    "corpus-census":  { "cadence_days": 90, "last_run": null,
                        "watermarks": { "manifest_shas": {}, "release_tag": "" },
                        "external_verification": {} },
    "corpus-sweep":   { "cadence_days": 60, "last_run": null, "blocked_on": "<issue-url>",
                        "watermarks": { "per_format_counts": {} } },
    "case-gen":       { "cadence_days": 60, "last_run": null, "blocked_on": "<issue-url>",
                        "watermarks": { "per_section_coverage": {} } },
    "format-radar":   { "cadence_days": 90, "last_run": null,
                        "decided": { "accepted": [], "rejected": {} } },
    "process-health": { "cadence_days": 90, "last_run": null,
                        "watermarks": { "model_id": "", "prompt_sha": "", "rubric_sha": "",
                                        "learnings_sha": "" },
                        "golden_set": ["html","json","po","xcstrings","xliff2","properties"],
                        "adjudicated": { "rubric_sha": "", "grades": {} } },
    "tier-review":    { "cadence_days": 0, "last_run": null,
                        "watermarks": { "support_sha": "" } }
  },
  "pending": [],
  "runs": []
}
```

Structured fields with non-obvious semantics:

- **`pending[]`** — the approval queue:
  `{id, ritual, type: tier-change|demotion|rubric-edit|radar-decision|adjudication|na-countersign,
  proposal, evidence, created, expires?}`. Rituals append proposals instead of
  deciding; every run ends by presenting all pending items as one batch; the
  maintainer approves/rejects in plain language ("approve pending 3"), and the
  skill applies + records the outcome in `runs[]`. Proposals survive between
  sessions — they never live only in a chat transcript.
- **`remediate.carryover[]`**: `{format, axis, gap, attempt_date,
  outcome: "test_failed|landed|skipped|blocked", evidence}` — failed attempts
  stop evaporating and feed the next run's plan ordering.
- **`process-health.adjudicated`**: `{rubric_sha, grades: {<format>:
  {<axis>: <grade>}}}` — the human-graded answers for the golden set,
  **versioned by the rubric they were graded under**. Calibration refuses
  cross-rubric comparison; a rubric change queues a re-adjudication `pending`
  item scoped to the axes the change touched. The golden set uses `po`, not
  `mo` (`mo` is the standing retirement candidate); replacing a golden member
  is itself a recorded procedure (adjudicate the substitute, note the
  discontinuity in `runs[]`).
- **`corpus-census.external_verification`**: per-file results of re-fetching
  `origin: url|archive-member` sources (`{path: {verified_at, ok}}`) — the C3
  wild-files floor reads this, not the self-computed manifest hash.
- **`runs[]`** (append-only): `{date, ritual, commit, model_id, outcome,
  evidence: [{check, exit, output_sha}], followups[], duration_min?}`. Each
  entry's `commit` lets any future run `git diff <commit>..HEAD --
  core/formats` to enumerate exactly what was built since. Mutation-check
  evidence for remediation-added tests lives here (the floor consumes it via
  `audit-format.py --ledger`).
- **Watermark sources** (the S-signals): `core_formats_sha` =
  `git log -1 --format=%H -- core/formats`; `audit_sha` = sha256 of
  `audit-format.py --all --json` output; dashboard/report dates from their
  JSON `generated_at`/`generatedAt`; upstream from the GitLab REST API
  (project 62298414) and `…/repository/tags`; CI from
  `gh run list --workflow=<w> -L1`; tracked issues from
  `gh issue view <ids> --json state,updatedAt`; spec/implementation feeds
  from the dossier `watch` entries (GitHub `releases.atom`, W3C/OASIS/Unicode
  feeds, MS-* revision pages).

## 5. The self-improvement loop

The prompts, the rubric, and the skill are versioned artifacts with a
regression gate — generic "improvements" are not monotonic, so:

1. **Every run feeds the loop.** Rituals end with a short reflection: what in
   the prompt/rubric/process fought the work? Observations land in the
   learnings file with date + evidence (a transcript pointer or diff), not as
   immediate edits.
2. **Calibration is the trigger and the safety net** (process-health phase 1).
   It re-scores the golden set anchor-free and compares against the
   adjudicated grades *for the same `rubric_sha`*. It runs on cadence **and**
   whenever the rubric, the scorer, any ritual prompt, or the model
   generation changes — and on model change it **blocks** scoring/remediation
   until it passes (§2.1). Disagreement >20–25% on any axis means the rubric
   or the prompts drifted — stop and fix before the next fleet sweep.
3. **Edits are benchmark-gated per surface.** Each ritual owns a benchmark
   under `.skills/format-ops/benchmarks/<ritual>/`: a frozen input snapshot
   (ledger state + captured signals — dashboard JSON, canned `gh`/GitLab
   responses, dossier watch states) plus named assertions on the ritual's
   proposed plan/output (spec-watch must flag the planted version drift and
   not the unchanged specs; remediate must rank the planted gating-axis gap
   first). **No prompt edit without a benchmark: the first edit to an
   unbenchmarked prompt begins by freezing current behavior as its
   benchmark.** Scoring-prompt edits additionally require golden-set
   calibration. Both paired runs are recorded in `runs[]`.
4. **Model upgrades follow a fixed runbook**: inventory the prompt surfaces →
   freeze them → swap the model → run calibration to isolate the delta →
   only then remove stale model-specific workarounds, one at a time.
5. **Constraints migrate downward.** Anything a prompt repeats forever
   (formats list, JSON contracts, citation regex shapes) belongs in a
   validator, a schema, or a script — prompts hold judgment, not facts the
   repo already knows.

## 6. New-format adoption funnel

```
radar candidate → adoption-evidence bar → proposal (accept as format|connector|watch,
   or reject + revisit_after) → maintainer approves (pending queue)
   → implement-format skill (ladder: L1 floor → L2 specified)
   → triage-score picks it up (formats list + denominator)
   → tier-review: Available → Maintained when L2; → Supported when gates wired
```

The **adoption-evidence bar** (all required before an accept proposal): real
demand signal (TMS format pages, ecosystem growth data, user requests), a
harvestable or generatable corpus, an identifiable spec source for the
dossier, and a statement of what kapi uniquely adds
(faithfulness/vocabulary/editor angle). Outcomes are three-valued — some
candidates are **connectors** (Figma REST, cmi5/SCORM) and some are
**watch-only** (USD/glTF today); the radar records all three so they are not
re-litigated. Prefer configuring existing readers (JSON/YAML) over new
bespoke formats where faithfulness allows; bespoke is justified when the
format has inline semantics (the Portable Text rule). The ranked shortlist
lives in `docs/internals/format-radar.yaml`.

Retirement runs the same funnel backwards: tier-review proposes, the
maintainer approves, the format drops to plugin tier or is archived with its
corpus and dossier intact (never deleted — `mo` is the standing candidate).

Engineering build-out work (multi-view spec runner, sweep harness, Security
axis prerequisites, formatSpecs retirement, harvest spec.yaml migration) is
**not** radar material — those are GitHub issues (§9), per the house rule
that implementation plans live in the tracker.

## 7. Artifact map

Single-sourced in `.skills/format-ops/artifacts.yaml`; this table is its
human rendering — when they disagree, the YAML wins and the run preamble
flags the drift.

| Artifact | Path | Written by |
|---|---|---|
| Rubric (tiers + axes) | `docs/internals/format-maturity.md` | maintainer + process-health (approved edits) |
| This process doc | `docs/internals/format-ops.md` | maintainer + process-health (approved edits) |
| Spec-case design | `docs/internals/format-spec-cases.md` | maintainer + process-health |
| Research base | `docs/internals/research/format-ops/` | frozen at adoption; superseded by ritual outputs |
| Runbook skill | `.skills/format-ops/` (SKILL.md, `scripts/due.mjs`, `artifacts.yaml`, `references/*.md`, `benchmarks/*/`) | process-health (benchmark-gated) |
| Ops ledger | `docs/internals/format-ops-ledger.json` | every ritual run |
| Learnings | `docs/internals/format-ops-learnings.md` | every ritual run; pruned by process-health |
| Support tiers | `core/formats/support.yaml` | tier-review (tier fields, human-approved) + triage-score (`last_certified` only) |
| Demotions ledger | `docs/internals/format-demotions.md` | tier-review |
| Format radar | `docs/internals/format-radar.yaml` | format-radar |
| Construct registry | `core/formats/constructs.yaml` | maintainer (versioned, stable IDs; change-controlled) |
| Per-format vocabulary matrix | `core/formats/<id>/vocabulary.yaml` | implement-format + remediate |
| Per-format dossier | `core/formats/<id>/dossier.yaml` | implement-format + upstream-watch |
| Per-format corpus manifest | `core/formats/<id>/corpus.yaml` | corpus tooling + corpus-census |
| Per-format structure & geometry | `core/formats/<id>/structure.yaml` | implement-format + tier-review (`na` geometry countersign) |
| Integrations index | `core/formats/integrations.yaml` | maintainer + editor work (change-controlled) |
| Spec knowledge base | `specs/` (catalog.yaml, snapshots/, sections/) | upstream-watch + case-gen |
| Corpus store | release `format-corpus-vN`, per-format assets; fetched to `corpus/` (gitignored) | `scripts/publish-corpus.sh` / `scripts/fetch-corpus.sh` |
| Scores + trend | `web/static/data/format-maturity{,-history}.json` | triage-score (contract: [format-maturity.md §3](./format-maturity.md)) |
| Scorer | `.claude/workflows/format-triage.js` + `audit-format.py` + `repro-check.mjs` | change-controlled with the rubric |
| Ops scripts | `scripts/format-ops/` (`validate-ledger.mjs`, `check-support-gates.mjs`, `check-citations.mjs`, `context-pack.mjs`) | maintainer + process-health |

## 8. Maintainer quick reference

- **Steady state:** say *"run the format-ops runbook"* every week or two.
  Expect: a reconcile report, a due-ness report, a ranked plan capped at the
  budget (default 2 heavy rituals; say "run everything that's due" or "only
  the top one" to override), executed rituals with evidence, one batch of
  pending approvals, a ledger commit.
- **Zero-cost status check:** `node .skills/format-ops/scripts/due.mjs` —
  prints due/blocked/pending without executing anything (no LLM cost). Run it
  whenever you wonder whether a session is worth starting.
- **Approvals:** proposals wait in the ledger's `pending[]` (they survive
  between sessions). Approve or reject in plain language — "approve pending
  2, reject 3 because …" — the skill applies and records. Always yours: tier
  changes, demotions, rubric/prompt edits, radar decisions, adjudicated
  grades, `na` countersigns.
- **After an absence:** 4–6 weeks ⇒ stale tier badges on the dashboard and a
  longer plan, nothing more; one triage-score run clears the badges. Decayed
  tier display starts only at 120 days.
- **A `blocked` outcome is safe to leave.** It stays due; the evidence is in
  the last `runs[]` entry and the learnings file. Blocked-on-issue rituals
  (corpus-sweep, case-gen) unblock themselves when their prerequisite lands.
- **When a new model generation lands:** nothing to remember — the run
  preamble detects the model change and forces calibration before any
  scoring. Saying so just gets it done sooner.
- **When you want a new format:** add it to the radar (or just ask); the
  funnel (§6) takes it from evidence to tier.
- **First ever run:** expect everything due and a bootstrap plan (§9) — that
  is the design, not a malfunction.

## 9. Bootstrap

The cold-start sequence from the single-axis world (one-time; each step is a
normal ritual run, so half-completing is safe):

1. **Seed the ledger** mechanically from live signals (`last_run` from the
   artifacts' own dates; staggered offsets for the 90-day rituals: radar 0,
   corpus-census −30, process-health −60).
2. **Seed `support.yaml`** from current Engine levels with
   `grandfathered: true` on every entry — the under-run rule and certification
   decay are suspended per-format until its first multi-axis triage-score
   publishes; tier-review then proposes lifting the flag (or adjusting the
   tier) format by format.
3. **Backfill the axis artifacts** in tier order — dossier.yaml +
   corpus.yaml + vocabulary.yaml for the intended-Supported set first, the
   long tail after — seeded into `remediate.carryover` so the existing
   ranking machinery drives the burndown.
4. **Adjudication session** (the largest single approval event in the
   design, scheduled explicitly): the maintainer grades the golden set —
   6 formats × 5 axes = 30 grades, ~an hour with the audit's floor output as
   the starting point — recorded as `adjudicated` with the current
   `rubric_sha`.
5. **Scorer v3 + skill scaffolding** land before the first fleet sweep
   (change-controlled commit: scorer + audit + mirror + rubric together).
6. **Engineering prerequisites** are GitHub issues from day one:
   multi-view spec-case runner (unblocks `case-gen`), corpus-sweep harness
   (unblocks `corpus-sweep` and the C3 sweep criterion), `core/safeio` +
   fuzz scaffolding (unblocks the Security axis), tier-aware parity split
   (Supported-vs-Maintained failure routing in `parity.yml`), editor-anchor
   overlay (unblocks comparable E2 evidence), formatSpecs retirement, harvest
   spec.yaml migration.
