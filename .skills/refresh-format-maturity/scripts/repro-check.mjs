#!/usr/bin/env node
// Deterministic reproducibility check for the format-triage scorer (v3, multi-axis).
//
// The scorer (.claude/workflows/format-triage.js) computes each format's level
// PER AXIS (engine L0-L4, vocabulary V0-V3, editor E0-E4, knowledge K0-K3,
// corpus C0-C3, security S0-S4, structure G0-G4) from the deterministic file
// floor (audit-format.py --json, additive `axes{}` block) plus the model-judged
// QUALITY dimensions of that axis: engine {writer, parity, corpus}, vocabulary
// {writecells}, knowledge {refs}, corpus {corpus — the cell SHARED with
// engine}, and editor {} security {} structure {} (no judged dims: spread 0 by
// construction, asserted below). This harness enumerates
// EVERY realistic value of each axis's quality dims and reports the resulting
// per-axis level spread — i.e. the maximum the model can move the published
// level. Spread 0 = fully pinned by files; spread 1 = a single boundary the
// evidence gate + sticky anchor then settle. Any >=2-STEP spread is a scoring
// leak: this script flags it and exits non-zero.
//
// Usage: python3 audit-format.py --all --json | node repro-check.mjs [fmt ...]
//
// Stdin contract: the audit JSON array. Legacy engine fields
// (format,type,has,test_kinds,base,ceiling) drive the engine fallback when the
// additive axes{} block is absent (then only the engine axis is checked).
//
// The scoring functions below MIRROR format-triage.js (kept in sync by hand;
// this is the verification companion, the workflow is the source of truth).

const AXES = {
  engine: ['L0', 'L1', 'L2', 'L3', 'L4'],
  vocabulary: ['V0', 'V1', 'V2', 'V3'],
  editor: ['E0', 'E1', 'E2', 'E3', 'E4'],
  knowledge: ['K0', 'K1', 'K2', 'K3'],
  corpus: ['C0', 'C1', 'C2', 'C3'],
  security: ['S0', 'S1', 'S2', 'S3', 'S4'],
  structure: ['G0', 'G1', 'G2', 'G3', 'G4'],
}
const AXIS_IDS = Object.keys(AXES)
const RANK = {}
for (const axis of AXIS_IDS) {
  RANK[axis] = {}
  AXES[axis].forEach((g, i) => { RANK[axis][g] = i })
}
const QUALITY = {
  engine: ['writer', 'parity', 'corpus'],
  vocabulary: ['writecells'],
  editor: [], // probe-pinned: no judged dims (spread 0 by construction)
  knowledge: ['refs'],
  corpus: ['corpus'], // SHARED with engine: one judgment, both gates
  security: [], // floor-only (file + ledger signals): spread 0 by construction
  structure: [], // floor-only (deterministic file greps): spread 0 by construction
}

function minG(axis, a, b) { return RANK[axis][a] <= RANK[axis][b] ? a : b }

// ── per-axis floor cells (mirror of format-triage.js) ──

function legacyEngineDims(floor, type) {
  const has = (floor && floor.has) || {}
  const kinds = (floor && floor.test_kinds) || []
  const k = (n) => kinds.includes(n)
  return {
    reader: has.reader ? 'complete' : 'none',
    writer: has.writer ? 'complete' : (type === 'read-only' ? 'na' : 'none'),
    config: has.config ? 'complete' : 'none',
    spec: type === 'harvest' ? 'na' : (has.spec_yaml && k('spec') ? 'complete' : (has.spec_yaml ? 'partial' : 'none')),
    parity: type === 'harvest' ? 'na' : (has.parity_spec_test ? 'complete' : 'none'),
    malformed: k('malformed') ? 'complete' : 'none',
    corpus: (k('corpus') || k('upstream')) ? 'complete' : (has.testdata ? 'partial' : 'none'),
    detection: 'complete',
    docs: 'complete',
  }
}
const ENGINE_DIMS = ['reader', 'writer', 'config', 'spec', 'parity', 'malformed', 'corpus', 'detection', 'docs']
function engineDims(floor, type) {
  const sig = floor && floor.axes && floor.axes.engine && floor.axes.engine.signals
  if (!sig) return legacyEngineDims(floor, type)
  const out = {}
  for (const k of ENGINE_DIMS) out[k] = sig[k] || 'none'
  return out
}

function vocabularyDims(floor, type) {
  const sig = floor && floor.axes && floor.axes.vocabulary && floor.axes.vocabulary.signals
  if (!sig) {
    return { vocabmap: 'none', vocabtypes: 'none', writecells: type === 'read-only' ? 'na' : 'none', equivalence: 'none' }
  }
  const cells = sig.cells || {}
  const present = sig.vocabmap === 'present'
  const vocabmap = !present ? 'none' : ((cells.unknown_read || 0) === 0 ? 'complete' : 'partial')
  const writecells = type === 'read-only' ? 'na'
    : !present ? 'none'
      : ((cells.write_claimed || 0) > 0 && (cells.unknown_write || 0) === 0) ? 'complete'
        : (cells.write_claimed || 0) > 0 ? 'partial' : 'none'
  return {
    vocabmap,
    vocabtypes: sig.vocabtypes ? 'complete' : 'none',
    writecells,
    equivalence: sig.equivalence ? 'complete' : 'none',
  }
}

function editorDims(floor) {
  const p = (floor && floor.axes && floor.axes.editor && floor.axes.editor.signals
    && floor.axes.editor.signals.probes) || {}
  return {
    preview: p.preview ? 'complete' : 'none',
    identity: p.roundtrip_test ? 'complete' : 'none',
    embedded: p.manifest ? 'complete' : 'none',
    events: p.handler ? 'complete' : 'none',
  }
}

function knowledgeDims(floor, type) {
  const sig = floor && floor.axes && floor.axes.knowledge && floor.axes.knowledge.signals
  if (!sig) return { dossier: 'none', sidecar: 'none', refs: 'none', citations: 'none', contextpack: 'none' }
  const refs = sig.refs || {}
  const kinds = (floor && floor.test_kinds) || []
  const hasSpecYaml = !!(floor && floor.has && floor.has.spec_yaml)
  const refsOk = (hasSpecYaml
    && (refs.spec_refs || 0) > 0 && (refs.native_refs || 0) > 0
    && ((refs.okapi_refs || 0) > 0 || type !== 'parity')
    && (refs.divergence_coverage == null || refs.divergence_coverage === 1))
    || (type === 'harvest' && kinds.includes('okapi_skip') && kinds.includes('invariants') && kinds.includes('corpus'))
  const anyRefs = (refs.spec_refs || 0) + (refs.okapi_refs || 0) + (refs.native_refs || 0) > 0
  return {
    dossier: sig.dossier === 'present'
      ? (((sig.spec_sources || {}).valid || 0) > 0 ? 'complete' : 'partial') : 'none',
    sidecar: sig.sidecar === 'ok' ? 'complete' : (sig.sidecar === 'template' ? 'partial' : 'none'),
    refs: refsOk ? 'complete' : ((hasSpecYaml || anyRefs) ? 'partial' : 'none'),
    citations: sig.citations === 'green' ? 'complete' : 'none',
    contextpack: sig.contextpack === 'green' ? 'complete' : 'none',
  }
}

function sweepCell(v) {
  if (v == null || v === 'unknown') return 'none'
  if (typeof v === 'object') {
    const bad = ['CRASH', 'HANG', 'OOM', 'ROUNDTRIP_DRIFT']
    return bad.some((k) => (v[k] || 0) > 0) ? 'partial' : 'complete'
  }
  return 'partial'
}

function corpusDims(floor, engineCells) {
  const sig = floor && floor.axes && floor.axes.corpus && floor.axes.corpus.signals
  const shared = engineCells.corpus
  if (!sig) return { corpusmanifest: 'none', corpus: shared, fetchwiring: 'none', acceptance: 'none', sweep: 'none' }
  const census = sig.census || {}
  const fw = sig.fetchwiring || {}
  const corpusmanifest = sig.corpusmanifest !== 'present' ? 'none'
    : ((census.uncovered || 0) === 0 ? 'complete' : 'partial')
  const fetchwiring = fw.wired ? 'complete'
    : sig.countersigned_na ? 'na'
      : (fw.scheme_in_spec || fw.fetch_script) ? 'partial' : 'none'
  return {
    corpusmanifest,
    corpus: shared,
    fetchwiring,
    acceptance: ['complete', 'partial', 'none', 'na'].includes(sig.acceptance) ? sig.acceptance : 'none',
    sweep: sweepCell(sig.sweep),
  }
}

function securityDims(floor) {
  const cells = (floor && floor.axes && floor.axes.security && floor.axes.security.signals
    && floor.axes.security.signals.cells) || {}
  return {
    safeio: cells.safeio || 'none',
    fuzz: cells.fuzz || 'none',
    sweepclean: cells.sweepclean || 'none',
    sustained: cells.sustained || 'none',
  }
}

function structureDims(floor) {
  const cells = (floor && floor.axes && floor.axes.structure && floor.axes.structure.signals
    && floor.axes.structure.signals.cells) || {}
  return {
    metaplane: cells.metaplane || 'none',
    readingorder: cells.readingorder || 'none',
    roles: cells.roles || 'none',
    geometry: cells.geometry || 'none',
  }
}

function floorDimsFor(axis, floor, type) {
  if (axis === 'engine') return engineDims(floor, type)
  if (axis === 'vocabulary') return vocabularyDims(floor, type)
  if (axis === 'editor') return editorDims(floor)
  if (axis === 'knowledge') return knowledgeDims(floor, type)
  if (axis === 'security') return securityDims(floor)
  if (axis === 'structure') return structureDims(floor)
  return corpusDims(floor, engineDims(floor, type))
}

// ── per-axis gates (mirror of format-triage.js) ──

function gateEngine(dims, type) {
  const c = (x) => dims[x] || 'none'
  const has = (x) => c(x) !== 'none'
  const full = (x) => c(x) === 'complete' || c(x) === 'na'
  if (!(has('reader') && has('writer') && has('config'))) return 'L0'
  const specPath = type === 'harvest' ? full('corpus') : full('spec')
  if (!(full('malformed') && specPath)) return 'L1'
  if (type === 'harvest') {
    if (has('corpus') && full('docs')) return 'L3'
    return 'L2'
  }
  if (!(has('parity') && has('corpus') && full('docs'))) return 'L2'
  if (full('writer') && full('config') && full('parity') && full('corpus')) return 'L4'
  return 'L3'
}

function gateVocab(dims) {
  const c = (k) => dims[k] || 'none'
  const has = (k) => c(k) !== 'none'
  const full = (k) => c(k) === 'complete' || c(k) === 'na'
  if (!(full('vocabmap') && full('vocabtypes'))) return 'V0'
  if (!(has('writecells') && has('equivalence'))) return 'V1'
  if (full('writecells') && full('equivalence')) return 'V3'
  return 'V2'
}

function gateEditor(dims) {
  const full = (k) => (dims[k] || 'none') === 'complete' || (dims[k] || 'none') === 'na'
  if (!full('preview')) return 'E0'
  if (!full('identity')) return 'E1'
  if (!full('embedded')) return 'E2'
  if (!full('events')) return 'E3'
  return 'E4'
}

function gateKnowledge(dims, hasSchema) {
  const c = (k) => dims[k] || 'none'
  const full = (k) => c(k) === 'complete' || c(k) === 'na'
  if (!(full('dossier') && full('sidecar'))) return 'K0'
  if (!(full('refs') && hasSchema)) return 'K1'
  if (!(full('citations') && full('contextpack'))) return 'K2'
  return 'K3'
}

function gateCorpus(dims) {
  const c = (k) => dims[k] || 'none'
  const has = (k) => c(k) !== 'none'
  const full = (k) => c(k) === 'complete' || c(k) === 'na'
  if (!(full('corpusmanifest') && has('corpus'))) return 'C0'
  if (!full('fetchwiring')) return 'C1'
  if (!(full('corpus') && full('acceptance') && full('sweep'))) return 'C2'
  return 'C3'
}

function gateSecurity(dims) {
  const full = (k) => (dims[k] || 'none') === 'complete' || (dims[k] || 'none') === 'na'
  if (!full('safeio')) return 'S0'
  if (!full('fuzz')) return 'S1'
  if (!full('sweepclean')) return 'S2'
  if (!full('sustained')) return 'S3'
  return 'S4'
}

function gateStructure(dims) {
  const full = (k) => (dims[k] || 'none') === 'complete' || (dims[k] || 'none') === 'na'
  if (!full('metaplane')) return 'G0'
  if (!full('readingorder')) return 'G1'
  if (!full('roles')) return 'G2'
  if (!full('geometry')) return 'G3'
  return 'G4'
}

// ── caps (mirror of format-triage.js) ──

function capEngine(level, floor) {
  const has = floor.has || {}
  let lvl = level
  if (has.reader === false) lvl = 'L0'
  if (has.writer === false && floor.type !== 'read-only') lvl = minG('engine', lvl, 'L1')
  if (RANK.engine[lvl] > RANK.engine[floor.ceiling]) lvl = floor.ceiling
  return lvl
}
function axisCeiling(axis, floor) {
  if (!floor) return null
  if (axis === 'engine') return floor.ceiling != null ? floor.ceiling : null
  const ax = floor.axes && floor.axes[axis]
  return (ax && ax.ceiling != null && (ax.ceiling in RANK[axis])) ? ax.ceiling : null
}
function capAxis(axis, grade, floor) {
  if (axis === 'engine') return capEngine(grade, floor)
  const ceil = axisCeiling(axis, floor)
  if (ceil != null && RANK[axis][grade] > RANK[axis][ceil]) return ceil
  return grade
}

function axisLevel(axis, dims, floor, type) {
  let g
  if (axis === 'engine') g = gateEngine(dims, type)
  else if (axis === 'vocabulary') g = gateVocab(dims)
  else if (axis === 'editor') g = gateEditor(dims)
  else if (axis === 'knowledge') g = gateKnowledge(dims, !!(floor && floor.has && floor.has.schema))
  else if (axis === 'security') g = gateSecurity(dims)
  else if (axis === 'structure') g = gateStructure(dims)
  else g = gateCorpus(dims)
  return capAxis(axis, g, floor)
}

// realistic value set the model can assign to a quality dim given its floor value
function realistic(floorVal) {
  if (floorVal === 'complete') return ['complete', 'partial'] // demote on a cited quality miss
  if (floorVal === 'partial') return ['partial', 'none']
  return [floorVal] // none / na are fixed by files
}

function* cartesian(sets) {
  if (!sets.length) { yield []; return }
  const [head, ...tail] = sets
  for (const v of head) for (const rest of cartesian(tail)) yield [v, ...rest]
}

function axisSpread(axis, floor) {
  const type = floor.type
  const base = floorDimsFor(axis, floor, type)
  const qs = QUALITY[axis]
  const levels = new Set()
  for (const combo of cartesian(qs.map((q) => realistic(base[q])))) {
    const dims = { ...base }
    qs.forEach((q, i) => { dims[q] = combo[i] })
    levels.add(axisLevel(axis, dims, floor, type))
  }
  return [...levels].sort((a, b) => RANK[axis][a] - RANK[axis][b])
}

// ── main ──
const want = process.argv.slice(2)
let raw = ''
process.stdin.setEncoding('utf8')
process.stdin.on('data', (d) => { raw += d })
process.stdin.on('end', () => {
  const all = JSON.parse(raw)
  const rows = all.filter((f) => !want.length || want.includes(f.format))
  const totals = {}
  for (const axis of AXIS_IDS) totals[axis] = { pinned: 0, boundary: 0, wide: 0 }
  let leak = false
  const head = AXIS_IDS.map((a) => a.slice(0, 9).padEnd(18)).join('')
  console.log(`format         type       ${head}`)
  console.log(`-------------  ---------  ${AXIS_IDS.map(() => '-'.repeat(16).padEnd(18)).join('')}`)
  for (const f of rows) {
    const hasAxes = !!f.axes
    const cols = []
    for (const axis of AXIS_IDS) {
      if (!hasAxes && axis !== 'engine') { cols.push('—'.padEnd(18)); continue } // legacy engine-only fallback
      const sp = axisSpread(axis, f)
      const n = sp.length - 1
      if (n === 0) totals[axis].pinned++; else if (n === 1) totals[axis].boundary++; else totals[axis].wide++
      if ((axis === 'editor' || axis === 'security' || axis === 'structure') && n !== 0) {
        // editor + security + structure have no quality dims — any spread is a
        // mirror/gate bug, not judgment (floor-only ⇒ spread 0 by construction)
        console.error(`!! ${axis} axis spread for ${f.format}: ${sp.join('|')} (must be 0 by construction)`)
        leak = true
      }
      if (n >= 2) leak = true
      const flag = n === 0 ? 'PINNED' : n === 1 ? '1-step' : `${n}-STEP!`
      cols.push(`${sp.join('|')} ${flag}`.padEnd(18))
    }
    console.log(`${f.format.padEnd(13)}  ${String(f.type).padEnd(9)}  ${cols.join('')}`)
  }
  console.log('-------------')
  for (const axis of AXIS_IDS) {
    const t = totals[axis]
    console.log(`${axis.padEnd(10)} n=${t.pinned + t.boundary + t.wide}: pinned(spread 0)=${t.pinned}, single-boundary(1)=${t.boundary}, wide(>=2)=${t.wide}`)
  }
  console.log('A wide (>=2) axis spread is one the model could still swing across >1 tier; aim is zero.')
  if (leak) {
    console.error('LEAK: at least one axis has a >=2-step spread (or a non-zero editor spread).')
    process.exitCode = 1
  }
})
