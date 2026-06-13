#!/usr/bin/env node
// record-sweep.mjs — write a corpus-sweep report into the format-ops ledger.
//
// Contract: docs/internals/format-ops.md §3 ritual 8 (corpus-sweep) and §4
// (ledger schema); the corpus-sweep watermark is
// rituals.corpus-sweep.watermarks.per_format_counts. This is the ledger-write
// half of the corpus-sweep harness (issue #848): the Go driver
// (cmd/corpus-sweep) emits the machine report with `--report <path>`, and this
// script folds it into the ledger in the SAME canonical 2-space JSON style as
// scripts/format-ops/bootstrap-publish.mjs, keeping
// scripts/format-ops/validate-ledger.mjs green.
//
// Usage:
//   node scripts/format-ops/record-sweep.mjs <report.json> [--ledger <path>]
//
// Inputs : the sweep report JSON written by `corpus-sweep --report`.
// Outputs: updates rituals.corpus-sweep.{last_run, watermarks.per_format_counts}
//          and appends an idempotent runs[] entry (commit "pending", model_id
//          "deterministic"). It deliberately does NOT touch blocked_on — the
//          ritual removes that only once #848 is closed and the harness exists
//          on HEAD (per the reference), which is a human-reviewed edit.

import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { resolve, dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const argv = process.argv.slice(2)
const ledgerIdx = argv.indexOf('--ledger')
const ledgerArg = ledgerIdx >= 0 ? argv[ledgerIdx + 1] : null
const skip = ledgerIdx >= 0 ? new Set([ledgerIdx, ledgerIdx + 1]) : new Set()
const positional = argv.filter((a, i) => !skip.has(i) && !a.startsWith('--'))

if (positional.length < 1) {
  console.error('usage: record-sweep.mjs <report.json> [--ledger <path>]')
  process.exit(2)
}

const reportPath = resolve(positional[0])
// Default the ledger to the repo's committed ledger, resolved relative to this
// script (scripts/format-ops/ → repo root).
const here = dirname(fileURLToPath(import.meta.url))
const defaultLedger = join(here, '..', '..', 'docs', 'internals', 'format-ops-ledger.json')
const ledgerPath = ledgerArg ? resolve(ledgerArg) : defaultLedger

if (!existsSync(reportPath)) {
  console.error(`record-sweep: report not found: ${reportPath}`)
  process.exit(1)
}
if (!existsSync(ledgerPath)) {
  console.error(`record-sweep: ledger not found: ${ledgerPath}`)
  process.exit(1)
}

const report = JSON.parse(readFileSync(reportPath, 'utf8'))
const ledger = JSON.parse(readFileSync(ledgerPath, 'utf8'))

const cs = ledger.rituals && ledger.rituals['corpus-sweep']
if (!cs) {
  console.error('record-sweep: ledger has no corpus-sweep ritual')
  process.exit(1)
}

const date = report.generated_at
const perFormat = report.per_format_counts || {}
const formatIds = Object.keys(perFormat).sort()

// ── watermarks + last_run ───────────────────────────────────────────────────
cs.last_run = date
cs.watermarks = cs.watermarks || {}
cs.watermarks.per_format_counts = perFormat

// ── run record (idempotent on date+ritual+model) ────────────────────────────
const totals = report.totals || {}
// Hard safety failures break the run; round-trip drift is advisory.
const safetyFailures = (totals.CRASH || 0) + (totals.HANG || 0) + (totals.OOM || 0)
const drift = totals.ROUNDTRIP_DRIFT || 0
const tierNote = report.tier_b_empty_all
  ? 'Tier B empty (smoke over committed Tier A testdata)'
  : 'Tier B present'
const exit = safetyFailures > 0 ? 3 : 0

const followups = []
for (const f of report.formats || []) {
  for (const p of f.promotions || []) {
    followups.push(`promote ${p.class} ${p.source_file} → ${p.seed_file} (add origin:bug entry; file a bug)`)
  }
}

ledger.runs = ledger.runs || []
ledger.runs = ledger.runs.filter(
  (r) => !(r.date === date && r.ritual === 'corpus-sweep' && r.model_id === 'deterministic'),
)
ledger.runs.push({
  date,
  ritual: 'corpus-sweep',
  commit: 'pending',
  model_id: 'deterministic',
  outcome:
    `corpus-sweep over ${formatIds.length} format(s); ` +
    `OK ${totals.OK || 0} / OK_ROUNDTRIP ${totals.OK_ROUNDTRIP || 0} / EXPECTED_REJECT ${totals.EXPECTED_REJECT || 0} / ` +
    `ROUNDTRIP_DRIFT ${totals.ROUNDTRIP_DRIFT || 0} / CRASH ${totals.CRASH || 0} / HANG ${totals.HANG || 0} / OOM ${totals.OOM || 0}; ` +
    tierNote,
  evidence: [{ check: `corpus-sweep ${formatIds.join(',')}`, exit, output_sha: report.output_sha }],
  followups,
})

writeFileSync(ledgerPath, JSON.stringify(ledger, null, 2) + '\n')

console.log(
  `record-sweep: ${ledgerPath} — corpus-sweep last_run ${date}, ` +
    `${formatIds.length} format(s), ${safetyFailures} safety-failure(s), ${drift} drift, ` +
    `output_sha ${String(report.output_sha).slice(0, 12)}`,
)
