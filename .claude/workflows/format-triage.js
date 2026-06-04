export const meta = {
  name: 'format-triage',
  description: 'Triage every neokapi format against the L0–L4 maturity rubric, rank the work to push each toward a target level, optionally remediate (add the top missing test/artifact, verified), and refresh the /format-maturity dashboard dataset',
  whenToUse: 'Run periodically to track and advance format maturity. Default (no args) = score + triage + publish the dashboard. args: {target:"L2|L3|L4", mode:"triage|remediate", formats:[ids], limit:N, publish:false, samples:N, anchor:true}',
  phases: [
    { title: 'Prep', detail: 'one agent: deterministic file-floor (audit-format.py --all --json) + prior dashboard levels' },
    { title: 'Score', detail: 'one agent per format (×samples) returns evidence-cited dimension cells — NOT a free level pick' },
    { title: 'Triage', detail: 'rank the formats below target and the highest-leverage gap for each' },
    { title: 'Remediate', detail: '(mode=remediate) add the top missing artifact per format and verify it' },
    { title: 'Publish', detail: 'write the refreshed dashboard dataset + a history snapshot' },
  ],
}

// ── config (all optional; defaults make a bare trigger do score+triage+publish) ──
function parseArgs(a) {
  if (a && typeof a === 'object' && !Array.isArray(a)) return a
  if (typeof a === 'string' && a.trim()) {
    try {
      const p = JSON.parse(a)
      if (p && typeof p === 'object' && !Array.isArray(p)) return p
    } catch (e) { /* fall through to defaults */ }
  }
  return {}
}
const cfg = parseArgs(args)
const TARGET = cfg.target || 'L4'
const MODE = cfg.mode || 'triage' // 'triage' | 'remediate'
const PUBLISH = cfg.publish !== false
const LIMIT = typeof cfg.limit === 'number' ? cfg.limit : 0 // 0 = no cap in remediate
const SAMPLES = Math.max(1, typeof cfg.samples === 'number' ? cfg.samples : 1) // ensemble size per format
const ANCHOR = cfg.anchor !== false // sticky to the prior committed level unless a cited delta

const ALL_FORMATS = [
  'androidxml', 'applestrings', 'arb', 'csv', 'designtokens', 'doxygen', 'dtd',
  'epub', 'fixedwidth', 'html', 'i18next', 'icml', 'idml', 'json', 'markdown',
  'mdx', 'messageformat', 'mif', 'mo', 'mosestext', 'odf', 'openxml',
  'paraplaintext', 'pdf', 'phpcontent', 'plaintext', 'po', 'properties', 'regex',
  'resx', 'rtf', 'splicedlines', 'srt', 'tex', 'tmx', 'transtable', 'ts', 'ttml',
  'ttx', 'txml', 'versifiedtext', 'vignette', 'vtt', 'wiki', 'xcstrings', 'xliff',
  'xliff2', 'xml', 'yaml',
]
const FORMATS = (Array.isArray(cfg.formats) && cfg.formats.length) ? cfg.formats : ALL_FORMATS

// The agents run with cwd = the neokapi repo root (the worktree this workflow is
// triggered from). Paths are repo-relative so the audit script + dashboard the
// agents read are the SAME tree the workflow edits — no cross-worktree drift.
const REPO = 'the neokapi repo root (your current working directory)'
const OKAPI = '/Users/asgeirf/src/okapi/Okapi/okapi/filters'
const ICU = 'export PKG_CONFIG_PATH="/opt/homebrew/opt/icu4c@78/lib/pkgconfig:$PKG_CONFIG_PATH";'
const AUDIT = 'python3 .skills/refresh-format-maturity/scripts/audit-format.py'

// ── schemas ──
// Prep returns two raw strings (verbatim tool output) so the deterministic floor
// and prior levels are PARSED here, not re-judged by the model.
const PREP = {
  type: 'object',
  additionalProperties: false,
  required: ['audit_json', 'prior_json'],
  properties: {
    audit_json: { type: 'string', description: 'verbatim stdout of `audit-format.py --all --json`' },
    prior_json: { type: 'string', description: 'verbatim contents of web/docs/static/data/format-maturity.json, or "" if absent' },
  },
}

// Score returns the 9 dimension cells WITH per-cell evidence — no free level pick.
const SCORE = {
  type: 'object',
  additionalProperties: false,
  properties: {
    format: { type: 'string' },
    is_real_format: { type: 'boolean' },
    okapi_counterpart: { type: 'string', description: 'matching okf_ filter or "none"' },
    dimension_scores: {
      type: 'array',
      description: 'EXACTLY these 9 dimensions, named EXACTLY: Reader, Writer, Config, Spec, Parity, Malformed, Corpus, Detection, Docs',
      items: {
        type: 'object',
        additionalProperties: false,
        properties: {
          dimension: { type: 'string', enum: ['Reader', 'Writer', 'Config', 'Spec', 'Parity', 'Malformed', 'Corpus', 'Detection', 'Docs'] },
          score: { type: 'string', enum: ['complete', 'partial', 'none', 'na'] },
          evidence: { type: 'string', description: 'REQUIRED for complete/partial: a file:line or TestName proving it; "" only for none/na' },
        },
        required: ['dimension', 'score', 'evidence'],
      },
    },
    delta_justification: { type: 'string', description: 'If your cells imply a level DIFFERENT from the prior level shown, cite the SPECIFIC artifact gained (move up) or removed/RED (move down). Else "".' },
    blocking_gaps: { type: 'array', items: { type: 'string' }, description: 'ordered, highest rubric-weight first, to reach the NEXT level' },
    top_risk: { type: 'string', description: 'the single most important correctness/robustness risk' },
    confidence: { type: 'string', enum: ['high', 'medium', 'low'] },
  },
  required: ['format', 'dimension_scores', 'confidence'],
}

const REMEDIATE = {
  type: 'object',
  additionalProperties: false,
  properties: {
    format: { type: 'string' },
    action: { type: 'string', description: 'what artifact was added/changed' },
    files_changed: { type: 'array', items: { type: 'string' } },
    test_command: { type: 'string' },
    test_passed: { type: 'boolean' },
    new_level_estimate: { type: 'string' },
    notes: { type: 'string' },
  },
  required: ['format', 'action', 'test_passed'],
}

const ORDER = { L0: 0, L1: 1, L2: 2, L3: 3, L4: 4 }
const RANK = { L0: 0, L1: 1, L2: 2, L3: 3, L4: 4 }
const NEXT = { L0: 'L1', L1: 'L2', L2: 'L3', L3: 'L4', L4: '—' }
const CANON = ['reader', 'writer', 'config', 'spec', 'parity', 'malformed', 'corpus', 'detection', 'docs']
const LABELS = {
  reader: 'Reader', writer: 'Writer / round-trip', config: 'Config + Schema',
  spec: 'spec.yaml', parity: 'Parity', malformed: 'Malformed / robustness',
  corpus: 'Corpus breadth', detection: 'Detection', docs: 'Docs + wiring',
}

function minL(a, b) { return RANK[a] <= RANK[b] ? a : b }
function normDim(name) {
  const n = String(name).toLowerCase().replace(/^\d+\.\s*/, '').trim()
  if (n.includes('reader')) return 'reader'
  if (n.includes('writer') || n.includes('round')) return 'writer'
  if (n.includes('config')) return 'config'
  if (n.includes('spec')) return 'spec'
  if (n.includes('parity')) return 'parity'
  if (n.includes('malformed')) return 'malformed'
  if (n.includes('corpus')) return 'corpus'
  if (n.includes('detection')) return 'detection'
  if (n.includes('docs') || n.includes('wiring')) return 'docs'
  return null
}

// enforce: build the dim map from one sample, downgrading any complete/partial
// cell whose evidence is empty to 'none' ("cited or it didn't happen").
function enforceEvidence(dimScores) {
  const dims = {}
  for (const d of dimScores || []) {
    const k = normDim(d.dimension)
    if (!k || (k in dims)) continue
    let s = d.score || 'none'
    const ev = String(d.evidence || '').trim()
    if ((s === 'complete' || s === 'partial') && ev === '') s = 'none'
    dims[k] = s
  }
  for (const k of CANON) if (!(k in dims)) dims[k] = 'none'
  return dims
}

// modeDimensions: per-dimension majority across N samples; ties break to the
// LOWER score (conservative). abstain/none dominate when the panel disagrees.
const SVAL = { complete: 3, partial: 2, na: 1, none: 0 }
function modeDimensions(samples) {
  const out = {}
  for (const k of CANON) {
    const counts = {}
    for (const s of samples) { const v = (s[k] || 'none'); counts[v] = (counts[v] || 0) + 1 }
    let best = 'none', bestN = -1
    for (const v of Object.keys(counts)) {
      const n = counts[v]
      if (n > bestN || (n === bestN && SVAL[v] < SVAL[best])) { best = v; bestN = n }
    }
    out[k] = best
  }
  return out
}

// dimsFromFloor: the mechanical dimensions decided PURELY by the deterministic
// file audit (reader/config/malformed/spec/parity/detection/docs presence).
// These are not re-judged by the model — pinning them is the dominant variance
// cut (the L1<->L2 swings came from the model re-deciding Malformed presence a
// file check already settles).
function dimsFromFloor(floor, type) {
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

// reconcileDims: start from the floor, then let the model DEMOTE (never raise)
// only the three genuinely judgment-bound dimensions — writer byte-equality,
// parity xfail hygiene, corpus real-vs-synthetic — each evidence-gated by
// enforceEvidence. Everything else is the floor verbatim.
const QUALITY = new Set(['writer', 'parity', 'corpus'])
const DORDER = { none: 0, partial: 1, complete: 2 }
function reconcileDims(agentDims, floor, type) {
  if (!floor) return agentDims
  const base = dimsFromFloor(floor, type)
  const out = {}
  for (const key of CANON) {
    const f = base[key]
    if (!QUALITY.has(key) || f === 'na' || f === 'none') { out[key] = f; continue }
    const a = agentDims[key] || 'none'
    out[key] = (a in DORDER && DORDER[a] < DORDER[f]) ? a : f
  }
  return out
}

// gateLevel: the rubric as a pure function of the dimension grid (deterministic).
// "highest tier whose gate is fully met; strictest unmet caps it."
function gateLevel(dims, type) {
  const c = (k) => dims[k] || 'none'
  const has = (k) => c(k) !== 'none' // present (working): partial | complete | na
  const full = (k) => c(k) === 'complete' || c(k) === 'na' // top-quality
  // L1: a working reader + writer + config (writer 'partial' = writable, just not byte-exact)
  if (!(has('reader') && has('writer') && has('config'))) return 'L0'
  // L2: spec (or harvest corpus substitute) + malformed, both fully present
  const specPath = type === 'harvest' ? full('corpus') : full('spec')
  if (!(full('malformed') && specPath)) return 'L1'
  if (type === 'harvest') {
    if (has('corpus') && full('docs')) return 'L3' // harvest ceiling via the self-contained ladder
    return 'L2'
  }
  // L3: parity test present (passes) + a corpus test + docs wiring
  if (!(has('parity') && has('corpus') && full('docs'))) return 'L2'
  // L4: byte-faithful writer + hygienic parity + full schema + REAL corpus (quality demotions land here)
  if (full('writer') && full('config') && full('parity') && full('corpus')) return 'L4'
  return 'L3'
}

// objective hard caps from files (always apply, both directions are unambiguous).
function capByFloor(level, floor) {
  if (!floor) return level
  const has = floor.has || {}
  let lvl = level
  if (has.reader === false) lvl = 'L0'
  if (has.writer === false && floor.type !== 'read-only') lvl = minL(lvl, 'L1')
  if (RANK[lvl] > RANK[floor.ceiling]) lvl = floor.ceiling // never above what files can support
  return lvl
}

// applySticky: re-runs reproduce the prior level; a move publishes only with a
// cited justification. Without a citation the suppressed move is recorded but
// the prior level stands — this is the dominant reproducibility lever.
function applySticky(prior, derived, justification) {
  if (!ANCHOR || !prior) return { level: derived, derived_from: 'dimensions', delta: derived !== prior && prior ? { from: prior, to: derived } : null }
  if (prior === derived) return { level: derived, derived_from: 'dimensions', delta: null }
  const j = String(justification || '').trim()
  const cited = /[\w/.-]+\.(go|yaml|json)(:\d+)?|Test\w+|\bRED\b|added|removed|now (asserts|passes|fails)/.test(j)
  if (cited) return { level: derived, derived_from: 'dimensions', delta: { from: prior, to: derived, why: j } }
  return { level: prior, derived_from: 'sticky-prior', delta: { from: prior, to: derived, why: 'SUPPRESSED: uncited move' } }
}

function ftype(floor, s) {
  if (floor && floor.type) return floor.type
  if (s.format === 'pdf') return 'read-only'
  if (s.format === 'splicedlines') return 'internal'
  const cp = String(s.okapi_counterpart || '')
  return (cp === '' || cp.startsWith('none') || cp.includes('harvest')) ? 'harvest' : 'parity'
}

function scorePrompt(fmt, floor, priorLevel) {
  const floorStr = floor ? JSON.stringify(floor) : '(none)'
  return `Score the neokapi format "${fmt}" against the rubric in docs/internals/format-maturity.md. cwd = ${REPO}. Okapi Java filters: ${OKAPI}/.

DETERMINISTIC FILE FLOOR (already computed by audit-format.py — TRUST IT, do not recompute file presence):
${floorStr}
PRIOR PUBLISHED LEVEL: ${priorLevel || '(none — first scoring)'}

Your job is NOT to pick a level. Score EXACTLY the 9 dimensions (Reader, Writer (writer + round-trip fidelity), Config (config + schema), Spec (spec.yaml; "na" for harvest), Parity ("na" for harvest), Malformed, Corpus, Detection, Docs) as complete/partial/none/na, and the workflow computes the level from your grid. Rules:
- The mechanical dimensions are already settled by the floor above (reader/writer/config presence, malformed_test/spec.yaml/parity_spec_test presence via test_kinds + has). ECHO them; do not contradict a file fact.
- Spend your judgment on the 3 dimensions the rubric bolds, and CITE evidence (file:line or TestName) for every complete/partial — an uncited complete is dropped to none:
  - Writer: READ the roundtrip/skeleton test assertions — "complete" only if they assert byte/semantic EQUALITY (require.Equal on rendered output), "partial" if they only assert no-error. Run if quick: ${ICU} go test -tags fts5 ./core/formats/${fmt}/ -count=1 — a RED writer test is "none".
  - Parity: open core/formats/${fmt}/parity-annotations.yaml — every skip/expected_fail must be non-native-bug with a spec/Okapi citation; unattributed or pure default-diff entries make Parity "partial". Use the real fields (severity / skip.reason), not "divergence_kind".
  - Corpus: "complete" only if testdata/ has REAL/upstream files (or spec.yaml input_file: okapi:...), "partial" if synthetic-only.
- If your grid implies a level DIFFERENT from the PRIOR level above, you MUST cite the specific artifact gained or removed/RED in delta_justification; otherwise leave it "" and the prior level stands.
Give blocking_gaps to the next level (highest weight first, citing files), the single top correctness/robustness risk, and your confidence. Be skeptical and specific.`
}

function remediatePrompt(p) {
  const gap = p.blocking_gaps[0] || 'add the highest-weight missing rubric artifact'
  return `You are advancing the neokapi format "${p.format}" from ${p.level} toward ${p.next_level}. cwd = ${REPO} (you may EDIT files here). Do exactly ONE focused, verified improvement: the highest-leverage gap:

    ${gap}

Rules:
- Touch ONLY core/formats/${p.format}/ (its own files) — other formats run in parallel.
- If the gap is a malformed_test: add core/formats/${p.format}/malformed_test.go with a table of broken/truncated/garbage/nil inputs, each asserting require.NotPanics, that Open rejects a nil doc/reader, and that parse errors surface on the channel (PartResult.Error) not silently. Match the package's test style; import core/internal/testutil. Template: core/formats/arb/malformed_test.go or core/formats/json/malformed_test.go. Do NOT leave scratch/probe test files behind.
- If the gap is schema.go: implement format.SchemaProvider on the Config exposing exactly the keys ApplyMap accepts; mirror core/formats/properties/schema.go. NOTE: this changes generated reference-data — only do it if you will also run \`make generate-reference-docs\`; otherwise skip and report.
- If the gap is "convert no-error roundtrip to assert equality" or a RED test: make the assertion real / fix or document the divergence — do NOT weaken a test to make it pass.
- Do NOT regex-rewrite serialized writer output. Do NOT weaken assertions. Do NOT add a reader to a deliberately write-only format. Follow golang-code-style.
- VERIFY: \`${ICU} go test -race -tags fts5 ./core/formats/${p.format}/ -count=1\`. It MUST compile and pass. If it surfaces a real bug, report it with test_passed=false rather than papering over it.

Return what you changed, the exact test command, and whether it passed.`
}

function publishPrompt(json) {
  return `Write the refreshed format-maturity dashboard dataset. cwd = ${REPO}.

1. Get today's date: run \`date -u +%Y-%m-%d\`. Call it TODAY.
2. Take this EXACT JSON, replace the literal string __DATE__ with TODAY, and write it to web/docs/static/data/format-maturity.json (overwrite):

${json}

3. Update web/docs/static/data/format-maturity-history.json (a JSON array): remove any entry whose "date" equals TODAY, then append {"date": TODAY, "total": <summary.total>, "by_level": <summary.by_level>, "golden_passed": <run_integrity.golden_passed>, "moves": <run_integrity.moves>}. Keep it sorted by date ascending.
4. Both files MUST be 2-space indented (the repo formatter, \`vp check\`, requires it). Verify both are valid JSON. Report the level distribution you published.`
}

function buildDataset(rows, runIntegrity) {
  const formats = rows.slice().sort((a, b) => a.id.localeCompare(b.id))
  const by_level = { L0: 0, L1: 0, L2: 0, L3: 0, L4: 0 }
  for (const f of formats) by_level[f.level] = (by_level[f.level] || 0) + 1
  return {
    generated_at: '__DATE__', target_level: TARGET, source: 'format-triage workflow (deterministic floor + evidence-cited dimensions + sticky anchor)',
    scorer_version: 2,
    run_integrity: runIntegrity,
    summary: { total: formats.length, by_level }, dimensions: CANON, dimension_labels: LABELS, formats,
  }
}

// ── Phase: Prep (deterministic floor + prior levels) ──
phase('Prep')
let floorByFmt = {}, priorByFmt = {}
{
  const prep = await agent(
    `Two verbatim captures, no editing or summarizing. cwd = ${REPO}.
1. Run: ${AUDIT} --all --json   → put its EXACT stdout in audit_json.
2. Read web/docs/static/data/format-maturity.json → put its EXACT contents in prior_json (or "" if the file does not exist).`,
    { label: 'prep', phase: 'Prep', schema: PREP },
  ).catch(() => null)
  if (prep) {
    try { for (const f of JSON.parse(prep.audit_json)) floorByFmt[f.format] = f } catch (e) { log('Prep: could not parse audit_json — proceeding without floor.') }
    try {
      const pj = JSON.parse(prep.prior_json || '{}')
      for (const f of (pj.formats || [])) priorByFmt[f.id] = f.level
    } catch (e) { log('Prep: no parseable prior dashboard — sticky anchoring disabled this run.') }
  }
  log(`Prep: floor for ${Object.keys(floorByFmt).length} formats, prior levels for ${Object.keys(priorByFmt).length}.`)
}

// ── Phase: Score (ensemble of evidence-cited grids; level COMPUTED, not picked) ──
phase('Score')
log(`Scoring ${FORMATS.length} formats × ${SAMPLES} sample(s) (target ${TARGET}, anchor=${ANCHOR}).`)
const tasks = []
for (const f of FORMATS) for (let i = 0; i < SAMPLES; i++) tasks.push({ f, i })
const raw = (await parallel(
  tasks.map((t) => () => agent(scorePrompt(t.f, floorByFmt[t.f], priorByFmt[t.f]), { label: SAMPLES > 1 ? `${t.f}#${t.i + 1}` : t.f, phase: 'Score', schema: SCORE })
    .then((r) => ({ ...r, format: (r && r.format) || t.f }))
    .catch(() => null)),
)).filter(Boolean)

// group samples by format → reconcile
const byFmt = {}
for (const r of raw) (byFmt[r.format] = byFmt[r.format] || []).push(r)

const rows = []
let movesPublished = 0, movesSuppressed = 0
for (const f of FORMATS) {
  const samples = byFmt[f] || []
  if (!samples.length) { log(`  ${f}: no sample returned — skipped.`); continue }
  const floor = floorByFmt[f]
  const type = ftype(floor, samples[0])
  // per-sample enforced+clamped grids → per-sample computed level (for agreement)
  const grids = samples.map((s) => reconcileDims(enforceEvidence(s.dimension_scores), floor, type))
  const perSampleLevel = grids.map((g) => capByFloor(gateLevel(g, type), floor))
  const dims = modeDimensions(grids)
  // harvest: spec/parity are na
  if (type === 'harvest') for (const k of ['spec', 'parity']) if (dims[k] === 'none') dims[k] = 'na'
  if (type === 'read-only' && dims.writer === 'none') dims.writer = 'na'
  const derived = capByFloor(gateLevel(dims, type), floor)
  // agreement = fraction of samples whose computed level == the modal computed level
  const lv = {}; for (const l of perSampleLevel) lv[l] = (lv[l] || 0) + 1
  const modal = Object.keys(lv).sort((a, b) => lv[b] - lv[a])[0]
  const agreement = Number((perSampleLevel.filter((l) => l === modal).length / perSampleLevel.length).toFixed(2))
  // pick the sample whose computed level == derived to source its delta_justification
  const lead = samples.find((s, idx) => perSampleLevel[idx] === derived) || samples[0]
  const sticky = applySticky(priorByFmt[f], derived, lead.delta_justification)
  if (sticky.delta && sticky.derived_from === 'dimensions') movesPublished++
  if (sticky.derived_from === 'sticky-prior') movesSuppressed++
  let cp = String(lead.okapi_counterpart || (floor && floor.okapi_counterpart) || '')
  if (cp.startsWith('none') || cp.includes('harvest') || cp.includes('internal')) cp = ''
  const evidence = {}
  for (const d of (lead.dimension_scores || [])) { const k = normDim(d.dimension); if (k && d.evidence) evidence[k] = String(d.evidence) }
  rows.push({
    id: f, type, level: sticky.level, next_level: NEXT[sticky.level] || '—',
    okapi_counterpart: cp, dimensions: dims, evidence,
    floor: floor ? floor.base : null, ceiling: floor ? floor.ceiling : null,
    derived_from: sticky.derived_from, delta: sticky.delta, agreement, samples: perSampleLevel.length,
    blocking_gaps: (lead.blocking_gaps || []).slice(0, 3),
    top_risk: lead.top_risk || '', confidence: lead.confidence || '',
  })
}
log(`Scored ${rows.length}/${FORMATS.length}. Moves published: ${movesPublished}, suppressed (uncited): ${movesSuppressed}.`)

// ── Phase: Triage ──
phase('Triage')
const dist = { L0: 0, L1: 0, L2: 0, L3: 0, L4: 0 }
for (const r of rows) dist[r.level] = (dist[r.level] || 0) + 1
const plan = rows
  .filter((r) => (ORDER[r.level] ?? 9) < (ORDER[TARGET] ?? 4))
  .map((r) => ({ format: r.id, level: r.level, next_level: r.next_level, blocking_gaps: r.blocking_gaps, top_risk: r.top_risk }))
  .sort((a, b) => (ORDER[a.level] - ORDER[b.level]))
const lowAgree = rows.filter((r) => r.agreement < 1 && r.samples > 1).map((r) => r.id)
log(`Distribution: ${JSON.stringify(dist)}. ${plan.length} below ${TARGET}.${lowAgree.length ? ' Low-agreement: ' + lowAgree.join(',') : ''}`)
for (const p of plan.slice(0, 10)) log(`  ${p.level}→${p.next_level} ${p.format}: ${(p.blocking_gaps[0] || '').slice(0, 80)}`)

// ── Phase: Remediate (optional) ──
let remediated = []
if (MODE === 'remediate' && plan.length) {
  phase('Remediate')
  const todo = LIMIT > 0 ? plan.slice(0, LIMIT) : plan
  log(`Remediating ${todo.length} formats (one verified improvement each).`)
  remediated = (await parallel(
    todo.map((p) => () => agent(remediatePrompt(p), { label: `fix:${p.format}`, phase: 'Remediate', schema: REMEDIATE }).catch(() => null)),
  )).filter(Boolean)
  const passed = remediated.filter((r) => r.test_passed).length
  log(`Remediation: ${passed}/${remediated.length} verified green. Review the diff before committing.`)
}

// ── Phase: Publish ──
const golden_passed = rows.every((r) => r.agreement === 1 || r.samples === 1)
const runIntegrity = { samples: SAMPLES, anchored: ANCHOR, moves: { published: movesPublished, suppressed: movesSuppressed }, low_agreement: lowAgree, golden_passed }
const dataset = buildDataset(rows, runIntegrity)
if (PUBLISH) {
  phase('Publish')
  await agent(publishPrompt(JSON.stringify(dataset, null, 2)), { label: 'publish', phase: 'Publish' })
  log('Published web/docs/static/data/format-maturity{,-history}.json — rebuild the docs to see it.')
}

// per-format computed levels exposed for reproducibility measurement
const levels = {}; for (const r of rows) levels[r.id] = r.level
const agreementByFmt = {}; for (const r of rows) agreementByFmt[r.id] = r.agreement
return { distribution: dist, target: TARGET, levels, agreement: agreementByFmt, moves: runIntegrity.moves, plan, remediated, dataset }
