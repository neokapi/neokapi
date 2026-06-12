---
name: format-ops
description: >-
  Run the format-ops runbook: reconcile the ops ledger with reality, compute
  which of the twelve format-maturity rituals are due, propose a ranked
  budgeted plan, execute the due rituals from their reference prompts, record
  executable evidence in the ledger (same commit), present pending approvals
  as one batch, and close with a reflection. Use when the maintainer says
  "run the format-ops runbook", "what format work is due", "format ops
  status", after a long absence, or after a model upgrade.
---

# Format Ops Runbook

The operating process for the format estate. The contracts are the committed
docs — this skill executes them, it does not restate them:

- Process (rituals, cadences, ledger, due()): [`docs/internals/format-ops.md`](../../docs/internals/format-ops.md)
- The bar (tiers, axes, floors, scorer rules): [`docs/internals/format-maturity.md`](../../docs/internals/format-maturity.md)
- Ledger: `docs/internals/format-ops-ledger.json` · Learnings: `docs/internals/format-ops-learnings.md`

**Progressive disclosure:** do not read every reference file. Read this file,
run `due.mjs`, then load `references/<id>.md` only for rituals actually in the
plan.

## The run loop

### 0. Due computation — delegate, never recompute

```bash
node .skills/format-ops/scripts/due.mjs --model-id <your exact model id>
```

This is the only due-ness computation. **Never recompute due() in prose** —
the script applies format-ops.md §1 (cadence term, watermark term, watch-only,
blocked-on passthrough), runs the path check, and prints
BLOCKING / DUE / WATCH / BLOCKED-ON-ISSUE / PENDING-APPROVALS / OK. It is a
report, not a gate (always exits 0). Signals it cannot get offline are printed
as `needs-network` with the exact command — run those commands and fold the
results into the plan.

### 1. Reconcile preamble (ops §2.1) — before any ritual

1. **Path check** — `due.mjs` asserts every `artifacts.yaml` path exists.
   A path-drift finding is **blocking**: fix the map or stop; do not let a
   repo refactor masquerade as a staleness avalanche.
2. **Clean worktree** — require `git status` clean, or explicitly adopt/stash
   leftovers and say so in the run record.
3. **Orphan adoption** — an artifact whose `generated_at` is newer than its
   ritual's watermark with no `runs[]` entry is adopted: fast-forward the
   watermark with a `runs[]` entry `outcome: "adopted-orphan"`. Never silently
   regenerate.
4. **Atomicity** — each ritual lands its artifact changes and its ledger
   update in the **same commit**; history-snapshot appends are idempotent,
   keyed on `generated_at`.
5. **Model check** — if the session's `model_id` differs from the calibration
   watermark (`process-health.watermarks.model_id`), the calibration phase of
   `process-health` is **due-and-blocking**: `triage-score`, `remediate`, and
   prompt edits are blocked until calibration passes against the adjudicated
   set. `due.mjs --model-id` surfaces this; honor it.

### 2. Plan — ranked, budgeted (ops §2.2)

Rank the due set: blocking items first (model check, path drift), then
freshness contracts (`triage-score`), then correctness (`remediate`,
`xfail-hygiene`, `upstream-watch`), then horizon (corpus rituals,
`format-radar`, `case-gen`), then process (`process-health` — except when
blocking). Execute at most **2 heavy rituals per run** unless the maintainer
overrides ("run everything that's due" / "only the top one"). `remediate`
additionally respects the ledger's `max_fixes_per_run` (default 3). The
remainder stays due — watermarks make deferral safe. Present the plan before
executing; the maintainer can reorder.

### 3. Execute — one ritual at a time

For each ritual in the plan, load **`references/<id>.md`** and follow it
exactly. It carries the full driving prompt: inputs, watermark-source
commands, steps, the verification check, the exact ledger fields to update,
and failure→blocked handling. A ritual whose check cannot run ends
`outcome: "blocked"` — a safe, durable state, not a failure to hide.

### 4. Evidence + ledger — the same-commit rule

A ritual is done when its check **ran** and its machine output is recorded:
append to `runs[]` `{date, ritual, commit, model_id, outcome,
evidence: [{check, exit, output_sha}], followups[]}` and update the ritual's
watermarks — committed **in the same commit** as the artifact changes. Bare
watermark updates without recorded check output are the system's documented
failure mode.

### 5. Pending approvals — one batch

Rituals **propose** into `pending[]`; they never decide. At the end of the
run, present every `pending[]` item as one batch. The maintainer
approves/rejects in plain language; apply approved items and record the
outcome in `runs[]`. Always the maintainer's: tier changes, demotions,
rubric/prompt edits, radar decisions, adjudicated grades, `na` countersigns.

### 6. Reflection → learnings

Close with a short dated entry in `docs/internals/format-ops-learnings.md`:
what in the prompt/rubric/process fought the work, with evidence (transcript
pointer or diff). Proposed improvements go to the pending queue — **never
silently applied**.

## Hard rules

- **Scorer/worker separation.** A change that improves a score may not touch
  the scorer, the rubric, the audit script, `constructs.yaml`,
  `integrations.yaml`, or relax an assertion (the change-control surface in
  format-maturity.md §3). Tier changes, rubric edits, demotions, radar
  decisions, and adjudications are proposed into the pending queue and
  approved by the maintainer.
- **`cadence_days: 0` means watermark-only** (never due by time alone).
- **`ci_owned: true` rituals are watch-only**: the skill never executes them,
  it only compares the latest CI run state against the watermark and raises a
  finding when CI is red or stale — they have no cadence term at all.
- **No prompt edit without a benchmark** (`benchmarks/README.md`): the first
  edit to an unbenchmarked prompt begins by freezing current behavior as its
  benchmark; paired old-vs-new runs are recorded in `runs[]`.
- **Demotion, rejection, and decay are normal outcomes**, recorded — never
  re-litigated from scratch.

## Ritual index

| Ritual | Cadence | One line | Reference |
|---|---|---|---|
| `triage-score` | 14d | Multi-axis fleet score → dashboard + history + docs snapshot + `last_certified` | [references/triage-score.md](references/triage-score.md) |
| `remediate` | 30d | Close top gating-axis gaps, ≤ `max_fixes_per_run`, mutation-checked | [references/remediate.md](references/remediate.md) |
| `parity-publish` | 30d | Re-run parity suite, publish dashboard JSON | [references/parity-publish.md](references/parity-publish.md) |
| `contract-audit` | watch-only | Watch the CI-owned contract audit; raise findings | [references/contract-audit.md](references/contract-audit.md) |
| `upstream-watch` | 30d | Okapi tracker/tags + dossier spec feeds → drift notes, bump plan | [references/upstream-watch.md](references/upstream-watch.md) |
| `xfail-hygiene` | 60d | Remove stale xfails, raise `divergence_kind` coverage | [references/xfail-hygiene.md](references/xfail-hygiene.md) |
| `corpus-census` | 90d | Verify manifests, re-fetch wild files, licensing red lines | [references/corpus-census.md](references/corpus-census.md) |
| `corpus-sweep` | 60d | **Blocked on [#848](https://github.com/neokapi/neokapi/issues/848)** — Tier B read→write→read sweep | [references/corpus-sweep.md](references/corpus-sweep.md) |
| `case-gen` | 60d | **Blocked on [#847](https://github.com/neokapi/neokapi/issues/847)** — spec-anchored case generation | [references/case-gen.md](references/case-gen.md) |
| `format-radar` | 90d | Emergence scan → accept/reject proposals | [references/format-radar.md](references/format-radar.md) |
| `process-health` | 90d | Calibration + benchmark-gated prompt/rubric improvement | [references/process-health.md](references/process-health.md) |
| `tier-review` | watermark-only | Tier-change/demotion/countersign proposals | [references/tier-review.md](references/tier-review.md) |

## Skill files

- `scripts/due.mjs` — due-ness report (zero-LLM-cost status check)
- `artifacts.yaml` — machine-readable artifact map (single source for ops §7)
- `references/<id>.md` — full driving prompt per ritual
- `benchmarks/<ritual>/` — frozen inputs + named assertions gating prompt edits

## First ever run

Expect everything due — that is the design, not a malfunction. Follow the
bootstrap order in format-ops.md §9: seed ledger → seed `support.yaml`
(grandfathered) → backfill axis artifacts into `remediate.carryover` →
schedule the adjudication session → scorer v3 before the first fleet sweep.
Blocked-on-issue rituals (corpus-sweep #848, case-gen #847) stay blocked until
their prerequisites land; report them, do nothing else.
