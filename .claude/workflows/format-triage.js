export const meta = {
  name: 'format-triage',
  description: 'Triage every neokapi format against the L0–L4 maturity rubric, rank the work to push each toward a target level, optionally remediate (add the top missing test/artifact, verified), and refresh the /format-maturity dashboard dataset',
  whenToUse: 'Run periodically to track and advance format maturity. Default (no args) = score + triage + publish the dashboard. args: {target:"L2|L3|L4", mode:"triage|remediate", formats:[ids], limit:N, publish:false}',
  phases: [
    { title: 'Score', detail: 'one agent per format scores it against the 9-dimension rubric' },
    { title: 'Triage', detail: 'rank the formats below target and the highest-leverage gap for each' },
    { title: 'Remediate', detail: '(mode=remediate) add the top missing artifact per format and verify it' },
    { title: 'Publish', detail: 'write the refreshed dashboard dataset + a history snapshot' },
  ],
}

// ── config (all optional; defaults make a bare trigger do score+triage+publish) ──
// args may arrive as a parsed object OR as a JSON string — accept both.
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

const ROOT = '/Users/asgeirf/src/neokapi/neokapi/.claude/worktrees/test-harness'
const OKAPI = '/Users/asgeirf/src/okapi/Okapi/okapi/filters'
const ICU = 'export PKG_CONFIG_PATH="/opt/homebrew/opt/icu4c@78/lib/pkgconfig:$PKG_CONFIG_PATH";'

const SCORE = {
  type: 'object',
  additionalProperties: false,
  properties: {
    format: { type: 'string' },
    is_real_format: { type: 'boolean' },
    okapi_counterpart: { type: 'string', description: 'matching okf_ filter or "none"' },
    level: { type: 'string', enum: ['L0', 'L1', 'L2', 'L3', 'L4'] },
    dimension_scores: {
      type: 'array',
      description: 'EXACTLY these 9 dimensions, in this order, named EXACTLY: Reader, Writer, Config, Spec, Parity, Malformed, Corpus, Detection, Docs',
      items: {
        type: 'object',
        additionalProperties: false,
        properties: {
          dimension: { type: 'string', enum: ['Reader', 'Writer', 'Config', 'Spec', 'Parity', 'Malformed', 'Corpus', 'Detection', 'Docs'] },
          score: { type: 'string', enum: ['complete', 'partial', 'none', 'na'] },
          note: { type: 'string' },
        },
        required: ['dimension', 'score'],
      },
    },
    blocking_gaps: { type: 'array', items: { type: 'string' }, description: 'ordered, highest rubric-weight first, to reach the NEXT level' },
    top_risk: { type: 'string', description: 'the single most important correctness/robustness risk' },
    confidence: { type: 'string', enum: ['high', 'medium', 'low'] },
  },
  required: ['format', 'level', 'dimension_scores', 'blocking_gaps', 'confidence'],
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
const NEXT = { L0: 'L1', L1: 'L2', L2: 'L3', L3: 'L4', L4: '—' }
const DIM_WEIGHT = { Reader: 5, Writer: 5, Config: 4, Spec: 4, Parity: 4, Malformed: 4, Corpus: 3, Detection: 3, Docs: 3 }

function scorePrompt(fmt) {
  return `Score the neokapi format "${fmt}" against the maturity rubric in docs/internals/format-maturity.md. Repo root: ${ROOT}. Okapi Java filters: ${OKAPI}/.

Inspect core/formats/${fmt}/: read config.go (does ApplyMap reject unknown keys?), SKIM reader.go/writer.go (channel pattern, error-on-channel, RenderRunsWithData, round-trip strategy). READ THE TEST ASSERTIONS — judge whether roundtrip/skeleton tests prove byte/semantic equality vs merely "no error"; whether a malformed_test asserts NotPanics + clean Error; whether invariants/corpus/acceptance exist. Check spec.yaml + cli/parity/formats/${fmt}_spec_test.go presence; check core/formats/${fmt}/parity-annotations.yaml for unattributed or pure default-diff expected_fail entries. Determine the Okapi counterpart (ls ${OKAPI}/; if none it is a harvest format — spec/parity are "na"). Run the format's tests if quick (${ICU} go test -tags fts5 ./core/formats/${fmt}/ -count=1) and note RED tests.

Score EXACTLY 9 dimensions named: Reader, Writer (writer + round-trip fidelity), Config (config + schema), Spec (spec.yaml; "na" for harvest), Parity ("na" for harvest), Malformed, Corpus, Detection, Docs (reference wiring). Use "na" where genuinely not applicable (Parity/Spec for harvest; Writer "na" for read-only pdf). Assign ONE level L0-L4 (strictest unmet criterion caps it). List blocking_gaps to the NEXT level, highest rubric-weight first, each concrete and citing files. Give the single top correctness/robustness risk. Be skeptical and specific.`
}

function remediatePrompt(p) {
  const gap = p.blocking_gaps[0] || 'add the highest-weight missing rubric artifact'
  return `You are advancing the neokapi format "${p.format}" from ${p.level} toward ${p.next_level}. Repo root: ${ROOT} (this is your cwd; you may EDIT files here). Do exactly ONE focused, verified improvement: the highest-leverage gap, which is:

    ${gap}

Rules:
- Touch ONLY core/formats/${p.format}/ (its own files) — other formats run in parallel.
- If the gap is a malformed_test: add core/formats/${p.format}/malformed_test.go with a table of broken/truncated/garbage/nil inputs, each asserting require.NotPanics, and asserting Open rejects a nil doc/reader and that parse errors surface on the channel (PartResult.Error) rather than silently. Match the package's existing test style; import helpers from core/internal/testutil. Study core/formats/arb/malformed_test.go or core/formats/xcstrings/malformed_test.go as the template.
- If the gap is schema.go: implement format.SchemaProvider on the Config exposing exactly the keys ApplyMap accepts, with defaults/labels; mirror core/formats/properties/schema.go.
- If the gap is "convert no-error roundtrip to assert equality" or a RED test: make the assertion real / fix or document the divergence — do NOT weaken a test to make it pass.
- Do NOT regex-rewrite serialized writer output. Do NOT weaken assertions. Follow golang-code-style.
- VERIFY: run \`cd ${ROOT} && ${ICU} go test -tags fts5 ./core/formats/${p.format}/ -count=1\` (and -run your new test). It MUST compile and pass. If it surfaces a real bug (panic / swallowed error), do NOT paper over it — report it in notes with test_passed=false and leave the failing test only if it documents a genuine defect the user should see (prefer reporting over committing red).

Return what you changed, the exact test command, and whether it passed.`
}

function publishPrompt(json, today) {
  return `Write the refreshed format-maturity dashboard dataset. cwd is the repo root (${ROOT}).

1. Get today's date: run \`date -u +%Y-%m-%d\`. Call it TODAY.
2. Take this EXACT JSON, replace the literal string __DATE__ with TODAY, and write it to web/docs/static/data/format-maturity.json (overwrite):

${json}

3. Update web/docs/static/data/format-maturity-history.json: Read it (a JSON array). Remove any entry whose "date" equals TODAY, then append {"date": TODAY, "total": <summary.total from the dataset>, "by_level": <summary.by_level from the dataset>}. Keep it sorted by date ascending. Write it back.
4. Verify both files are valid JSON (python3 -c "import json;json.load(open(...))"). Report the level distribution you published.`
}

// ── dimension normalization + dataset builder (mirror of the seed transform) ──
const CANON = ['reader', 'writer', 'config', 'spec', 'parity', 'malformed', 'corpus', 'detection', 'docs']
const LABELS = {
  reader: 'Reader', writer: 'Writer / round-trip', config: 'Config + Schema',
  spec: 'spec.yaml', parity: 'Parity', malformed: 'Malformed / robustness',
  corpus: 'Corpus breadth', detection: 'Detection', docs: 'Docs + wiring',
}
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
function ftype(s) {
  if (s.format === 'pdf') return 'read-only'
  if (s.format === 'splicedlines') return 'internal'
  const cp = String(s.okapi_counterpart || '')
  return (cp === '' || cp.startsWith('none') || cp.includes('harvest')) ? 'harvest' : 'parity'
}
function buildDataset(scores) {
  const formats = scores.slice().sort((a, b) => a.format.localeCompare(b.format)).map((s) => {
    const dims = {}
    for (const d of s.dimension_scores || []) {
      const k = normDim(d.dimension)
      if (k && !(k in dims)) dims[k] = d.score || 'none'
    }
    for (const k of CANON) if (!(k in dims)) dims[k] = 'none'
    const type = ftype(s)
    // harvest formats have no Okapi counterpart → spec/parity are not applicable
    if (type === 'harvest') {
      for (const k of ['spec', 'parity']) if (dims[k] === 'none') dims[k] = 'na'
    }
    if (type === 'read-only' && dims.writer === 'none') dims.writer = 'na'
    let cp = String(s.okapi_counterpart || '')
    if (cp.startsWith('none') || cp.includes('harvest') || cp.includes('internal')) cp = ''
    return {
      id: s.format, type, level: s.level, next_level: NEXT[s.level] || '—',
      okapi_counterpart: cp, dimensions: dims,
      blocking_gaps: (s.blocking_gaps || []).slice(0, 3),
      top_risk: s.top_risk || (s.correctness_risks && s.correctness_risks[0]) || '',
      confidence: s.confidence || '',
    }
  })
  const by_level = { L0: 0, L1: 0, L2: 0, L3: 0, L4: 0 }
  for (const f of formats) by_level[f.level] = (by_level[f.level] || 0) + 1
  return {
    generated_at: '__DATE__', target_level: TARGET, source: 'format-triage workflow (agent assessment)',
    summary: { total: formats.length, by_level }, dimensions: CANON, dimension_labels: LABELS, formats,
  }
}

// ── Phase: Score ──
phase('Score')
log(`Scoring ${FORMATS.length} formats (target ${TARGET}, mode ${MODE}).`)
const scores = (await parallel(
  FORMATS.map((f) => () => agent(scorePrompt(f), { label: f, phase: 'Score', schema: SCORE }).then((r) => ({ ...r, format: (r && r.format) || f })).catch(() => null))
)).filter(Boolean)
log(`Scored ${scores.length}/${FORMATS.length}.`)

// ── Phase: Triage ──
phase('Triage')
const plan = scores
  .filter((s) => (ORDER[s.level] ?? 9) < (ORDER[TARGET] ?? 4))
  .map((s) => ({ format: s.format, level: s.level, next_level: NEXT[s.level], blocking_gaps: s.blocking_gaps || [], top_risk: s.top_risk || '' }))
  .sort((a, b) => (ORDER[a.level] - ORDER[b.level]))
const dist = { L0: 0, L1: 0, L2: 0, L3: 0, L4: 0 }
for (const s of scores) dist[s.level] = (dist[s.level] || 0) + 1
log(`Distribution: ${JSON.stringify(dist)}. ${plan.length} formats below ${TARGET}.`)
for (const p of plan.slice(0, 10)) log(`  ${p.level}→${p.next_level} ${p.format}: ${(p.blocking_gaps[0] || '').slice(0, 80)}`)

// ── Phase: Remediate (optional) ──
let remediated = []
if (MODE === 'remediate' && plan.length) {
  phase('Remediate')
  const todo = LIMIT > 0 ? plan.slice(0, LIMIT) : plan
  log(`Remediating ${todo.length} formats (one verified improvement each).`)
  remediated = (await parallel(
    todo.map((p) => () => agent(remediatePrompt(p), { label: `fix:${p.format}`, phase: 'Remediate', schema: REMEDIATE }).catch(() => null))
  )).filter(Boolean)
  const passed = remediated.filter((r) => r.test_passed).length
  log(`Remediation: ${passed}/${remediated.length} verified green. Review the diff before committing.`)
}

// ── Phase: Publish ──
let dataset = buildDataset(scores)
if (PUBLISH) {
  phase('Publish')
  await agent(publishPrompt(JSON.stringify(dataset, null, 1), null), { label: 'publish', phase: 'Publish' })
  log('Published web/docs/static/data/format-maturity{,-history}.json — rebuild the docs to see it.')
}

return { distribution: dist, target: TARGET, plan, remediated, dataset }
