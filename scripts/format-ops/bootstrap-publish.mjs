#!/usr/bin/env node
// Publish the format axes from the repository audit and the Prose probe.
// The scorer retains cited quality judgments from the last review. A tier review
// records passing certification gates. See docs/internals/format-maturity.md.
// Use --dry-run to inspect grades, --date to pin the date and --check to verify
// the published measurements without changing files.

import { execFileSync } from 'node:child_process'
import { readFileSync, writeFileSync, existsSync, mkdtempSync, rmSync, renameSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { assertTierMinimum, formatTreeDigest, nextAxisGaps } from './lib.mjs'

const ROOT = process.cwd()
const argv = process.argv.slice(2)
const DRY = argv.includes('--dry-run')
const CHECK = argv.includes('--check')
const temp = mkdtempSync(join(tmpdir(), 'format-publish-'))
process.on('exit', () => rmSync(temp, { recursive: true, force: true }))
const dateArg = (() => { const i = argv.indexOf('--date'); return i >= 0 ? argv[i + 1] : null })()
const TODAY = dateArg || new Date().toISOString().slice(0, 10)
if (!/^\d{4}-\d{2}-\d{2}$/.test(TODAY) || new Date(TODAY).toISOString().slice(0, 10) !== TODAY) throw new Error('invalid snapshot date')

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
    ' floorDimsAll, reconcileDims, applySticky, axisLevel, axisCeiling, axisGaps, buildDataset, attachProse, proseDetail, languageRows };\n'
  const file = join(temp, 'prelude.mjs')
  writeFileSync(file, prelude)
  return import(pathToFileURL(file).href)
}

const S = await loadScorer()

// Score repository evidence against the same gates used for a reviewed publication.
const auditJson = execFileSync('python3', [P.audit, '--all', '--json', '--repository-only', '--ledger', P.ledger], { cwd: ROOT, maxBuffer: 64 * 1024 * 1024 }).toString()
const auditSha = createHash('sha256').update(auditJson).digest('hex')
const floors = JSON.parse(auditJson)
const readYaml = 'import json,sys,yaml; print(json.dumps(yaml.safe_load(sys.stdin), default=str))'
const declarations = JSON.parse(execFileSync('python3', ['-c', readYaml], { input: readFileSync(P.support) })).formats
const tierByFmt = Object.fromEntries(Object.entries(declarations).map(([id, t]) => [id, {
  declared: t.tier, since: t.tier_since, last_certified: t.last_certified ?? null, gates: t.gates,
}]))
if (Object.keys(tierByFmt).sort().join() !== floors.map(f => f.format).sort().join()) {
  throw new Error('The support declarations and the format inventory differ.')
}
const prior = JSON.parse(readFileSync(P.dataset, 'utf8'))
const priorByID = new Map(prior.formats.map(row => [row.id, row]))
if (TODAY < prior.generated_at) throw new Error('The snapshot predates the current publication.')

// ── the Prose probe (rubric §2.8) ───────────────────────────────────────────
// The Prose floor is the probe's report rather than an audit grep: a rung
// counts only when its rung test passed. A run whose canary is not scored as
// built exits non-zero and writes no report, and this publish stops here,
// before any file is written.
const proseOut = join(temp, 'report.json')
try {
  execFileSync('make', ['--no-print-directory', 'prose-probe', `PROSE_PROBE_OUT=${proseOut}`],
    { cwd: ROOT, stdio: ['ignore', 'ignore', 'inherit'] })
} catch {
  console.error('bootstrap-publish: the Prose probe failed or its run was invalid; nothing published')
  process.exit(1)
}
const proseReport = JSON.parse(readFileSync(proseOut, 'utf8'))
S.attachProse(floors, proseReport)
const languages = S.languageRows(proseReport)

const movesByAxis = {}
for (const a of S.AXIS_IDS) movesByAxis[a] = { published: 0, suppressed: 0 }

const rows = []
for (const floor of floors) {
  const type = floor.type || 'parity'
  const previous = priorByID.get(floor.format)
  const review = previous?.confidence && previous.confidence !== 'floor'
    ? { date: prior.generated_at, levels: previous.levels, dims: previous.dimensions,
        evidence: previous.evidence || {}, confidence: previous.confidence, risk: previous.top_risk }
    : previous?.last_review
  const floorDims = S.floorDimsAll(floor, type)
  const dims = review ? S.reconcileDims(review, floorDims) : floorDims
  const levels = {}, measured = {}, next = {}, axes = {}
  for (const axis of S.AXIS_IDS) {
    const derived = S.axisLevel(axis, dims, floor, type)
    measured[axis] = derived
    const { level, delta, derived_from } = S.applySticky(axis, review?.levels?.[axis], derived, '', S.axisCeiling(axis, floor))
    if (delta) movesByAxis[axis][derived_from === 'sticky-prior' ? 'suppressed' : 'published']++
    levels[axis] = level
    next[axis] = axis === 'engine' && type === 'harvest' && level === 'L3' ? '—' : (S.NEXT[axis][level] || '—')
    const band = axis === 'engine'
      ? { floor: floor.base, ceiling: floor.ceiling }
      : { floor: (floor.axes && floor.axes[axis] && floor.axes[axis].base) || null, ceiling: S.axisCeiling(axis, floor) }
    axes[axis] = {
      level, next: next[axis], floor: band.floor, ceiling: band.ceiling,
      derived_from, delta,
      blocking_gaps: axis === 'prose' ? proseGaps(floor) : nextAxisGaps(floor, axis, level, dims),
      ...(axis === 'prose' ? S.proseDetail(floor) : {}),
    }
  }
  assertTierMinimum(floor.format, declarations[floor.format], measured)
  let cp = String(floor.okapi_counterpart || '')
  if (cp.startsWith('none') || cp.includes('harvest') || cp.includes('internal') || cp.includes('verify') || cp.startsWith('unknown')) cp = ''
  const tier = tierByFmt[floor.format]
  rows.push({
    id: floor.format, type, level: levels.engine, next_level: next.engine,
    levels, next, okapi_counterpart: cp, dimensions: dims, evidence: review?.evidence || {},
    ...(review ? { last_review: review } : {}),
    floor: floor.base, ceiling: floor.ceiling,
    derived_from: axes.engine.derived_from, delta: axes.engine.delta, samples: 0, axes,
    tier: tier ? { declared: tier.declared, since: tier.since, last_certified: tier.last_certified, gates: tier.gates } : null,
    blocking_gaps: axes.engine.blocking_gaps,
    top_risk: review?.risk || '', confidence: 'floor',
  })
}

// proseGaps names the first rung that is not met. A rung test that exists and
// did not run is reported as that, with its reason, rather than as missing.
function proseGaps(floor) {
  const sig = floor.axes && floor.axes.prose && floor.axes.prose.signals
  if (!sig) return []
  // A format that reads several languages is scored on their rows, so the
  // gap points there rather than at a test named for the format.
  if (sig.languages && sig.languages.length) return [`scored on the language rows: ${sig.languages.join(', ')}`]
  for (const rung of ['P1', 'P2', 'P3', 'P4']) {
    const r = sig.rungs[rung]
    if (!r || r.outcome === 'met') continue
    const test = `TestProse${rung}_${floor.format}`
    if (r.outcome === 'did-not-run') return [`${test} did not run (${r.reason})`]
    if (r.reason === 'no test') return [`add ${test} (${rung})`]
    return [`${test} ${r.reason}`]
  }
  return []
}

// ── assemble the dataset via the workflow's own buildDataset ────────────────
const runIntegrity = {
  samples: 0, anchored: rows.some(r => r.last_review),
  moves: { published: Object.values(movesByAxis).reduce((n, v) => n + v.published, 0),
    suppressed: Object.values(movesByAxis).reduce((n, v) => n + v.suppressed, 0), by_axis: movesByAxis },
  low_agreement: [],
  prose: { probe_version: proseReport.probe_version, tags: proseReport.tags,
    canaries: proseReport.canaries.map((c) => ({ id: c.id, presence: c.presence, level: c.level || null })) },
}
const dataset = S.buildDataset(rows, runIntegrity, tierByFmt, languages)
dataset.generated_at = TODAY
dataset.source = 'Repository audit and Prose probe with prior quality reviews retained'

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
  console.log(`  languages   ${JSON.stringify(dataset.summary.languages)}`)
  process.exit(0)
}

// ── write the dataset (2-space) ─────────────────────────────────────────────
const projections = new Map([[P.dataset, JSON.stringify(dataset, null, 2) + '\n']])

// ── history: remove TODAY then append; never rewrite old entries ────────────
const history = existsSync(P.history) ? JSON.parse(readFileSync(P.history, 'utf8')) : []
const kept = history.filter((h) => h.date !== TODAY)
kept.push({ date: TODAY, total: dataset.summary.total, by_level: dataset.summary.by_level, by_axis: dataset.summary.by_axis, languages: dataset.summary.languages, moves: runIntegrity.moves })
kept.sort((a, b) => a.date.localeCompare(b.date))
projections.set(P.history, JSON.stringify(kept, null, 2) + '\n')

// ── docs snapshot block (between the generated markers) ─────────────────────
const BEGIN = '<!-- BEGIN: gap-analysis report (generated) -->'
const END = '<!-- END: gap-analysis report -->'
const md = readFileSync(P.rubric, 'utf8')
const b = md.indexOf(BEGIN), e = md.indexOf(END)
if (b < 0 || e <= b) throw new Error('Missing format maturity report markers in ' + P.rubric)
{
  const block = renderDocsBlock()
  projections.set(P.rubric, md.slice(0, b) + BEGIN + '\n' + block + '\n' + md.slice(e))
}
function renderDocsBlock() {
  const L = []
  L.push(`## Maturity report (snapshot: ${TODAY})`)
  L.push('')
  L.push('The repository audit measures the format axes. The Prose probe runs the rung tests.')
  L.push('The [dashboard](/format-maturity) shows the measurements and support declarations.')
  L.push('')
  L.push('### Per-axis distribution')
  L.push('')
  for (const axis of S.AXIS_IDS) {
    const grades = S.AXES[axis]
    const cells = grades.map((g) => `${g}:${byAxisDist[axis][g]}`).join(' · ')
    L.push(`- **${S.AXIS_LABELS[axis]}**: ${cells}`)
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
    const t = r.tier && r.tier.declared ? r.tier.declared : 'unlisted'
    const gap = (r.blocking_gaps[0] || 'none').slice(0, 60)
    const cells = [`\`${r.id}\``, t, ...S.AXIS_IDS.map((a) => r.levels[a]), gap]
    L.push(`| ${cells.join(' | ')} |`)
  }
  // Languages carry only the Prose axis, so they get their own table. A
  // language with no level is absent from the build, or its presence test did
  // not run.
  L.push('')
  L.push('### Languages (Prose axis)')
  L.push('')
  L.push('| Language | Presence | Prose | P1 | P2 | P3 | P4 | Presence reason |')
  L.push('|---|---|---|---|---|---|---|---|')
  for (const l of languages) {
    const rungs = ['P1', 'P2', 'P3', 'P4'].map((g) => l.rungs[g].outcome)
    const reason = (l.presence_reason || '').split('\n')[0].replace(/\|/g, '/')
    L.push(`| \`${l.id}\` | ${l.presence} | ${l.level || 'none'} | ${rungs.join(' | ')} | ${reason} |`)
  }
  return L.join('\n')
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
    if (!CHECK || !ts.watermarks.core_formats_sha) ts.watermarks.core_formats_sha = coreSha
    ts.watermarks.format_tree_sha256 = formatTreeDigest(ROOT)
    ts.watermarks.audit_sha = auditSha
    ts.watermarks.scorer_version = 5
    ts.watermarks.axes_published = S.AXIS_IDS.slice()
  }
  ledger.runs = ledger.runs || []
  // idempotent: a re-publish on the same day replaces its own run record
  ledger.runs = ledger.runs.filter((r) => !(r.date === TODAY && r.ritual === 'triage-score' && r.model_id === 'deterministic'))
  ledger.runs.push({
    date: TODAY, ritual: 'triage-score', commit: 'pending', model_id: 'deterministic',
    // No "first snapshot" claim: this path re-runs, so the outcome names the
    // scoring MODE (deterministic floor, no LLM scoring) rather than asserting
    // a novelty that is false on every run after the first.
    outcome: 'Published the repository audit and Prose probe. The format dashboard records the measured grades.',
    evidence: [{ check: 'audit-format.py --all --json --repository-only --ledger ' + P.ledger, exit: 0, output_sha: auditSha },
      { check: 'make prose-probe', exit: 0, output_sha: createHash('sha256').update(JSON.stringify({ languages, rows: rows.map(r => r.axes.prose) })).digest('hex') }],
    followups: [],
  })
  projections.set(P.ledger, JSON.stringify(ledger, null, 2) + '\n')
}

const changed = [...projections].filter(([file, bytes]) => readFileSync(file, 'utf8') !== bytes)
if (CHECK && changed.length) throw new Error('Stale maturity projections: ' + changed.map(([file]) => file).join(', '))
for (const [file, bytes] of changed) {
  const stage = file + '.tmp-' + process.pid
  writeFileSync(stage, bytes)
  renameSync(stage, file)
}
const indexPath = 'web/src/pages/evals/_index.json'
const indexStage = join(temp, 'index.json')
execFileSync('go', ['run', './scripts/evalindex', '-out', indexStage], { cwd: ROOT, stdio: 'inherit' })
const index = readFileSync(indexStage, 'utf8')
if (readFileSync(indexPath, 'utf8') !== index) {
  if (CHECK) throw new Error('Stale evaluation index: ' + indexPath)
  writeFileSync(indexPath, index)
}
console.log(`bootstrap-publish: ${CHECK ? 'checked' : 'published'} ${rows.length} formats on ${TODAY}`)
