#!/usr/bin/env node
// due.mjs — format-ops due-ness report. Report tool, not a gate: ALWAYS exits 0.
//
// Reads docs/internals/format-ops-ledger.json + .skills/format-ops/artifacts.yaml,
// computes the cheap+offline live signals (git shas, artifact generated_at
// fields, file hashes), applies due() per docs/internals/format-ops.md §1:
//
//   due(ritual) = (cadence_days > 0 AND today − last_run > cadence_days)
//                 OR any(current(signal) ≠ watermark)
//
//   cadence_days: 0  → watermark-only (never due by time alone)
//   ci_owned: true   → watch-only (never executed; no cadence term)
//   blocked_on       → BLOCKED-ON-ISSUE passthrough (stays due, never executed)
//
// Signals needing the network (gh, GitLab REST, release tags) are reported as
// "needs-network" with the exact command, never failed on.
//
// Flags:
//   --model-id <id>   session model id; compared to the process-health
//                     calibration watermark — a mismatch is BLOCKING (§2.1)
//   --today <date>    override today (YYYY-MM-DD), for benchmarks/tests
//   --root <dir>      override repo root discovery
//
// Watermark conventions implemented here (stated, since §1 leaves them open):
//   - An EMPTY scalar watermark ("" / 0) is "unseeded": reported as a note,
//     not a mismatch — cadence governs until the first run seeds it. (This
//     preserves the §9.1 stagger; first-ever runs are still due by cadence.)
//   - DICT watermarks (manifest_shas, tracked_issues, per_spec) compare
//     per-key strictly: a live key absent from the watermark IS a mismatch
//     ("never consumed"), because that is the signal's whole point.
//   - The model check treats an empty calibration watermark as "calibration
//     never passed" → blocking when --model-id is provided.

import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

// ── helpers ────────────────────────────────────────────────────────────────

const scriptDir = path.dirname(fileURLToPath(import.meta.url));

function parseFlags(argv) {
  const flags = {};
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    const m = /^--([a-z-]+)(?:=(.*))?$/.exec(a);
    if (!m) continue;
    flags[m[1]] = m[2] !== undefined ? m[2] : argv[++i];
  }
  return flags;
}

function findRepoRoot(override) {
  if (override) return path.resolve(override);
  let d = scriptDir;
  while (d !== path.dirname(d)) {
    if (fs.existsSync(path.join(d, 'go.work'))) return d;
    d = path.dirname(d);
  }
  // Fallback: .skills/format-ops/scripts → repo root is three levels up.
  return path.resolve(scriptDir, '..', '..', '..');
}

function git(root, args) {
  try {
    return execFileSync('git', args, { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  } catch {
    return null;
  }
}

function sha256(buf) {
  return crypto.createHash('sha256').update(buf).digest('hex');
}

function sha256File(p) {
  try { return sha256(fs.readFileSync(p)); } catch { return null; }
}

function readJSON(p) {
  try { return JSON.parse(fs.readFileSync(p, 'utf8')); } catch { return null; }
}

function datePart(s) {
  return typeof s === 'string' ? s.slice(0, 10) : '';
}

function daysBetween(today, dateStr) {
  const a = Date.parse(today + 'T00:00:00Z');
  const b = Date.parse(datePart(dateStr) + 'T00:00:00Z');
  if (Number.isNaN(a) || Number.isNaN(b)) return null;
  return Math.floor((a - b) / 86400000);
}

// Minimal YAML-subset parser for artifacts.yaml: a top-level "artifacts:" list
// of flat "- key: value" maps with scalar values. No nesting, no inline
// comments after values (by convention in that file).
function parseArtifactsYaml(text) {
  const items = [];
  let cur = null;
  for (const raw of text.split('\n')) {
    const line = raw.replace(/\t/g, '  ');
    if (/^\s*#/.test(line) || !line.trim()) continue;
    if (/^artifacts:\s*$/.test(line)) continue;
    const item = /^\s*-\s+(\S+):\s*(.*)$/.exec(line);
    const cont = /^\s+(\S+):\s*(.*)$/.exec(line);
    if (item) {
      cur = {};
      items.push(cur);
      cur[item[1]] = scalar(item[2]);
    } else if (cont && cur) {
      cur[cont[1]] = scalar(cont[2]);
    }
  }
  return items;
  function scalar(v) {
    v = v.trim();
    if (v === 'null' || v === '~' || v === '') return null;
    if (v === 'true') return true;
    if (v === 'false') return false;
    if (/^-?\d+$/.test(v)) return parseInt(v, 10);
    if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) return v.slice(1, -1);
    return v;
  }
}

function globMatches(root, pattern) {
  // Supports the one shape artifacts.yaml uses: dir/*/file.ext
  const m = /^(.*)\/\*\/(.*)$/.exec(pattern);
  if (!m) return fs.existsSync(path.join(root, pattern)) ? [pattern] : [];
  const base = path.join(root, m[1]);
  if (!fs.existsSync(base)) return null; // glob root itself missing
  const out = [];
  for (const e of fs.readdirSync(base, { withFileTypes: true })) {
    if (!e.isDirectory()) continue;
    const p = path.join(base, e.name, m[2]);
    if (fs.existsSync(p)) out.push(path.join(m[1], e.name, m[2]));
  }
  return out;
}

// ── main ───────────────────────────────────────────────────────────────────

const flags = parseFlags(process.argv);
const root = findRepoRoot(flags.root);
const today = flags.today || new Date().toISOString().slice(0, 10);
const modelId = flags['model-id'] || null;

const report = { blocking: [], due: [], watch: [], blockedOnIssue: [], pending: [], ok: [], notes: [] };
const blockedByModel = new Set();

// 1. Path check (FIRST — a repo refactor must degrade to one clear error).
const artifactsPath = flags.artifacts
  ? path.resolve(flags.artifacts)
  : path.join(scriptDir, '..', 'artifacts.yaml');
let artifacts = [];
if (!fs.existsSync(artifactsPath)) {
  report.blocking.push(`path-drift: artifacts.yaml itself missing at ${artifactsPath}`);
} else {
  artifacts = parseArtifactsYaml(fs.readFileSync(artifactsPath, 'utf8'));
  for (const a of artifacts) {
    if (a.glob) {
      const matches = globMatches(root, a.path);
      if (matches === null) {
        report.blocking.push(`path-drift: ${a.id} glob root missing (${a.path})`);
      } else if (matches.length === 0) {
        report.notes.push(`${a.id}: 0 matches for ${a.path}` + (a.bootstrap ? ' (bootstrap §9.3 pending)' : ''));
      } else {
        report.notes.push(`${a.id}: ${matches.length} matches for ${a.path}`);
      }
      continue;
    }
    if (fs.existsSync(path.join(root, a.path))) continue;
    if (a.fetch_on_demand) {
      report.notes.push(`${a.id}: ${a.path} absent (fetch-on-demand — make fetch-corpus)`);
    } else {
      report.blocking.push(
        `path-drift: ${a.id} missing (${a.path}; owner ${a.owner})` +
        (a.bootstrap ? ' [bootstrap-pending — format-ops.md §9]' : ''),
      );
    }
  }
}
const artifactById = Object.fromEntries(artifacts.map((a) => [a.id, a]));

// 2. Ledger.
const ledgerPath = flags.ledger
  ? path.resolve(flags.ledger)
  : path.join(root, 'docs', 'internals', 'format-ops-ledger.json');
const ledger = readJSON(ledgerPath);
if (!ledger || !ledger.rituals) {
  report.blocking.push(`ledger missing or unparseable: ${ledgerPath}`);
  print(report, today, root, modelId);
  process.exit(0);
}
const R = ledger.rituals;
const runs = Array.isArray(ledger.runs) ? ledger.runs : [];

function hasRunSince(ritual, date) {
  return runs.some((r) => r.ritual === ritual && datePart(r.date) >= datePart(date));
}

// Shared cheap signals.
const dash = readJSON(path.join(root, artifactById['maturity-dashboard']?.path ?? 'web/static/data/format-maturity.json'));
const parity = readJSON(path.join(root, artifactById['parity-report']?.path ?? 'web/static/data/parity-report.json'));
const caudit = readJSON(path.join(root, artifactById['contract-audit-report']?.path ?? 'web/static/data/contract-audit.json'));

// 3. Model check (blocking per §2.1).
{
  const wm = R['process-health']?.watermarks?.model_id ?? '';
  if (modelId) {
    if (wm === '' || wm !== modelId) {
      report.blocking.push(
        `model-check: session model "${modelId}" ≠ calibration watermark "${wm || '(never calibrated)'}" — ` +
        `process-health calibration is due-and-blocking; triage-score, remediate, and prompt edits are ` +
        `BLOCKED until it passes against the adjudicated golden set`,
      );
      blockedByModel.add('triage-score').add('remediate');
    }
  } else {
    report.notes.push(`model-check: unverified — re-run with --model-id <session model id> (watermark: "${R['process-health']?.watermarks?.model_id || '(unseeded)'}")`);
  }
}

// 4. Per-ritual evaluation.
function evalCadence(r, ritual) {
  if (!r || r.cadence_days === 0) return null;
  if (r.last_run == null) return 'never run (cadence-due)';
  const d = daysBetween(today, r.last_run);
  if (d !== null && d > r.cadence_days) return `cadence-due (${d}d since ${r.last_run} > ${r.cadence_days}d)`;
  return null;
}

function scalarSignal(reasons, notes, label, current, watermark) {
  if (current == null) return;
  if (watermark === '' || watermark === 0 || watermark == null) {
    notes.push(`${label}: watermark unseeded (current: ${String(current).slice(0, 24)}…) — first run seeds it`);
  } else if (String(current) !== String(watermark)) {
    reasons.push(`${label}: ${String(current).slice(0, 40)} ≠ watermark ${String(watermark).slice(0, 40)}`);
  }
}

const results = {}; // ritual → {reasons, notes}
for (const id of Object.keys(R)) results[id] = { reasons: [], notes: [] };

// triage-score
{
  const r = R['triage-score'];
  const { reasons, notes } = results['triage-score'];
  const c = evalCadence(r, 'triage-score');
  if (c) reasons.push(c);
  scalarSignal(reasons, notes, 'core_formats_sha', git(root, ['log', '-1', '--format=%H', '--', 'core/formats']), r.watermarks.core_formats_sha);
  const dashVer = dash ? (dash.scorer_version ?? 1) : null;
  if (dashVer != null && dashVer !== r.watermarks.scorer_version) {
    reasons.push(`scorer_version drift: dashboard is v${dashVer}, ledger watermark v${r.watermarks.scorer_version} (multi-axis sweep not yet published)`);
  }
  notes.push('audit_sha not recomputed here (runs the full audit); the ritual recomputes: python3 .skills/refresh-format-maturity/scripts/audit-format.py --all --json | shasum -a 256');
  if (dash?.generated_at && r.last_run && datePart(dash.generated_at) > datePart(r.last_run) && !hasRunSince('triage-score', dash.generated_at)) {
    report.blocking.push(`reconcile/orphan: maturity-dashboard generated_at ${dash.generated_at} newer than triage-score last_run ${r.last_run} with no runs[] entry — adopt (outcome: adopted-orphan) before planning`);
  }
}

// remediate
{
  const r = R['remediate'];
  const { reasons, notes } = results['remediate'];
  const c = evalCadence(r);
  if (c) reasons.push(c);
  if ((r.carryover ?? []).length > 0) reasons.push(`carryover non-empty (${r.carryover.length} entries)`);
  scalarSignal(reasons, notes, 'dashboard_generated_at', dash?.generated_at ?? null, r.watermarks.dashboard_generated_at);
}

// parity-publish
{
  const r = R['parity-publish'];
  const { reasons, notes } = results['parity-publish'];
  const c = evalCadence(r);
  if (c) reasons.push(c);
  scalarSignal(reasons, notes, 'report_generated_at', parity?.generated_at ?? null, r.watermarks.report_generated_at);
  const since = r.watermarks.report_generated_at || r.last_run;
  if (since) {
    const changed = git(root, ['log', '-1', '--since', since, '--format=%H', '--', 'core/formats', 'cli/parity']);
    if (changed) reasons.push(`parity-affecting paths changed since report (${changed.slice(0, 12)} in core/formats|cli/parity)`);
  }
}

// contract-audit (ci_owned, watch-only)
{
  const r = R['contract-audit'];
  const { reasons, notes } = results['contract-audit'];
  scalarSignal(reasons, notes, 'generatedAt', caudit?.generatedAt ?? null, r.watermarks.generatedAt);
  scalarSignal(reasons, notes, 'okapiTag', caudit?.okapiTag ?? null, r.watermarks.okapiTag);
  notes.push('CI state: needs-network — gh run list --workflow=contract-audit.yml -L1 --json status,conclusion,updatedAt');
}

// upstream-watch
{
  const r = R['upstream-watch'];
  const { reasons, notes } = results['upstream-watch'];
  const c = evalCadence(r);
  if (c) reasons.push(c);
  notes.push('Okapi issues: needs-network — curl -s "https://gitlab.com/api/v4/projects/62298414/issues?order_by=updated_at&sort=desc&state=all&per_page=50"');
  notes.push('Okapi tags: needs-network — curl -s "https://gitlab.com/api/v4/projects/62298414/repository/tags?per_page=5" (pinned: ' + r.watermarks.okapi_pinned + ')');
  const month = parseInt(today.slice(5, 7), 10);
  if (month === 10 || month === 11) notes.push('annual deep window OPEN: Oct–Nov Unicode/CLDR/ICU release train');
}

// xfail-hygiene
{
  const r = R['xfail-hygiene'];
  const { reasons, notes } = results['xfail-hygiene'];
  const c = evalCadence(r);
  if (c) reasons.push(c);
  const tracked = Object.keys(r.watermarks.tracked_issues ?? {});
  if (tracked.length === 0) notes.push('no tracked issues watermarked yet — first run seeds from expected_fail annotations');
  else notes.push(`issue states: needs-network — gh issue view ${tracked.join(' ')} --json state,updatedAt`);
}

// corpus-census
{
  const r = R['corpus-census'];
  const { reasons, notes } = results['corpus-census'];
  const c = evalCadence(r);
  if (c) reasons.push(c);
  const manifests = globMatches(root, 'core/formats/*/corpus.yaml') ?? [];
  for (const rel of manifests) {
    const fmt = rel.split('/')[2];
    const cur = sha256File(path.join(root, rel));
    const wm = (r.watermarks.manifest_shas ?? {})[fmt];
    if (wm == null) reasons.push(`corpus.yaml for ${fmt} never census-verified`);
    else if (wm !== cur) reasons.push(`corpus.yaml sha mismatch for ${fmt}`);
  }
  for (const fmt of Object.keys(r.watermarks.manifest_shas ?? {})) {
    if (!manifests.some((m) => m.split('/')[2] === fmt)) reasons.push(`watermarked manifest for ${fmt} no longer exists`);
  }
  notes.push('release respin: needs-network — gh release view ' + (r.watermarks.release_tag || 'format-corpus-v1') + ' --json tagName,assets');
}

// corpus-sweep / case-gen (blocked_on passthrough handled at classification)
for (const id of ['corpus-sweep', 'case-gen']) {
  const r = R[id];
  const c = evalCadence(r);
  if (c) results[id].reasons.push(c);
}

// format-radar
{
  const c = evalCadence(R['format-radar']);
  if (c) results['format-radar'].reasons.push(c);
}

// process-health
{
  const r = R['process-health'];
  const { reasons, notes } = results['process-health'];
  const c = evalCadence(r);
  if (c) reasons.push(c);
  const skillDir = path.join(scriptDir, '..');
  const promptFiles = [path.join(skillDir, 'SKILL.md')];
  const refDir = path.join(skillDir, 'references');
  if (fs.existsSync(refDir)) for (const f of fs.readdirSync(refDir).sort()) promptFiles.push(path.join(refDir, f));
  const promptSha = sha256(promptFiles.map((f) => { try { return fs.readFileSync(f, 'utf8'); } catch { return ''; } }).join('\n'));
  scalarSignal(reasons, notes, 'prompt_sha', promptSha, r.watermarks.prompt_sha);
  scalarSignal(reasons, notes, 'rubric_sha', sha256File(path.join(root, 'docs/internals/format-maturity.md')), r.watermarks.rubric_sha);
  const learnings = sha256File(path.join(root, 'docs/internals/format-ops-learnings.md'));
  if (learnings) scalarSignal(reasons, notes, 'learnings_sha', learnings, r.watermarks.learnings_sha);
  if (Object.keys(r.adjudicated?.grades ?? {}).length === 0) {
    notes.push('golden set not yet adjudicated (bootstrap §9.4) — calibration cannot pass until the maintainer grades it');
  }
  if (blockedByModel.size > 0) reasons.push('model changed → calibration phase due-and-blocking (§2.1)');
}

// tier-review (watermark-only)
{
  const r = R['tier-review'];
  const { reasons, notes } = results['tier-review'];
  const supportPath = path.join(root, 'core/formats/support.yaml');
  if (fs.existsSync(supportPath)) {
    scalarSignal(reasons, notes, 'support_sha', sha256File(supportPath), r.watermarks.support_sha);
  } else {
    notes.push('support.yaml absent — seeding it is bootstrap §9.2, owned by tier-review');
  }
  for (const row of dash?.formats ?? []) {
    const lc = row?.tier?.last_certified;
    if (lc && daysBetween(today, lc) > 120) reasons.push(`certification decayed for ${row.id} (last_certified ${lc} > 120d)`);
  }
  const countersigns = (ledger.pending ?? []).filter((p) => p.type === 'na-countersign');
  if (countersigns.length > 0) reasons.push(`${countersigns.length} na-countersign request(s) pending`);
}

// 5. Classification.
const rank = ['triage-score', 'parity-publish', 'remediate', 'xfail-hygiene', 'upstream-watch',
  'corpus-census', 'corpus-sweep', 'case-gen', 'format-radar', 'tier-review', 'process-health'];
const order = (id) => { const i = rank.indexOf(id); return i === -1 ? 99 : i; };

for (const id of Object.keys(R).sort((a, b) => order(a) - order(b))) {
  const r = R[id];
  const { reasons, notes } = results[id];
  if (r.blocked_on) {
    report.blockedOnIssue.push(`${id}  ${r.blocked_on}` +
      (reasons.length ? `  [also: ${reasons.join('; ')} — stays due while blocked]` : '') +
      `  (re-check: gh issue view ${r.blocked_on.split('/').pop()} --json state -R neokapi/neokapi → needs-network)`);
    continue;
  }
  if (r.ci_owned) {
    const stale = reasons.length > 0;
    report.watch.push(`${id}  ${stale ? 'FINDING — stale vs watermark: ' + reasons.join('; ') + ' → raise finding; fix delegated to remediate' : 'artifact matches watermark'}${notes.length ? '  | ' + notes.join(' | ') : ''}`);
    continue;
  }
  if (blockedByModel.has(id)) {
    // Listed inside the model-check BLOCKING finding; keep out of DUE.
    report.notes.push(`${id}: blocked by model-check (calibration first)` + (reasons.length ? ` — pent-up: ${reasons.join('; ')}` : ''));
    continue;
  }
  if (reasons.length > 0) {
    report.due.push(`${id}  ${reasons.join('; ')}${notes.length ? '\n      note: ' + notes.join('\n      note: ') : ''}`);
  } else {
    report.ok.push(`${id}${notes.length ? '  (' + notes.join(' | ') + ')' : ''}`);
  }
}

for (const p of ledger.pending ?? []) {
  report.pending.push(`[${p.id}] ${p.type} (${p.ritual}): ${typeof p.proposal === 'string' ? p.proposal : JSON.stringify(p.proposal)} (created ${p.created}${p.expires ? ', expires ' + p.expires : ''})`);
}

print(report, today, root, modelId);
process.exit(0);

// ── output ─────────────────────────────────────────────────────────────────

function print(rep, today, root, modelId) {
  const lines = [];
  lines.push(`format-ops due report — ${today}  (root: ${root}${modelId ? `, model: ${modelId}` : ''})`);
  lines.push('');
  section('BLOCKING', rep.blocking, '(none)');
  section('DUE  (ranked per format-ops.md §3; budget: 2 heavy rituals/run by default)', rep.due, '(none)');
  section('WATCH  (ci_owned — never executed by the skill)', rep.watch, '(none)');
  section('BLOCKED-ON-ISSUE  (safe, durable; unblocks when the issue closes)', rep.blockedOnIssue, '(none)');
  section('PENDING-APPROVALS  (present as one batch at end of run)', rep.pending, '(none)');
  section('OK', rep.ok, '(none)');
  if (rep.notes.length) section('notes', rep.notes, '');
  lines.push('exit 0 — this is a report, not a gate.');
  process.stdout.write(lines.join('\n') + '\n');

  function section(title, items, empty) {
    lines.push(title);
    if (items.length === 0) { if (empty) lines.push('  ' + empty); } else for (const i of items) lines.push('  ' + i);
    lines.push('');
  }
}
