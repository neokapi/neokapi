#!/usr/bin/env node
// Deterministic reproducibility check for the format-triage scorer.
//
// The scorer (.claude/workflows/format-triage.js) computes a format's level from
// the deterministic file floor (audit-format.py --json) plus AT MOST three
// model-judged quality dimensions (writer byte-equality, parity xfail hygiene,
// corpus real-vs-synthetic). This harness enumerates EVERY realistic value of
// those three dims for each format and reports the resulting level spread — i.e.
// the maximum the model can move the published level. Spread 0 = fully pinned by
// files; spread 1 = a single L3<->L4 (or L2<->L3) boundary the evidence gate +
// sticky anchor then settle. The old free-level scorer, by contrast, was
// observed swinging L1<->L2<->L3 (spread 2) on csv/openxml/tmx across runs.
//
// Usage: python3 audit-format.py --all --json | node repro-check.mjs [fmt ...]
//
// The four scoring functions below MIRROR format-triage.js (kept in sync by
// hand; this is the verification companion, the workflow is the source of truth).

const RANK = { L0: 0, L1: 1, L2: 2, L3: 3, L4: 4 }
const CANON = ['reader', 'writer', 'config', 'spec', 'parity', 'malformed', 'corpus', 'detection', 'docs']
const QUALITY = ['writer', 'parity', 'corpus']

function minL(a, b) { return RANK[a] <= RANK[b] ? a : b }

function dimsFromFloor(floor, type) {
  const has = floor.has || {}
  const kinds = floor.test_kinds || []
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

function gateLevel(dims, type) {
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

function capByFloor(level, floor) {
  const has = floor.has || {}
  let lvl = level
  if (has.reader === false) lvl = 'L0'
  if (has.writer === false && floor.type !== 'read-only') lvl = minL(lvl, 'L1')
  if (RANK[lvl] > RANK[floor.ceiling]) lvl = floor.ceiling
  return lvl
}

// realistic value set the model can assign to a quality dim given its floor value
function realistic(floorVal) {
  if (floorVal === 'complete') return ['complete', 'partial'] // demote on a cited quality miss
  if (floorVal === 'partial') return ['partial', 'none']
  return [floorVal] // none / na are fixed by files
}

function levelSpread(floor) {
  const type = floor.type
  const base = dimsFromFloor(floor, type)
  const sets = QUALITY.map((q) => realistic(base[q]))
  const levels = new Set()
  for (const w of sets[0]) for (const p of sets[1]) for (const co of sets[2]) {
    const dims = { ...base, writer: w, parity: p, corpus: co }
    levels.add(capByFloor(gateLevel(dims, type), floor))
  }
  return [...levels].sort((a, b) => RANK[a] - RANK[b])
}

// ── main ──
const want = process.argv.slice(2)
let raw = ''
process.stdin.setEncoding('utf8')
process.stdin.on('data', (d) => { raw += d })
process.stdin.on('end', () => {
  const all = JSON.parse(raw)
  const rows = all.filter((f) => !want.length || want.includes(f.format))
  let pinned = 0, boundary = 0, wide = 0
  console.log('format         type       floor  level-range (over all realistic quality dims)   spread')
  console.log('-------------  ---------  -----  ----------------------------------------------  ------')
  for (const f of rows) {
    const sp = levelSpread(f)
    const n = sp.length - 1
    if (n === 0) pinned++; else if (n === 1) boundary++; else wide++
    const flag = n === 0 ? 'PINNED' : n === 1 ? '1-step' : `${n}-STEP!`
    console.log(`${f.format.padEnd(13)}  ${f.type.padEnd(9)}  ${f.base.padEnd(5)}  ${sp.join(' | ').padEnd(46)}  ${flag}`)
  }
  console.log('-------------')
  console.log(`n=${rows.length}: pinned(spread 0)=${pinned}, single-boundary(1)=${boundary}, wide(>=2)=${wide}`)
  console.log('A wide (>=2) format is one the model could still swing across >1 tier; aim is zero.')
})
