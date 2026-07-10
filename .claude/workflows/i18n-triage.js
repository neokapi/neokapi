export const meta = {
  name: 'i18n-triage',
  description: 'Sweep every framework in the kapi skill\'s i18n registry (cli/skills/data/kapi/references/i18n/frameworks.yaml) for ecosystem drift — new majors, deprecations, stewardship changes — re-score the Toil Index with citations, and optionally apply the updates to the registry and playbooks',
  whenToUse: 'Run quarterly, or after a big ecosystem event (framework major, library abandonment/revival). Default (no args) = sweep + triage report. args: {mode:"triage|apply", frameworks:[ids], limit:N}',
  phases: [
    { title: 'Prep', detail: 'one agent: parse frameworks.yaml into the entry list (id, watch watermarks, toil, status)' },
    { title: 'Sweep', detail: 'one web-research agent per framework: verify the watch claims, find drift since `verified`, propose cited re-scores' },
    { title: 'Triage', detail: 'in-script: suppress uncited score moves (sticky anchor), rank drift' },
    { title: 'Apply', detail: '(mode=apply) one agent per drifted framework: update registry entry + playbook, bump verified, validate' },
  ],
}

function parseArgs(a) {
  if (a && typeof a === 'object' && !Array.isArray(a)) return a
  if (typeof a === 'string' && a.trim()) {
    try {
      const p = JSON.parse(a)
      if (p && typeof p === 'object' && !Array.isArray(p)) return p
    } catch (e) { /* defaults */ }
  }
  return {}
}
const cfg = parseArgs(args)
const MODE = cfg.mode || 'triage' // 'triage' | 'apply'
const ONLY = Array.isArray(cfg.frameworks) ? cfg.frameworks : null
const LIMIT = typeof cfg.limit === 'number' ? cfg.limit : 0

const REGISTRY = 'cli/skills/data/kapi/references/i18n/frameworks.yaml'
const RUBRIC = 'docs/internals/i18n-toil.md'

// ── Phase 1: Prep — parse the registry ─────────────────────────────────────
phase('Prep')
const ENTRIES_SCHEMA = {
  type: 'object',
  required: ['entries'],
  properties: {
    entries: {
      type: 'array',
      items: {
        type: 'object',
        required: ['id', 'ecosystem', 'reference', 'status', 'grade', 'verified'],
        properties: {
          id: { type: 'string' },
          ecosystem: { type: 'string' },
          reference: { type: 'string' },
          status: { type: 'string' },
          grade: { type: 'string' },
          verified: { type: 'string' },
          latest_verified: { type: 'string' },
          repo: { type: 'string' },
          toil_recommended: { type: 'string', description: 'compact "w,x,s,r,e" of the recommended vector' },
        },
      },
    },
  },
}
const prep = await agent(
  `Read ${REGISTRY} and return every framework entry as structured output: id, ecosystem, reference, status, toil.grade as grade, watch.verified as verified, watch.latest_verified, watch.repo, and toil.recommended as a compact "w,x,s,r,e" string. Do not editorialize; this is a mechanical parse.`,
  { label: 'parse-registry', schema: ENTRIES_SCHEMA },
)
if (!prep) throw new Error('Prep failed: could not parse the registry')
let entries = prep.entries
if (ONLY) entries = entries.filter((e) => ONLY.includes(e.id))
if (LIMIT > 0) entries = entries.slice(0, LIMIT)
log(`Sweeping ${entries.length} frameworks (mode=${MODE})`)

// ── Phase 2+3: Sweep (one agent per framework), then triage in-script ──────
const DRIFT_SCHEMA = {
  type: 'object',
  required: ['id', 'drift', 'findings'],
  properties: {
    id: { type: 'string' },
    drift: { type: 'string', enum: ['none', 'minor', 'major'] },
    findings: {
      type: 'array',
      items: {
        type: 'object',
        required: ['claim', 'evidence_url'],
        properties: {
          claim: { type: 'string', description: 'what changed since the verified date' },
          evidence_url: { type: 'string' },
        },
      },
    },
    proposed_status: { type: 'string', description: 'recommended|supported|caution|avoid — only if it should change, else empty' },
    proposed_toil: { type: 'string', description: 'compact "w,x,s,r,e" recommended vector — only if it should change, else empty' },
    proposed_latest: { type: 'string', description: 'new latest_verified watermark string' },
    summary: { type: 'string' },
  },
}

const reports = (
  await parallel(
    entries.map((e) => () =>
      agent(
        `You are auditing one entry of the kapi skill's i18n framework registry for drift. Today's date is in the environment. Entry:
id=${e.id}, ecosystem=${e.ecosystem}, status=${e.status}, grade=${e.grade}, recommended toil vector (w,x,s,r,e)=${e.toil_recommended}, last verified=${e.verified}, watermark="${e.latest_verified}", watch repo=${e.repo}.

Use WebSearch/WebFetch to check, with sources:
1. Latest stable version / release cadence vs the watermark. New major since ${e.verified}?
2. Maintenance signals: deprecation, abandonment, revival, stewardship change, license change.
3. Breaking changes affecting the adoption playbook (extraction tool changes, catalog format changes, renamed packages).
4. Anything that would move a Toil Index axis per the rubric axes (marking/extraction/sync/runtime-correctness/ecosystem risk, each 0-3). Propose a new vector ONLY with a citation per moved axis — uncited moves will be discarded.

Classify drift: none (watermark still current), minor (patch/minor releases, no playbook impact), major (playbook or score or status should change). Return structured output only.`,
        { label: `sweep:${e.id}`, phase: 'Sweep', schema: DRIFT_SCHEMA },
      ),
    ),
  )
).filter(Boolean)

phase('Triage')
// Sticky anchor: a proposed score/status change with no findings citations is suppressed.
const triaged = reports.map((r) => {
  const cited = (r.findings || []).some((f) => f.evidence_url && f.evidence_url.startsWith('http'))
  const suppressed = (r.proposed_toil || r.proposed_status) && !cited
  return { ...r, suppressed: !!suppressed }
})
const major = triaged.filter((r) => r.drift === 'major' && !r.suppressed)
const minor = triaged.filter((r) => r.drift === 'minor')
const clean = triaged.filter((r) => r.drift === 'none')
log(`Drift: ${major.length} major, ${minor.length} minor, ${clean.length} clean, ${triaged.filter((r) => r.suppressed).length} suppressed (uncited)`)

// ── Phase 4: Apply (optional) ───────────────────────────────────────────────
let applied = []
if (MODE === 'apply' && major.length) {
  phase('Apply')
  applied = (
    await parallel(
      major.map((r) => () => {
        const entry = entries.find((e) => e.id === r.id)
        return agent(
          `Apply a cited drift update to the kapi skill's i18n knowledge for framework "${r.id}".

Findings (each has an evidence URL — carry them into the edit rationale, not into the shipped files):
${JSON.stringify(r.findings, null, 2)}
Proposed: status=${r.proposed_status || '(unchanged)'}, toil(recommended w,x,s,r,e)=${r.proposed_toil || '(unchanged)'}, latest_verified=${r.proposed_latest || '(unchanged)'}.

Steps:
1. Read ${RUBRIC} §2-§3 (scoring criteria + rules) and the registry ${REGISTRY}.
2. Update the "${r.id}" entry: watch.latest_verified, watch.verified (today), and — only where the findings justify it per the rubric — toil vectors, grade (recompute per §1 from the recommended vector), status, buys, notes.
3. Update the matching playbook cli/skills/data/kapi/references/i18n/${entry ? entry.reference : ''} so its prose, grade table, and setup steps agree with the registry again. Keep the file's existing structure; change only what the findings require.
4. Validate: node -e "require('./node_modules/js-yaml').load(require('fs').readFileSync('${REGISTRY}','utf8'))" (repo root) must pass; grades in the playbook table must match the registry.
5. IMPORTANT change control (rubric §3.7): do not edit ${RUBRIC} or this workflow in the same change.

Return a summary of exactly what changed.`,
          { label: `apply:${r.id}`, phase: 'Apply' },
        )
      }),
    )
  ).filter(Boolean)
}

return {
  swept: triaged.length,
  major: major.map((r) => ({ id: r.id, summary: r.summary, findings: r.findings, proposed_status: r.proposed_status, proposed_toil: r.proposed_toil })),
  minor: minor.map((r) => ({ id: r.id, summary: r.summary })),
  suppressed: triaged.filter((r) => r.suppressed).map((r) => r.id),
  applied,
  note: MODE === 'triage' ? 'Run again with args {mode:"apply"} to land the major-drift updates.' : `${applied.length} updates applied — review with git diff, then bump verified dates for clean entries if desired.`,
}
