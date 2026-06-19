export const meta = {
  name: 'format-triage',
  description: 'Triage every neokapi format against the multi-axis maturity rubric (Engine L0–L4, Vocabulary V0–V3, Editor E0–E4, Knowledge K0–K3, Corpus C0–C3, Security S0–S4, Structure & Geometry G0–G4), rank the work to push each toward a target level, optionally remediate (add the top missing test/artifact, verified), and refresh the /format-maturity dashboard dataset',
  whenToUse: 'Run periodically to track and advance format maturity. Default (no args) = score + triage + publish the dashboard. args: {target:"L2|L3|L4", mode:"triage|remediate", formats:[ids], limit:N, publish:false, samples:N, anchor:true}',
  phases: [
    { title: 'Prep', detail: 'one agent: deterministic per-axis file-floor (audit-format.py --all --json) + prior dashboard levels + support.yaml tiers' },
    { title: 'Score', detail: 'one agent per format (×samples) returns evidence-cited quality-dimension cells — NOT a free level pick' },
    { title: 'Triage', detail: 'rank the formats below target and the highest-leverage gap for each' },
    { title: 'Remediate', detail: '(mode=remediate) add the top missing artifact per format and verify it' },
    { title: 'Publish', detail: 'write the refreshed dashboard dataset + history snapshot + docs snapshot block + support.yaml last_certified + ledger run record' },
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
const TARGET = cfg.target || 'L4' // engine-axis target; the other axes report distributions
const MODE = cfg.mode || 'triage' // 'triage' | 'remediate'
const PUBLISH = cfg.publish !== false
const LIMIT = typeof cfg.limit === 'number' ? cfg.limit : 0 // 0 = no cap in remediate
const SAMPLES = Math.max(1, typeof cfg.samples === 'number' ? cfg.samples : 1) // ensemble size per format
const ANCHOR = cfg.anchor !== false // sticky to the prior committed level unless a cited delta

// FALLBACK ONLY. The scored universe is normally derived from the Prep audit
// (audit-format.py dir-walks core/formats/ minus exec/jsx/memorytest), so a new
// format is picked up automatically — no edit here, matching bootstrap-publish
// and the dir-walk validators. This list is used only when the audit can't be
// parsed (Prep failed) and no explicit `cfg.formats` was given. Workflow scripts
// have no filesystem access, so it can't readdir directly — hence the audit JSON.
const ALL_FORMATS = [
  'androidxml', 'applestrings', 'arb', 'csv', 'designtokens', 'doxygen', 'dtd',
  'epub', 'fixedwidth', 'html', 'i18next', 'icml', 'idml', 'json', 'markdown',
  'mdx', 'messageformat', 'mif', 'mo', 'mosestext', 'odf', 'openxml',
  'paraplaintext', 'pdf', 'phpcontent', 'plaintext', 'po', 'properties', 'regex',
  'resx', 'rtf', 'splicedlines', 'srt', 'tex', 'tmx', 'transtable', 'ts', 'ttml',
  'ttx', 'txml', 'versifiedtext', 'vignette', 'vtt', 'wiki', 'xcstrings', 'xliff',
  'xliff2', 'xml', 'yaml',
]
// Resolved after Prep: explicit override → audit universe (dynamic) → fallback.
let FORMATS = (Array.isArray(cfg.formats) && cfg.formats.length) ? cfg.formats : ALL_FORMATS

// The agents run with cwd = the neokapi repo root (the worktree this workflow is
// triggered from). Paths are repo-relative so the audit script + dashboard the
// agents read are the SAME tree the workflow edits — no cross-worktree drift.
const REPO = 'the neokapi repo root (your current working directory)'
const OKAPI = '/Users/asgeirf/src/okapi/Okapi/okapi/filters'
const ICU = 'export PKG_CONFIG_PATH="/opt/homebrew/opt/icu4c@78/lib/pkgconfig:$PKG_CONFIG_PATH";'
const AUDIT = 'python3 .skills/refresh-format-maturity/scripts/audit-format.py'

// ── axes (scorer v4) ──
// One grade alphabet per axis (docs/internals/format-maturity.md §2). Engine is
// the v2 L-ladder unchanged; the other axes score independently. The seven group
// into three reading-aid families — Comprehension {engine, vocabulary,
// structure} / Assurance {corpus, security} / Enablement {knowledge, editor} —
// but the gating rule (min over engine∧corpus∧knowledge) operates on the axis
// set, NOT on families.
const AXES = {
  engine: ['L0', 'L1', 'L2', 'L3', 'L4'],
  vocabulary: ['V0', 'V1', 'V2', 'V3'],
  editor: ['E0', 'E1', 'E2', 'E3', 'E4'],
  knowledge: ['K0', 'K1', 'K2', 'K3'],
  corpus: ['C0', 'C1', 'C2', 'C3'],
  // S0–S4: a NON-GATING display axis (rubric §2.6) — floor-only, no quality
  // dims. S1 bounded (core/safeio), S2 fuzzed, S3 hostile-hardened (clean
  // ledger sweep), S4 continuously-assured (sustained ledger signal).
  security: ['S0', 'S1', 'S2', 'S3', 'S4'],
  // G0–G4: a NON-GATING display axis (rubric §2.7) — floor-only, no quality
  // dims. A cumulative comprehension-depth ladder: G1 metadata plane, G2 linear
  // body text (groups / reading order), G3 logical structure (roles + tables),
  // G4 + spatial geometry (bbox/glyphs). Geometry rides on roles (G4 requires
  // G3); geometry-without-roles caps at G2 (odf/idml). na geometry is a CEILING
  // cap for non-spatial catalogs, never a gate-pass.
  structure: ['G0', 'G1', 'G2', 'G3', 'G4'],
}
const AXIS_IDS = Object.keys(AXES)
const AXIS_LABELS = { engine: 'Engine', vocabulary: 'Vocabulary', editor: 'Editor', knowledge: 'Knowledge', corpus: 'Corpus', security: 'Security', structure: 'Structure & Geometry' }
// per-axis RANK/NEXT lookups (generalize the v2 L-only tables)
const RANK = {}, NEXT = {}
for (const axis of AXIS_IDS) {
  RANK[axis] = {}; NEXT[axis] = {}
  AXES[axis].forEach((g, i) => { RANK[axis][g] = i; NEXT[axis][g] = AXES[axis][i + 1] || '—' })
}
const ORDER = RANK.engine // engine alias kept for the triage plan (v2 name)

// Dimension canon per axis (exact ids — rubric §3). The engine nine are the v2
// nine verbatim (detection/docs stay floor constants, rubric §2.1). `corpus` is
// deliberately SHARED between engine and the corpus axis: one cell, one
// judgment, consumed by both gates.
const AXIS_DIMS = {
  engine: ['reader', 'writer', 'config', 'spec', 'parity', 'malformed', 'corpus', 'detection', 'docs'],
  vocabulary: ['vocabmap', 'vocabtypes', 'writecells', 'equivalence'],
  editor: ['preview', 'identity', 'embedded', 'events'],
  knowledge: ['dossier', 'sidecar', 'refs', 'citations', 'contextpack'],
  corpus: ['corpusmanifest', 'corpus', 'fetchwiring', 'acceptance', 'sweep'],
  // security ladder signals (rubric §2.6) — floor-only, no quality dims:
  // S1 safeio import, S2 fuzz target+seed, S3 clean ledger sweep, S4 sustained.
  security: ['safeio', 'fuzz', 'sweepclean', 'sustained'],
  // structure ladder signals (rubric §2.7) — floor-only, no quality dims:
  // G1 metaplane (metadata plane / core/docmeta / caption relation), G2
  // readingorder (group emission / reading-order), G3 roles (SetSemanticRole +
  // tables/relations + a roles test), G4 geometry (SetGeometry + a geometry
  // test). The audit down-fills the cells so they are cumulative (a deeper
  // payload implies the shallower body-text/plane rungs).
  structure: ['metaplane', 'readingorder', 'roles', 'geometry'],
}
const CANON = [] // ordered union (engine first; shared `corpus` appears once)
const DIM_AXES = {} // dim id -> [axis ids] (corpus maps to both)
for (const axis of AXIS_IDS) {
  for (const d of AXIS_DIMS[axis]) {
    DIM_AXES[d] = (DIM_AXES[d] || []).concat(axis)
    if (!CANON.includes(d)) CANON.push(d)
  }
}
const DIM_SET = new Set(CANON)
const LABELS = {
  reader: 'Reader', writer: 'Writer / round-trip', config: 'Config + Schema',
  spec: 'spec.yaml', parity: 'Parity', malformed: 'Malformed / robustness',
  corpus: 'Corpus breadth', detection: 'Detection', docs: 'Docs + wiring',
  vocabmap: 'Vocabulary map', vocabtypes: 'Canonical types', writecells: 'Write cells', equivalence: 'Equivalence test',
  preview: 'Preview', identity: 'Identity binding', embedded: 'Embedded add-in', events: 'Editor events',
  dossier: 'Dossier', sidecar: 'Nativedocs sidecar', refs: 'Spec refs', citations: 'Citations check', contextpack: 'Context pack',
  corpusmanifest: 'Corpus manifest', fetchwiring: 'Fetch wiring', acceptance: 'Acceptance CI', sweep: 'Corpus sweep',
  safeio: 'Bounded (core/safeio)', fuzz: 'Fuzz target + seed', sweepclean: 'Clean sweep', sustained: 'Sustained',
  metaplane: 'Metadata plane', readingorder: 'Reading order', roles: 'Semantic roles', geometry: 'Geometry / bbox',
}

// Per-axis QUALITY sets (rubric §3 table): the only dims the model may judge,
// demote-only, citation required. Editor has none (probe-pinned).
const QUALITY = {
  engine: new Set(['writer', 'parity', 'corpus']),
  vocabulary: new Set(['writecells']),
  editor: new Set(),
  knowledge: new Set(['refs']),
  corpus: new Set(['corpus']), // the SHARED dim — same cell as engine's corpus
  security: new Set(), // floor-only (deterministic file + ledger signals)
  structure: new Set(), // floor-only (deterministic file greps): spread 0 by construction
}
const QUALITY_UNION = new Set(['writer', 'parity', 'corpus', 'writecells', 'refs'])

// ── schemas ──
// Prep returns raw strings (verbatim tool/file output) so the deterministic
// floor, prior levels, and tiers are PARSED here, not re-judged by the model.
const PREP = {
  type: 'object',
  additionalProperties: false,
  required: ['audit_json', 'prior_json', 'support_yaml'],
  properties: {
    audit_json: { type: 'string', description: 'verbatim stdout of `audit-format.py --all --json`' },
    prior_json: { type: 'string', description: 'verbatim contents of web/static/data/format-maturity.json, or "" if absent' },
    support_yaml: { type: 'string', description: 'verbatim contents of core/formats/support.yaml, or "" if absent' },
  },
}

// Score returns evidence-cited quality cells — no free level pick. The enum is
// the exact dimension canon; cells for floor-pinned dims are accepted but ignored.
const SCORE = {
  type: 'object',
  additionalProperties: false,
  properties: {
    format: { type: 'string' },
    is_real_format: { type: 'boolean' },
    okapi_counterpart: { type: 'string', description: 'matching okf_ filter or "none"' },
    dimension_scores: {
      type: 'array',
      description: 'Judge ONLY the quality dimensions — ids EXACTLY (lowercase): writer, parity, corpus (shared engine+corpus axis), writecells, refs. Cells for any other dimension are floor-pinned and ignored.',
      items: {
        type: 'object',
        additionalProperties: false,
        properties: {
          dimension: { type: 'string', enum: CANON },
          score: { type: 'string', enum: ['complete', 'partial', 'none', 'na'] },
          evidence: { type: 'string', description: 'REQUIRED for complete/partial AND for any demotion below the floor: a file:line (path ending .go/.yaml/.json, optional :N) or TestName proving it; "" only for an uncontested none/na' },
        },
        required: ['dimension', 'score', 'evidence'],
      },
    },
    delta_justification: {
      type: 'object',
      additionalProperties: false,
      description: 'Per axis: if your cells imply a level DIFFERENT from that axis\'s prior level shown, cite the SPECIFIC artifact gained (move up) or removed/RED (move down). Omit or "" otherwise.',
      properties: {
        engine: { type: 'string' }, vocabulary: { type: 'string' }, editor: { type: 'string' },
        knowledge: { type: 'string' }, corpus: { type: 'string' }, security: { type: 'string' },
        structure: { type: 'string' },
      },
    },
    blocking_gaps: {
      type: 'object',
      additionalProperties: false,
      description: 'Per axis: ordered gaps (highest rubric-weight first, citing files) to reach that axis\'s NEXT level',
      properties: {
        engine: { type: 'array', items: { type: 'string' } },
        vocabulary: { type: 'array', items: { type: 'string' } },
        editor: { type: 'array', items: { type: 'string' } },
        knowledge: { type: 'array', items: { type: 'string' } },
        corpus: { type: 'array', items: { type: 'string' } },
        security: { type: 'array', items: { type: 'string' } },
        structure: { type: 'array', items: { type: 'string' } },
      },
    },
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

function minG(axis, a, b) { return RANK[axis][a] <= RANK[axis][b] ? a : b }

// normDim: EXACT id match only (rubric §3) — substring matching mis-buckets the
// new ids (e.g. "writecells" vs "writer"). An unmatched agent dim is dropped
// with a warning, never coerced to 'none'.
function normDim(name, warnings) {
  const n = String(name).toLowerCase().replace(/^\d+\.\s*/, '').trim()
  if (DIM_SET.has(n)) return n
  if (warnings) warnings.add(String(name))
  return null
}

// enforceEvidence: collect one sample's cells (first occurrence per dim wins).
// "Cited or it didn't happen" is enforced in reconcileDims: a quality demotion
// without evidence is DROPPED (the floor stands) — rubric §3.
function enforceEvidence(dimScores, warnings) {
  const dims = {}, evidence = {}
  for (const d of dimScores || []) {
    const k = normDim(d.dimension, warnings)
    if (!k || (k in dims)) continue
    dims[k] = d.score || 'none'
    evidence[k] = String(d.evidence || '').trim()
  }
  return { dims, evidence }
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

// ── per-axis floor cells (decided PURELY by audit-format.py's axes block) ──
// The audit parses the axis artifacts (vocabulary.yaml census with evidence
// resolution, dossier.yaml, corpus.yaml, integrations.yaml + probes); absent
// artifacts floor at the zero grade. The model never re-decides a file fact.

// engine: prefer the audit's v3 axes.engine.signals (it carries the
// mutation-check demotion, rubric §3: remediation-introduced tests count as
// 'partial' until mutation-checked); fall back to the v2 computation for
// legacy audit JSON without axes.
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
function engineDims(floor, type) {
  const sig = floor && floor.axes && floor.axes.engine && floor.axes.engine.signals
  if (!sig) return legacyEngineDims(floor, type)
  const out = {}
  for (const k of AXIS_DIMS.engine) out[k] = sig[k] || 'none'
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

// editor: a pure echo of the audit's deterministic probes (E1 preview, E2
// identity round-trip test, E3 committed manifest, E4 event handler). The
// declared-vs-probed min lands via the axis ceiling. No quality dims.
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
    // harvest ladder stands in for spec.yaml refs (rubric K2 "or the harvest ladder")
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

// sweep cell: 'none' until a corpus-sweep record exists; a recorded sweep with
// CRASH/HANG/OOM/ROUNDTRIP_DRIFT counts is not green.
function sweepCell(v) {
  if (v == null || v === 'unknown') return 'none'
  if (typeof v === 'object') {
    const bad = ['CRASH', 'HANG', 'OOM', 'ROUNDTRIP_DRIFT']
    return bad.some((k) => (v[k] || 0) > 0) ? 'partial' : 'complete'
  }
  return 'partial' // recorded but in an unrecognized shape — present, not proven green
}

// corpus axis: the shared `corpus` cell is the ENGINE seed (v2 semantics —
// one judgment flows into both gates); the rest come from corpus.yaml census,
// fetch wiring probes, and ledger-recorded acceptance/sweep results.
function corpusDims(floor, engineCells) {
  const sig = floor && floor.axes && floor.axes.corpus && floor.axes.corpus.signals
  const shared = engineCells.corpus
  if (!sig) return { corpusmanifest: 'none', corpus: shared, fetchwiring: 'none', acceptance: 'none', sweep: 'none' }
  const census = sig.census || {}
  const fw = sig.fetchwiring || {}
  const corpusmanifest = sig.corpusmanifest !== 'present' ? 'none'
    : ((census.uncovered || 0) === 0 ? 'complete' : 'partial')
  const fetchwiring = fw.wired ? 'complete'
    : sig.countersigned_na ? 'na' // countersigned na satisfies the C2 gate (rubric §2.5)
      : (fw.scheme_in_spec || fw.fetch_script) ? 'partial' : 'none'
  return {
    corpusmanifest,
    corpus: shared,
    fetchwiring,
    acceptance: ['complete', 'partial', 'none', 'na'].includes(sig.acceptance) ? sig.acceptance : 'none',
    sweep: sweepCell(sig.sweep),
  }
}

// security: a pure echo of the audit's deterministic signal cells (S1 safeio
// import, S2 fuzz target+seed, S3 clean ledger sweep, S4 sustained). Like
// editorDims, it reads audit `axes.security.signals` and never re-judges a file
// fact. Floor-only — no quality dims, so the published level is fully pinned.
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

// structure: a pure echo of the audit's deterministic signal cells (G1
// metaplane, G2 readingorder, G3 roles, G4 geometry), already down-filled to be
// cumulative by the audit (a deeper payload implies the shallower body-text/plane
// rungs). Like securityDims it never re-judges a file fact. Floor-only — no
// quality dims, so the published level is fully pinned (spread 0 by construction).
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

// the full floor grid: every canon dim, decided by files (shared corpus once)
function floorDimsAll(floor, type) {
  const e = engineDims(floor, type)
  return { ...e, ...vocabularyDims(floor, type), ...editorDims(floor), ...knowledgeDims(floor, type), ...corpusDims(floor, e), ...securityDims(floor), ...structureDims(floor) }
}

// reconcileDims: start from the floor, then let the model DEMOTE (never raise)
// only the per-axis QUALITY dims — writer byte-equality, parity xfail hygiene,
// corpus real-vs-synthetic (shared), vocabulary write-cell authorship,
// knowledge ref accuracy. Each demotion requires evidence or it is DROPPED
// (rubric §3: "demote-only, each demotion requiring a citation or it is
// dropped"). Everything else is the floor verbatim.
const DORDER = { none: 0, partial: 1, complete: 2 }
function reconcileDims(cells, floorDims) {
  const out = {}
  for (const key of CANON) {
    const f = floorDims[key]
    if (!QUALITY_UNION.has(key) || f === 'na' || f === 'none') { out[key] = f; continue }
    const a = cells.dims[key]
    const ev = String(cells.evidence[key] || '').trim()
    out[key] = (a in DORDER && DORDER[a] < DORDER[f] && ev !== '') ? a : f
  }
  return out
}

// ── per-axis gates: the rubric as pure functions of the dimension grid ──
// "highest tier whose gate is fully met; strictest unmet caps it."

// gateEngine == scorer-v2 gateLevel VERBATIM (rubric §2.1: gates unchanged so
// published history stays numerically comparable).
function gateEngine(dims, type) {
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

// gateVocab (rubric §5): V1 = read cells resolved + canonical types;
// V2 = write cells + equivalence present; the top return is V3 and the audit
// ceiling clamps it to V2 unless zero-unknown cells are file-proven — so a
// single writecells demotion moves exactly one step (V3→V2, V2→V1).
function gateVocab(dims) {
  const c = (k) => dims[k] || 'none'
  const has = (k) => c(k) !== 'none'
  const full = (k) => c(k) === 'complete' || c(k) === 'na'
  if (!(full('vocabmap') && full('vocabtypes'))) return 'V0'
  if (!(has('writecells') && has('equivalence'))) return 'V1'
  if (full('writecells') && full('equivalence')) return 'V3'
  return 'V2'
}

// gateEditor (rubric §2.3): a pure cumulative probe ladder — a missing lower
// rung caps the level. No judgment anywhere on this axis.
function gateEditor(dims) {
  const full = (k) => (dims[k] || 'none') === 'complete' || (dims[k] || 'none') === 'na'
  if (!full('preview')) return 'E0'
  if (!full('identity')) return 'E1'
  if (!full('embedded')) return 'E2'
  if (!full('events')) return 'E3'
  return 'E4'
}

// gateKnowledge (rubric §5): K1 = dossier + sidecar; K2 = + refs + schema.go
// (a deterministic file fact passed in, not a judged dim); K3 = + green
// recorded citations + context-pack checks (ledger oracles).
function gateKnowledge(dims, hasSchema) {
  const c = (k) => dims[k] || 'none'
  const full = (k) => c(k) === 'complete' || c(k) === 'na'
  if (!(full('dossier') && full('sidecar'))) return 'K0'
  if (!(full('refs') && hasSchema)) return 'K1'
  if (!(full('citations') && full('contextpack'))) return 'K2'
  return 'K3'
}

// gateCorpus (rubric §5): C1 = manifest covers all testdata + tests consume it
// (the shared corpus cell, has = partial counts); C2 = + Tier B wiring or a
// countersigned na; C3 = + REAL corpus (full shared cell) + green acceptance CI
// + green sweep — the externally-verified-wild-files bar rides the audit
// ceiling (an oracle outside the scoring agent's control).
function gateCorpus(dims) {
  const c = (k) => dims[k] || 'none'
  const has = (k) => c(k) !== 'none'
  const full = (k) => c(k) === 'complete' || c(k) === 'na'
  if (!(full('corpusmanifest') && has('corpus'))) return 'C0'
  if (!full('fetchwiring')) return 'C1'
  if (!(full('corpus') && full('acceptance') && full('sweep'))) return 'C2'
  return 'C3'
}

// gateSecurity (rubric §2.6): a pure cumulative floor ladder — a missing lower
// rung caps the level, no quality dims (so spread 0 by construction). S1
// bounded (safeio), S2 fuzzed, S3 hostile-hardened (clean sweep), S4
// continuously-assured (sustained). The S3/S4 cells are ledger-driven; the
// audit ceiling (S2 absent a clean sweep) clamps the published level.
function gateSecurity(dims) {
  const full = (k) => (dims[k] || 'none') === 'complete' || (dims[k] || 'none') === 'na'
  if (!full('safeio')) return 'S0'
  if (!full('fuzz')) return 'S1'
  if (!full('sweepclean')) return 'S2'
  if (!full('sustained')) return 'S3'
  return 'S4'
}

// gateStructure (rubric §2.7): a pure cumulative comprehension-depth ladder,
// mirror of gateSecurity — a missing lower rung caps the level, no quality dims
// (spread 0 by construction). G1 metadata plane, G2 linear body text, G3 logical
// structure (roles), G4 + spatial geometry. The audit DOWN-FILLS the cells so a
// deeper payload implies the shallower rungs (geometry ⇒ body-text/plane, roles
// ⇒ body-text/plane) and so geometry-without-roles stops at G2 (odf/idml). The
// na geometry case is handled as a per-format CEILING cap in the audit, never as
// a full('na') gate-pass here (rubric §5 decision 6).
// Strict-cumulative is SAFE only because audit-format.py down-fills the cells
// first (a deeper payload forces the shallower cells complete: roles/geometry ⇒
// metaplane+readingorder), so a roles-rich format is never capped at G0. Do not
// remove that down-fill (see _structure_axis) or this gate mis-scores.
function gateStructure(dims) {
  const full = (k) => (dims[k] || 'none') === 'complete' || (dims[k] || 'none') === 'na'
  if (!full('metaplane')) return 'G0'
  if (!full('readingorder')) return 'G1'
  if (!full('roles')) return 'G2'
  if (!full('geometry')) return 'G3'
  return 'G4'
}

// objective hard caps from files (always apply, both directions are unambiguous).
// engine keeps the v2 reader/writer hard caps + top-level ceiling; the other
// axes clamp to their audit band ceiling (per-axis — harvest ceilings diverge).
function capEngine(level, floor) {
  if (!floor) return level
  const has = floor.has || {}
  let lvl = level
  if (has.reader === false) lvl = 'L0'
  if (has.writer === false && floor.type !== 'read-only') lvl = minG('engine', lvl, 'L1')
  if (RANK.engine[lvl] > RANK.engine[floor.ceiling]) lvl = floor.ceiling // never above what files can support
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

// applySticky (per axis): re-runs reproduce the prior level; a move publishes
// only with a cited justification — EXCEPT the floor-forced case: sticky may
// never preserve a prior above the axis's floor ceiling (an artifact was
// deleted, a test removed) — then the derived level publishes citation-free
// (rubric §3). New-axis first publish has no prior => derived publishes, no delta.
function applySticky(axis, prior, derived, justification, ceiling) {
  if (!ANCHOR || !prior) return { level: derived, derived_from: 'dimensions', delta: derived !== prior && prior ? { from: prior, to: derived } : null }
  if (prior === derived) return { level: derived, derived_from: 'dimensions', delta: null }
  if (ceiling != null && RANK[axis][prior] != null && RANK[axis][prior] > RANK[axis][ceiling]) {
    return { level: derived, derived_from: 'dimensions', delta: { from: prior, to: derived, why: 'FLOOR-FORCED demotion' } }
  }
  const j = String(justification || '').trim()
  const cited = /[\w/.-]+\.(go|yaml|json)(:\d+)?|Test\w+|\bRED\b|added|removed|now (asserts|passes|fails)/.test(j)
  if (cited) return { level: derived, derived_from: 'dimensions', delta: { from: prior, to: derived, why: j } }
  return { level: prior, derived_from: 'sticky-prior', delta: { from: prior, to: derived, why: 'SUPPRESSED: uncited move' } }
}

// Consume-only boundaries: a reader but no native writer. The audit is the
// source of truth (floor.type); this fallback set generalizes the old hardcoded
// 'pdf' so docling — and any future consume-only / OCR ingestion boundary — also
// gets the read-only `na` writer/writecells patch (rubric §4 applicability).
const READ_ONLY = new Set(['pdf', 'docling'])
function ftype(floor, s) {
  if (floor && floor.type) return floor.type
  if (READ_ONLY.has(s.format)) return 'read-only'
  if (s.format === 'splicedlines') return 'internal'
  const cp = String(s.okapi_counterpart || '')
  return (cp === '' || cp.startsWith('none') || cp.includes('harvest')) ? 'harvest' : 'parity'
}

// per-axis accessors over a sample (defensive against legacy string/array shapes)
function axisJust(s, axis) {
  const dj = s && s.delta_justification
  if (!dj) return ''
  if (typeof dj === 'string') return axis === 'engine' ? dj : ''
  return String(dj[axis] || '')
}
function axisGaps(s, axis) {
  const bg = s && s.blocking_gaps
  if (Array.isArray(bg)) return axis === 'engine' ? bg : []
  return (bg && Array.isArray(bg[axis])) ? bg[axis] : []
}

// ── support.yaml tiers (tiny strict reader — the file's committed shape only:
// `formats:` → 2-space ids → 4-space scalars + 4-space `gates:` with 6-space
// list items). js-yaml is not resolvable in the workflow runtime.
function unq(s) {
  s = String(s).trim()
  if (s === '' || s === 'null' || s === '~') return null
  const q = /^"(.*)"$|^'(.*)'$/.exec(s)
  return q ? (q[1] != null ? q[1] : q[2]) : s
}
function parseSupportYaml(text) {
  const out = {}
  let cur = null, inGates = false
  for (const raw of String(text || '').split('\n')) {
    const line = raw.replace(/\s+$/, '')
    if (!line || /^\s*#/.test(line)) continue
    let m
    if ((m = /^  ([A-Za-z0-9_-]+):\s*$/.exec(line))) {
      cur = m[1]
      out[cur] = { declared: null, since: null, last_certified: null, gates: [] }
      inGates = false
      continue
    }
    if (!cur) continue
    if (/^    gates:\s*$/.test(line)) { inGates = true; continue }
    if (inGates && (m = /^      - (.+)$/.exec(line))) { out[cur].gates.push(unq(m[1])); continue }
    if ((m = /^    ([A-Za-z0-9_]+):\s*(.*)$/.exec(line))) {
      inGates = false
      const v = unq(m[2])
      if (m[1] === 'tier') out[cur].declared = v
      else if (m[1] === 'tier_since') out[cur].since = v
      else if (m[1] === 'last_certified') out[cur].last_certified = v
    }
  }
  return out
}

function scorePrompt(fmt, floor, priors) {
  const floorStr = floor ? JSON.stringify(floor) : '(none)'
  const priorStr = AXIS_IDS.map((a) => `${a}: ${(priors && priors[a]) || '(none — first scoring)'}`).join(' | ')
  return `Score the neokapi format "${fmt}" against the multi-axis rubric in docs/internals/format-maturity.md (§2–§3, §5). cwd = ${REPO}. Okapi Java filters: ${OKAPI}/.

DETERMINISTIC FILE FLOOR (already computed by audit-format.py — TRUST IT, do not recompute file presence; per-axis bands + signals are under "axes"):
${floorStr}
PRIOR PUBLISHED LEVELS: ${priorStr}

Your job is NOT to pick levels. Each axis level is COMPUTED from the floor above plus your judged cells. You may judge ONLY these quality dimensions, DEMOTE-ONLY (you can never raise a floor cell), and you must CITE evidence (file:line or TestName) for every cell — an uncited demotion is DROPPED and the floor stands:
- writer (engine): READ the roundtrip/skeleton test assertions — "complete" only if they assert byte/semantic EQUALITY (require.Equal on rendered output), "partial" if they only assert no-error. Run if quick: ${ICU} go test -tags fts5 ./core/formats/${fmt}/ -count=1 — a RED writer test is "none".
- parity (engine): open core/formats/${fmt}/parity-annotations.yaml — every skip/expected_fail must be non-native-bug with a spec/Okapi citation; unattributed or pure default-diff entries make parity "partial". Use the real fields (severity / skip.reason), not "divergence_kind".
- corpus (ONE dim SHARED by the engine and corpus axes — your single judgment flows into both gates): "complete" only if testdata/ + the corpus.yaml origin census (axes.corpus.signals.corpus) show REAL/upstream files (origin url/archive-member/bug, or spec.yaml input_file: okapi:...), "partial" if synthetic/vendored-only.
- writecells (vocabulary): do the tests cited by vocabulary.yaml's write cells actually AUTHOR a target from canonical vocabulary types (fmt:*/link:*/media:*/code:*), or merely echo what the reader produced? Echo-only => "partial".
- refs (knowledge): do the spec_refs/okapi_refs/native_refs in core/formats/${fmt}/spec.yaml actually point at the clause that justifies the behavior? Wrong/vague clauses => "partial".
The editor, security, and structure axes have NO judged dimensions (editor probe-pinned; security and structure are non-gating display axes pinned to deterministic file greps — core/safeio-import / fuzz-target / ledger-sweep for security, structure/geometry payload emission for structure). Every other dimension is floor-pinned — cells you return for them are ignored. Dimension ids EXACTLY as listed (lowercase); unknown ids are dropped with a warning.
Evidence/citation shapes the gate accepts (use EXACTLY these forms): a path ending .go/.yaml/.json (optionally :line, e.g. core/formats/${fmt}/roundtrip_test.go:42), a TestName (e.g. TestRoundTrip), the word RED, "added ...", "removed ...", "now asserts/passes/fails ...".
If your cells imply a level on some axis DIFFERENT from that axis's PRIOR above, you MUST cite the specific artifact gained or removed/RED in delta_justification.<axis> (same accepted shapes); otherwise leave it "" and the prior stands. A prior sitting above an axis's floor ceiling is demoted automatically (FLOOR-FORCED) — no citation needed from you.
Give blocking_gaps PER AXIS to each axis's next level (highest weight first, citing files), the single top correctness/robustness risk, and your confidence. Be skeptical and specific.`
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
  return `Publish the refreshed multi-axis format-maturity dashboard + its companion artifacts. cwd = ${REPO}. Edit files only — do NOT commit or push (the workflow caller owns the commit).

1. Get today's date: run \`date -u +%Y-%m-%d\`. Call it TODAY.
2. Take this EXACT JSON, replace the literal string __DATE__ with TODAY, and write it to web/static/data/format-maturity.json (overwrite):

${json}

3. Update web/static/data/format-maturity-history.json (a JSON array): remove any entry whose "date" equals TODAY, then append {"date": TODAY, "total": <summary.total>, "by_level": <summary.by_level>, "by_axis": <summary.by_axis>, "golden_passed": <run_integrity.golden_passed>, "moves": <run_integrity.moves>}. Keep it sorted by date ascending. NEVER rewrite, reshape, or add fields to any EXISTING entry — "by_axis" appears on the NEW snapshot only; old single-axis entries stay byte-identical.
4. Regenerate the snapshot block in docs/internals/format-maturity.md: replace everything BETWEEN the marker lines \`<!-- BEGIN: gap-analysis report (generated) -->\` and \`<!-- END: gap-analysis report -->\` (keep both marker lines exactly) with a compact fleet report derived ONLY from the dataset you wrote in step 2:
   - the "## Maturity report" heading, then one short paragraph: generated TODAY by the triage-score ritual; data lives in web/static/data/format-maturity{,-history}.json (the /format-maturity dashboard); regenerated by any ritual that republishes the dashboard — do not edit by hand.
   - a per-axis distribution table: one row per axis (Engine, Vocabulary, Editor, Knowledge, Corpus, Security, Structure & Geometry), columns = that axis's grades with counts from summary.by_level (engine) / summary.by_axis (the rest).
   - a per-format table sorted by id, one line each: | format | axis vector (levels.engine levels.vocabulary levels.editor levels.knowledge levels.corpus levels.security levels.structure space-separated, e.g. "L3 V0 E1 K0 C0 S1 G0") | top gap (the first entry of the row's engine blocking_gaps, truncated to ~100 chars, or "—") |
5. Refresh certification in core/formats/support.yaml: for EVERY format id present in the dataset's formats[], set that format's \`last_certified\` field to "TODAY" (quoted). Change NOTHING else in this file — tier / tier_since / gates / notes / grandfathered are owned by the tier-review ritual (writer partition, format-maturity.md §1).
6. Record the run in docs/internals/format-ops-ledger.json:
   - rituals."triage-score".last_run = TODAY
   - rituals."triage-score".watermarks: core_formats_sha = output of \`git log -1 --format=%H -- core/formats\`; audit_sha = the sha256 hex of \`python3 .skills/refresh-format-maturity/scripts/audit-format.py --all --json | shasum -a 256\`; scorer_version = 4; axes_published = ["engine","vocabulary","editor","knowledge","corpus","security","structure"]. Leave model_id and prompt_sha as they are.
   - append to runs[] (append-only — never modify existing entries): {"date": TODAY, "ritual": "triage-score", "commit": output of \`git rev-parse HEAD\`, "model_id": "", "outcome": "published", "evidence": [], "followups": []}
7. Every edited JSON file MUST be 2-space indented (the repo formatter, \`vp check\`, requires it). Verify every edited JSON file parses as valid JSON. Report the engine level distribution and the per-axis distributions you published.`
}

function buildDataset(rows, runIntegrity, tierByFmt) {
  const formats = rows.slice().sort((a, b) => a.id.localeCompare(b.id))
  const by_level = { L0: 0, L1: 0, L2: 0, L3: 0, L4: 0 } // engine distribution (v1/v2 parser contract)
  const by_axis = {}
  for (const axis of AXIS_IDS) {
    by_axis[axis] = {}
    for (const g of AXES[axis]) by_axis[axis][g] = 0 // zero-init every grade for stable JSON diffs
  }
  for (const f of formats) {
    by_level[f.level] = (by_level[f.level] || 0) + 1
    for (const axis of AXIS_IDS) {
      const g = f.levels && f.levels[axis]
      if (g != null && g in by_axis[axis]) by_axis[axis][g] += 1
    }
  }
  return {
    generated_at: '__DATE__', target_level: TARGET,
    source: 'format-triage workflow (deterministic per-axis floors + evidence-cited quality dimensions + sticky anchor)',
    scorer_version: 4,
    run_integrity: runIntegrity,
    summary: { total: formats.length, by_level, by_axis },
    axes: AXES, axis_labels: AXIS_LABELS,
    dimensions: CANON, dimension_labels: LABELS, dimension_axes: DIM_AXES,
    formats,
  }
}

// ── Phase: Prep (deterministic floor + prior levels + tiers) ──
phase('Prep')
let floorByFmt = {}, priorByFmt = {}, tierByFmt = {}
{
  const prep = await agent(
    `Three verbatim captures, no editing or summarizing. cwd = ${REPO}.
1. Run: ${AUDIT} --all --json   → put its EXACT stdout in audit_json.
2. Read web/static/data/format-maturity.json → put its EXACT contents in prior_json (or "" if the file does not exist).
3. Read core/formats/support.yaml → put its EXACT contents in support_yaml (or "" if the file does not exist).`,
    { label: 'prep', phase: 'Prep', schema: PREP },
  ).catch(() => null)
  if (prep) {
    try { for (const f of JSON.parse(prep.audit_json)) floorByFmt[f.format] = f } catch (e) { log('Prep: could not parse audit_json — proceeding without floor.') }
    try {
      const pj = JSON.parse(prep.prior_json || '{}')
      // Missing scorer_version => v1 => engine-only priors (rubric §3); the
      // prior parser never gates row reads on version — `level` is the engine
      // mirror in every dataset generation, `levels{}` is v3-additive.
      const sv = Number(pj.scorer_version) || 1
      for (const f of (pj.formats || [])) {
        const p = {}
        if (sv >= 3 && f.levels && typeof f.levels === 'object') {
          for (const axis of AXIS_IDS) {
            const g = f.levels[axis]
            if (typeof g === 'string' && g in RANK[axis]) p[axis] = g
          }
        }
        if (typeof f.level === 'string' && f.level in RANK.engine) p.engine = f.level
        priorByFmt[f.id] = p
      }
    } catch (e) { log('Prep: no parseable prior dashboard — sticky anchoring disabled this run.') }
    try { tierByFmt = parseSupportYaml(prep.support_yaml) } catch (e) { log('Prep: could not parse support.yaml — rows publish tier: null.') }
  }
  log(`Prep: floor for ${Object.keys(floorByFmt).length} formats, priors for ${Object.keys(priorByFmt).length}, tiers for ${Object.keys(tierByFmt).length}.`)
  // Derive the scored universe from the dir-walk audit so a newly added format
  // is picked up automatically (no ALL_FORMATS edit). An explicit cfg.formats
  // always wins; the static list is the fallback only when Prep yields no floor.
  if (!(Array.isArray(cfg.formats) && cfg.formats.length)) {
    const discovered = Object.keys(floorByFmt).sort()
    if (discovered.length) {
      const added = discovered.filter((f) => !ALL_FORMATS.includes(f))
      const dropped = ALL_FORMATS.filter((f) => !discovered.includes(f))
      FORMATS = discovered
      if (added.length || dropped.length) log(`Prep: universe from audit (${discovered.length}); +[${added.join(',')}] -[${dropped.join(',')}] vs the static fallback.`)
    } else {
      log(`Prep: no audit floor — scoring the static ALL_FORMATS fallback (${FORMATS.length}).`)
    }
  }
}

// ── Phase: Score (ensemble of evidence-cited grids; levels COMPUTED per axis, not picked) ──
phase('Score')
log(`Scoring ${FORMATS.length} formats × ${SAMPLES} sample(s) across ${AXIS_IDS.length} axes (engine target ${TARGET}, anchor=${ANCHOR}).`)
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
const movesByAxis = {}
for (const axis of AXIS_IDS) movesByAxis[axis] = { published: 0, suppressed: 0 }
for (const f of FORMATS) {
  const samples = byFmt[f] || []
  if (!samples.length) { log(`  ${f}: no sample returned — skipped.`); continue }
  const floor = floorByFmt[f]
  const type = ftype(floor, samples[0])
  const floorDims = floorDimsAll(floor, type)
  const warn = new Set()
  // per-sample enforced+reconciled grids → per-sample computed level per axis (for agreement)
  const cells = samples.map((s) => enforceEvidence(s.dimension_scores, warn))
  const grids = cells.map((c) => reconcileDims(c, floorDims))
  if (warn.size) log(`  ${f}: dropped unknown dimension id(s): ${[...warn].join(', ')}`)
  const perSample = {}
  for (const axis of AXIS_IDS) perSample[axis] = grids.map((g) => axisLevel(axis, g, floor, type))
  const dims = modeDimensions(grids)
  // harvest/read-only na patches (floor-less safety; the audit floor already encodes these)
  if (type === 'harvest') for (const k of ['spec', 'parity']) if (dims[k] === 'none') dims[k] = 'na'
  if (type === 'read-only') for (const k of ['writer', 'writecells']) if (dims[k] === 'none') dims[k] = 'na'

  const levels = {}, next = {}, axes = {}
  let leadEngine = samples[0]
  for (const axis of AXIS_IDS) {
    const derived = axisLevel(axis, dims, floor, type)
    // agreement = fraction of samples whose computed level == the modal computed level
    const lv = {}; for (const l of perSample[axis]) lv[l] = (lv[l] || 0) + 1
    const modal = Object.keys(lv).sort((a, b) => lv[b] - lv[a])[0]
    const agreement = Number((perSample[axis].filter((l) => l === modal).length / perSample[axis].length).toFixed(2))
    // pick the sample whose computed level == derived to source its per-axis delta_justification
    const lead = samples.find((s, idx) => perSample[axis][idx] === derived) || samples[0]
    if (axis === 'engine') leadEngine = lead
    const prior = priorByFmt[f] && priorByFmt[f][axis]
    const sticky = applySticky(axis, prior, derived, axisJust(lead, axis), axisCeiling(axis, floor))
    if (sticky.delta && sticky.derived_from === 'dimensions') movesByAxis[axis].published++
    if (sticky.derived_from === 'sticky-prior') movesByAxis[axis].suppressed++
    levels[axis] = sticky.level
    next[axis] = NEXT[axis][sticky.level] || '—'
    const band = axis === 'engine'
      ? { floor: floor ? floor.base : null, ceiling: floor ? floor.ceiling : null }
      : { floor: (floor && floor.axes && floor.axes[axis] && floor.axes[axis].base) || null, ceiling: axisCeiling(axis, floor) }
    axes[axis] = {
      level: sticky.level, next: next[axis], floor: band.floor, ceiling: band.ceiling,
      derived_from: sticky.derived_from, delta: sticky.delta, agreement,
      blocking_gaps: axisGaps(leadEngine, axis).slice(0, 3),
    }
  }

  let cp = String(leadEngine.okapi_counterpart || (floor && floor.okapi_counterpart) || '')
  if (cp.startsWith('none') || cp.includes('harvest') || cp.includes('internal')) cp = ''
  const evidence = {}
  for (const d of (leadEngine.dimension_scores || [])) { const k = normDim(d.dimension); if (k && d.evidence) evidence[k] = String(d.evidence) }
  const tier = tierByFmt[f]
  rows.push({
    // level/next_level mirror the ENGINE axis (the prior parser + un-migrated
    // page contract); levels/next/axes/tier are v3-additive fields.
    id: f, type, level: levels.engine, next_level: next.engine,
    levels, next,
    okapi_counterpart: cp, dimensions: dims, evidence,
    floor: floor ? floor.base : null, ceiling: floor ? floor.ceiling : null,
    derived_from: axes.engine.derived_from, delta: axes.engine.delta,
    agreement: axes.engine.agreement, samples: grids.length,
    axes,
    tier: tier ? { declared: tier.declared, since: tier.since, last_certified: tier.last_certified, gates: tier.gates } : null,
    blocking_gaps: axisGaps(leadEngine, 'engine').slice(0, 3),
    top_risk: leadEngine.top_risk || '', confidence: leadEngine.confidence || '',
  })
}
{
  const pub = AXIS_IDS.reduce((n, a) => n + movesByAxis[a].published, 0)
  const sup = AXIS_IDS.reduce((n, a) => n + movesByAxis[a].suppressed, 0)
  log(`Scored ${rows.length}/${FORMATS.length}. Moves published: ${pub}, suppressed (uncited): ${sup}.`)
}

// ── Phase: Triage ──
phase('Triage')
const dist = { L0: 0, L1: 0, L2: 0, L3: 0, L4: 0 }
for (const r of rows) dist[r.level] = (dist[r.level] || 0) + 1
const distByAxis = {}
for (const axis of AXIS_IDS) {
  distByAxis[axis] = {}
  for (const g of AXES[axis]) distByAxis[axis][g] = 0
  for (const r of rows) if (r.levels[axis] in distByAxis[axis]) distByAxis[axis][r.levels[axis]]++
}
const plan = rows
  .filter((r) => (ORDER[r.level] ?? 9) < (ORDER[TARGET] ?? 4))
  .map((r) => ({ format: r.id, axis: 'engine', level: r.level, next_level: r.next_level, blocking_gaps: r.blocking_gaps, top_risk: r.top_risk }))
  .sort((a, b) => (ORDER[a.level] - ORDER[b.level]))
const lowAgreeByAxis = {}
for (const axis of AXIS_IDS) lowAgreeByAxis[axis] = rows.filter((r) => r.axes[axis].agreement < 1 && r.samples > 1).map((r) => r.id)
log(`Engine distribution: ${JSON.stringify(dist)}. ${plan.length} below ${TARGET}.`)
for (const axis of AXIS_IDS.filter((a) => a !== 'engine')) log(`  ${axis}: ${JSON.stringify(distByAxis[axis])}`)
const lowAgreeAll = [...new Set(AXIS_IDS.flatMap((a) => lowAgreeByAxis[a]))]
if (lowAgreeAll.length) log(`  Low-agreement: ${lowAgreeAll.join(',')}`)
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
const golden_passed = rows.every((r) => r.samples === 1 || AXIS_IDS.every((a) => r.axes[a].agreement === 1))
const runIntegrity = {
  samples: SAMPLES, anchored: ANCHOR,
  moves: {
    published: AXIS_IDS.reduce((n, a) => n + movesByAxis[a].published, 0),
    suppressed: AXIS_IDS.reduce((n, a) => n + movesByAxis[a].suppressed, 0),
    by_axis: movesByAxis,
  },
  low_agreement: lowAgreeByAxis,
  golden_passed,
}
const dataset = buildDataset(rows, runIntegrity, tierByFmt)
if (PUBLISH) {
  phase('Publish')
  await agent(publishPrompt(JSON.stringify(dataset, null, 2)), { label: 'publish', phase: 'Publish' })
  log('Published web/static/data/format-maturity{,-history}.json + docs snapshot block + support.yaml last_certified + ledger run record — rebuild the docs to see it.')
}

// per-format computed levels exposed for reproducibility measurement
// (`levels` stays the flat engine map for existing callers; `levels_by_axis` is additive)
const levels = {}; for (const r of rows) levels[r.id] = r.level
const levelsByAxis = {}; for (const r of rows) levelsByAxis[r.id] = r.levels
const agreementByFmt = {}; for (const r of rows) agreementByFmt[r.id] = r.agreement
return { distribution: dist, distribution_by_axis: distByAxis, target: TARGET, levels, levels_by_axis: levelsByAxis, agreement: agreementByFmt, moves: runIntegrity.moves, plan, remediated, dataset }
