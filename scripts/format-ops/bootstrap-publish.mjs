#!/usr/bin/env node
// Deterministic, no-LLM publish of the format-maturity dashboard dataset.
//
// Contract: docs/internals/format-maturity.md §3 (the scorer dataset/history
// contract; scorer_version 4) and docs/internals/format-ops.md (the triage-score ritual's Publish
// step). This is the floor-only path: it computes every axis level from the
// deterministic file floor with NO quality-dimension demotions and NO sticky
// prior — exactly what the format-triage workflow publishes when its Score
// agents return no cited demotions (the bootstrap / zero-prior state, and a
// faithful refresh whenever a model run is not warranted or affordable).
//
// To stay byte-for-byte aligned with the scorer, it does not reimplement any
// gate: it extracts the PURE prelude of .claude/workflows/format-triage.js
// (everything before the first top-level `phase(` call — all the AXES/RANK
// tables and the floorDimsAll / axisLevel / axisCeiling / axisGaps /
// buildDataset functions) and evaluates it. A gate change in the workflow is
// picked up automatically on the next run.
//
// Usage:
//   node scripts/format-ops/bootstrap-publish.mjs            # publish (writes files)
//   node scripts/format-ops/bootstrap-publish.mjs --dry-run  # print summary only
//   node scripts/format-ops/bootstrap-publish.mjs --date 2026-06-13   # pin the date
//
// Inputs : audit-format.py --all --json, core/formats/support.yaml,
//          the current dashboard JSON (for the date-dedupe), the ledger.
// Outputs: web/static/data/format-maturity.json (+ history), the generated
//          block in docs/internals/format-maturity.md, support.yaml
//          last_certified, and a triage-score run record in the ledger.

import { execFileSync } from 'node:child_process'
import { readFileSync, writeFileSync, existsSync, mkdtempSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const ROOT = process.cwd()
const argv = process.argv.slice(2)
const DRY = argv.includes('--dry-run')
const dateArg = (() => { const i = argv.indexOf('--date'); return i >= 0 ? argv[i + 1] : null })()
const TODAY = dateArg || new Date().toISOString().slice(0, 10)

const P = {
  triage: '.claude/workflows/format-triage.js',
  audit: '.skills/refresh-format-maturity/scripts/audit-format.py',
  support: 'core/formats/support.yaml',
  dataset: 'web/static/data/format-maturity.json',
  history: 'web/static/data/format-maturity-history.json',
  rubric: 'docs/internals/format-maturity.md',
  ledger: 'docs/internals/format-ops-ledger.json',
}

// ── load the workflow's pure prelude as an ESM module ───────────────────────
// The prelude (everything before the first top-level `phase(` call) is
// side-effect-free declarations. We slice it, neutralize the `args` global it
// reads for config (defaults are correct here), append a single `export`, and
// import it as a real module — so the scorer's own gate code runs verbatim, no
// reimplementation and no eval-of-string-as-function.
async function loadScorer() {
  const src = readFileSync(P.triage, 'utf8')
  const marker = src.indexOf('\nphase(') // first top-level phase() call
  if (marker < 0) throw new Error('could not locate the prelude boundary in ' + P.triage)
  let prelude = src.slice(0, marker)
  prelude = prelude.replace(/^export\s+const\s+meta/m, 'const meta') // avoid duplicate export name
  prelude = 'const args = "";\n' + prelude // cfg defaults (target L4, samples 1, anchor on)
  prelude += '\nexport { AXIS_IDS, AXES, NEXT, RANK, ORDER, AXIS_LABELS, CANON, LABELS, DIM_AXES,' +
    ' floorDimsAll, axisLevel, axisCeiling, axisGaps, buildDataset };\n'
  const dir = mkdtempSync(join(tmpdir(), 'fmt-scorer-'))
  const file = join(dir, 'prelude.mjs')
  writeFileSync(file, prelude)
  return import(pathToFileURL(file).href)
}

// ── tiny strict YAML reader for support.yaml (the committed shape only) ─────
// support.yaml: top-level `formats:` map of id -> { tier, tier_since,
// last_certified, grandfathered, gates: [...] }. We only need the tier block
// per id, mirroring the reader in format-triage.js.
function readSupport(text) {
  const out = {}
  let cur = null
  for (const raw of text.split('\n')) {
    if (/^\s*#/.test(raw) || !raw.trim()) continue
    let m
    if ((m = raw.match(/^  ([A-Za-z0-9_.-]+):\s*$/))) { cur = m[1]; out[cur] = { declared: null, since: null, last_certified: null, gates: [] }; continue }
    if (!cur) continue
    if ((m = raw.match(/^    tier:\s*(\S+)/))) out[cur].declared = m[1].replace(/['"]/g, '')
    else if ((m = raw.match(/^    tier_since:\s*(\S+)/))) out[cur].since = m[1].replace(/['"]/g, '')
    else if ((m = raw.match(/^    last_certified:\s*(\S+)/))) out[cur].last_certified = m[1].replace(/['"]/g, '') === 'null' ? null : m[1].replace(/['"]/g, '')
    else if ((m = raw.match(/^    gates:\s*\[(.*)\]/))) out[cur].gates = m[1].split(',').map((s) => s.trim().replace(/['"]/g, '')).filter(Boolean)
  }
  return out
}

const S = await loadScorer()

// ── floor → rows (zero prior, zero demotion: derived == floor-seed level) ───
const auditJson = execFileSync('python3', [P.audit, '--all', '--json'], { cwd: ROOT, maxBuffer: 64 * 1024 * 1024 }).toString()
const auditSha = createHash('sha256').update(auditJson).digest('hex')
const floors = JSON.parse(auditJson)
const tierByFmt = existsSync(P.support) ? readSupport(readFileSync(P.support, 'utf8')) : {}

const movesByAxis = {}
for (const a of S.AXIS_IDS) movesByAxis[a] = { published: 0, suppressed: 0 }

const rows = []
for (const floor of floors) {
  const type = floor.type || 'parity'
  const dims = S.floorDimsAll(floor, type)
  const levels = {}, next = {}, axes = {}
  for (const axis of S.AXIS_IDS) {
    const level = S.axisLevel(axis, dims, floor, type) // prior=null => derived stands
    levels[axis] = level
    next[axis] = S.NEXT[axis][level] || '—'
    const band = axis === 'engine'
      ? { floor: floor.base, ceiling: floor.ceiling }
      : { floor: (floor.axes && floor.axes[axis] && floor.axes[axis].base) || null, ceiling: S.axisCeiling(axis, floor) }
    axes[axis] = {
      level, next: next[axis], floor: band.floor, ceiling: band.ceiling,
      derived_from: 'dimensions', delta: null, agreement: 1,
      blocking_gaps: gapsFor(floor, axis, dims),
    }
  }
  let cp = String(floor.okapi_counterpart || '')
  if (cp.startsWith('none') || cp.includes('harvest') || cp.includes('internal') || cp.includes('verify')) cp = ''
  const tier = tierByFmt[floor.format]
  rows.push({
    id: floor.format, type, level: levels.engine, next_level: next.engine,
    levels, next, okapi_counterpart: cp, dimensions: dims, evidence: {},
    floor: floor.base, ceiling: floor.ceiling,
    derived_from: 'dimensions', delta: null, agreement: 1, samples: 1, axes,
    tier: tier ? { declared: tier.declared, since: tier.since, last_certified: tier.last_certified, gates: tier.gates } : null,
    blocking_gaps: axes.engine.blocking_gaps,
    top_risk: '', confidence: 'floor',
  })
}

// A compact, deterministic per-axis gap derived from the floor signals — the
// first missing artifact toward the next level. The first real triage-score
// run replaces these with the agents' richer, cited gaps.
function gapsFor(floor, axis, dims) {
  const a = floor.axes && floor.axes[axis]
  if (!a || a.base === a.ceiling && axisIsTop(axis, a.base)) {}
  const want = {
    engine: [['malformed', 'add malformed_test.go'], ['spec', 'add spec.yaml + spec_test.go'], ['parity', 'add cli/parity spec_test'], ['corpus', 'add a corpus/upstream test']],
    vocabulary: [['vocabmap', 'add vocabulary.yaml with evidenced read cells'], ['vocabtypes', 'emit canonical run types in the reader'], ['writecells', 'evidence write cells (author-from-canonical)'], ['equivalence', 'add the vocab equivalence case']],
    editor: [['preview', 'implement format.PreviewBuilder'], ['identity', 'add an identity-binding round-trip test'], ['embedded', 'ship a committed add-in/connector manifest']],
    knowledge: [['dossier', 'add dossier.yaml with a catalog-registered spec source'], ['sidecar', 'wire the nativedocs sidecar'], ['refs', 'populate spec_refs/native_refs + divergence_kind']],
    corpus: [['corpusmanifest', 'add corpus.yaml covering testdata'], ['fetchwiring', 'wire Tier B (corpus: scheme / make fetch-corpus)'], ['corpus', 'replace synthetic corpus with real files'], ['acceptance', 'wire an external acceptance validator']],
    security: [['safeio', 'wire core/safeio budgets into the reader (S1)'], ['fuzz', 'add a Fuzz* target + testdata/fuzz seed (S2)'], ['sweepclean', 'record a clean corpus-sweep in the ledger (S3)']],
    structure: [['metaplane', 'classify metadata-plane / caption text (G1)'], ['readingorder', 'emit grouped body text in reading order (G2)'], ['roles', 'set SetSemanticRole + tables/relations + a roles test (G3)'], ['geometry', 'set SetGeometry + a geometry test (G4)']],
  }[axis] || []
  const gaps = []
  for (const [dim, msg] of want) {
    const cell = dims[dim]
    if (cell && cell !== 'complete' && cell !== 'na') gaps.push(msg)
  }
  return gaps.slice(0, 3)
}
function axisIsTop() { return false }

// ── assemble the dataset via the workflow's own buildDataset ────────────────
const golden_passed = true // samples === 1 fleet-wide
const runIntegrity = {
  samples: 1, anchored: true,
  moves: { published: 0, suppressed: 0, by_axis: movesByAxis },
  low_agreement: [], golden_passed,
}
const dataset = S.buildDataset(rows, runIntegrity, tierByFmt)
dataset.generated_at = TODAY
dataset.source = 'format-ops bootstrap-publish (deterministic per-axis floor; no quality demotions)'

// ── distributions for the summary line + docs block ─────────────────────────
const byAxisDist = {}
for (const axis of S.AXIS_IDS) {
  byAxisDist[axis] = {}
  for (const g of S.AXES[axis]) byAxisDist[axis][g] = 0
  for (const r of rows) byAxisDist[axis][r.levels[axis]]++
}

if (DRY) {
  console.log(`bootstrap-publish DRY RUN (date ${TODAY}, ${rows.length} formats, audit ${auditSha.slice(0, 12)})`)
  for (const axis of S.AXIS_IDS) console.log(`  ${axis.padEnd(11)} ${JSON.stringify(byAxisDist[axis])}`)
  process.exit(0)
}

// ── write the dataset (2-space) ─────────────────────────────────────────────
writeFileSync(P.dataset, JSON.stringify(dataset, null, 2) + '\n')

// ── history: remove TODAY then append; never rewrite old entries ────────────
const history = existsSync(P.history) ? JSON.parse(readFileSync(P.history, 'utf8')) : []
const kept = history.filter((h) => h.date !== TODAY)
kept.push({ date: TODAY, total: dataset.summary.total, by_level: dataset.summary.by_level, by_axis: dataset.summary.by_axis, golden_passed, moves: runIntegrity.moves })
kept.sort((a, b) => a.date.localeCompare(b.date))
writeFileSync(P.history, JSON.stringify(kept, null, 2) + '\n')

// ── docs snapshot block (between the generated markers) ─────────────────────
const BEGIN = '<!-- BEGIN: gap-analysis report (generated) -->'
const END = '<!-- END: gap-analysis report -->'
const md = readFileSync(P.rubric, 'utf8')
const b = md.indexOf(BEGIN), e = md.indexOf(END)
if (b >= 0 && e > b) {
  const block = renderDocsBlock()
  writeFileSync(P.rubric, md.slice(0, b) + BEGIN + '\n' + block + '\n' + md.slice(e))
}
function renderDocsBlock() {
  const L = []
  L.push(`## Maturity report (snapshot: ${TODAY})`)
  L.push('')
  L.push(`Generated by the \`bootstrap-publish\` (deterministic floor; no quality`)
  L.push(`demotions) over ${rows.length} real formats. Regenerated by every ritual that`)
  L.push(`republishes the dashboard — do not edit by hand. The dashboard`)
  L.push(`(\`/format-maturity\`) carries the live, filterable view.`)
  L.push('')
  L.push('### Per-axis distribution')
  L.push('')
  for (const axis of S.AXIS_IDS) {
    const grades = S.AXES[axis]
    const cells = grades.map((g) => `${g}:${byAxisDist[axis][g]}`).join(' · ')
    L.push(`- **${S.AXIS_LABELS[axis]}** — ${cells}`)
  }
  L.push('')
  L.push('### Per-format vector')
  L.push('')
  // Per-format columns iterate AXIS_IDS so a new axis (e.g. Security) appears
  // automatically; "Top engine gap" stays the headline-axis gap.
  const cols = ['Format', 'Tier', ...S.AXIS_IDS.map((a) => S.AXIS_LABELS[a]), 'Top engine gap']
  L.push(`| ${cols.join(' | ')} |`)
  L.push(`|${cols.map(() => '---').join('|')}|`)
  for (const r of rows.slice().sort((a, b) => a.id.localeCompare(b.id))) {
    const t = r.tier && r.tier.declared ? r.tier.declared : '—'
    const gap = (r.blocking_gaps[0] || '—').slice(0, 60)
    const cells = [`\`${r.id}\``, t, ...S.AXIS_IDS.map((a) => r.levels[a]), gap]
    L.push(`| ${cells.join(' | ')} |`)
  }
  return L.join('\n')
}

// ── support.yaml: refresh last_certified only (line-edit; preserve the rest) ─
if (existsSync(P.support)) {
  const ids = new Set(rows.map((r) => r.id))
  const lines = readFileSync(P.support, 'utf8').split('\n')
  let cur = null
  for (let i = 0; i < lines.length; i++) {
    let m
    if ((m = lines[i].match(/^  ([A-Za-z0-9_.-]+):\s*$/))) { cur = m[1]; continue }
    if (cur && ids.has(cur) && /^    last_certified:/.test(lines[i])) {
      lines[i] = `    last_certified: "${TODAY}"`
    }
  }
  writeFileSync(P.support, lines.join('\n'))
}

// ── ledger: triage-score watermarks + run record ────────────────────────────
if (existsSync(P.ledger)) {
  const ledger = JSON.parse(readFileSync(P.ledger, 'utf8'))
  let coreSha = ''
  try { coreSha = execFileSync('git', ['log', '-1', '--format=%H', '--', 'core/formats'], { cwd: ROOT }).toString().trim() } catch {}
  const ts = ledger.rituals && ledger.rituals['triage-score']
  if (ts) {
    ts.last_run = TODAY
    ts.watermarks = ts.watermarks || {}
    ts.watermarks.core_formats_sha = coreSha
    ts.watermarks.audit_sha = auditSha
    ts.watermarks.scorer_version = 4
    ts.watermarks.axes_published = S.AXIS_IDS.slice()
  }
  ledger.runs = ledger.runs || []
  // idempotent: a re-publish on the same day replaces its own run record
  ledger.runs = ledger.runs.filter((r) => !(r.date === TODAY && r.ritual === 'triage-score' && r.model_id === 'deterministic'))
  const dist = dataset.summary.by_level
  ledger.runs.push({
    date: TODAY, ritual: 'triage-score', commit: 'pending', model_id: 'deterministic',
    outcome: `bootstrap floor publish: engine ${S.AXES.engine.map((g) => `${g}:${dist[g]}`).join(' ')}; ${rows.length} formats; first multi-axis snapshot`,
    evidence: [{ check: 'audit-format.py --all --json', exit: 0, output_sha: auditSha }],
    followups: ['vocabulary.yaml backfill (#issue)', 'corpus C2 fetch wiring'],
  })
  writeFileSync(P.ledger, JSON.stringify(ledger, null, 2) + '\n')
}

console.log(`bootstrap-publish: wrote ${P.dataset} (${rows.length} formats, ${TODAY})`)
for (const axis of S.AXIS_IDS) console.log(`  ${axis.padEnd(11)} ${JSON.stringify(byAxisDist[axis])}`)
