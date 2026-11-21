import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, renameSync, statSync, writeFileSync } from 'node:fs';
import { isAbsolute, normalize, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { isDeepStrictEqual, parseArgs } from 'node:util';
import { DEFAULT_ROOT } from './lib.mjs';

const formatPath = 'web/static/data/format-maturity.json';
const currentPath = 'web/static/data/content-capabilities.json';
const historyPath = 'web/static/data/content-capabilities-history.json';
const statuses = new Set(['verified', 'declared', 'unverified', 'unsupported', 'failed']);
const json = value => JSON.stringify(value, null, 2) + '\n';
const read = file => JSON.parse(readFileSync(file, 'utf8'));

export function snapshotDate(date) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || !Number.isFinite(Date.parse(date)) ||
      new Date(date).toISOString().slice(0, 10) !== date) throw new Error('Invalid snapshot date: ' + date);
  return date;
}

function ordered(items, label) {
  if (!Array.isArray(items) || !items.length) throw new Error('Empty inventory: ' + label);
  const ids = new Set();
  for (const { id } of items) {
    if (typeof id !== 'string' || !/^[a-z0-9][a-z0-9._-]*$/.test(id) || ids.has(id)) {
      throw new Error('Invalid or duplicate ' + label + ' id: ' + id);
    }
    ids.add(id);
  }
  return [...items].sort((a, b) => a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
}

export function capabilityRows(root, rows, kind) {
  return ordered(rows, kind).map(({ id, capabilities }) => ({
    id,
    capabilities: ordered(capabilities, kind + '/' + id).map(cap => {
      if (!statuses.has(cap.status) || typeof cap.detail !== 'string' || !cap.detail.trim() ||
          !Array.isArray(cap.evidence) || (cap.status !== 'unverified' && !cap.evidence.length)) {
        throw new Error('Missing or invalid evidence for ' + kind + '/' + id + '/' + cap.id);
      }
      for (const ref of cap.evidence) {
        const [file, anchor, extra] = ref.split('#');
        if (!file || isAbsolute(file) || file.includes('\\') || normalize(file) !== file || file.startsWith('../') ||
            extra !== undefined || !existsSync(resolve(root, file)) || !statSync(resolve(root, file)).isFile()) throw new Error('Invalid evidence file: ' + ref);
        if (anchor !== undefined && (!/^[A-Za-z_]\w*$/.test(anchor) ||
            !new RegExp('\\b' + anchor + '\\b').test(readFileSync(resolve(root, file), 'utf8')))) {
          throw new Error('Missing evidence anchor: ' + ref);
        }
      }
      return { id: cap.id, status: cap.status, evidence: [...new Set(cap.evidence)].sort(), detail: cap.detail };
    }),
  }));
}

const formatLevels = data => Object.fromEntries(data.formats.map(row => [row.id, row.level]).sort());
export const states = rows => Object.fromEntries(rows.map(row =>
  [row.id, Object.fromEntries(row.capabilities.map(cap => [cap.id, cap.status]))]));

// Evidence states from weakest to strongest. A fresh probe that moves an
// operation down this order is a regression.
const strength = ['failed', 'unverified', 'unsupported', 'declared', 'verified'];

/**
 * Compare committed capability rows with a fresh probe of the same checkout.
 * A weaker status, or an operation of a measured subject that the probe no
 * longer reports, is a regression. A stronger status, a new subject or
 * operation, a removed subject and changed details or evidence are changes the
 * scheduled refresh records.
 */
export function capabilityRegressions(committed, fresh) {
  const now = new Map(fresh.map(row => [row.id, new Map(row.capabilities.map(cap => [cap.id, cap]))]));
  const regressions = [], changes = [];
  for (const row of committed) {
    const probed = now.get(row.id);
    if (!probed) { changes.push(row.id + ' is no longer measured'); continue; }
    for (const cap of row.capabilities) {
      const next = probed.get(cap.id);
      const name = row.id + '/' + cap.id;
      if (!next) regressions.push(name + ' is no longer measured');
      else if (strength.indexOf(next.status) < strength.indexOf(cap.status)) regressions.push(name + ' ' + cap.status + ' -> ' + next.status);
      else if (next.status !== cap.status) changes.push(name + ' ' + cap.status + ' -> ' + next.status);
      else if (next.detail !== cap.detail || next.evidence.join() !== cap.evidence.join()) changes.push(name + ' evidence changed');
    }
    for (const id of probed.keys()) if (!row.capabilities.some(cap => cap.id === id)) changes.push(row.id + '/' + id + ' is newly measured');
  }
  for (const id of now.keys()) if (!committed.some(row => row.id === id)) changes.push(id + ' is newly measured');
  return { regressions, changes };
}

/**
 * Settle one capability group. With check, the committed rows are the
 * publication: their evidence must resolve, they must be in canonical order,
 * and the fresh probe may not weaken them. Without check, the fresh probe is
 * the new publication.
 */
export function settleCapabilities({ root, check, file, kind, group, committed, fresh }) {
  const probed = capabilityRows(root, fresh, kind);
  if (!check) return probed;
  const rows = capabilityRows(root, committed?.[group] ?? [], kind);
  if (JSON.stringify(rows) !== JSON.stringify(committed[group])) throw new Error('Rows in ' + file + ' are not in canonical form.');
  const { regressions, changes } = capabilityRegressions(rows, probed);
  if (regressions.length) throw new Error('A fresh probe weakens committed ' + kind + ' evidence:\n  ' + regressions.join('\n  '));
  for (const change of changes) console.log('content-maturity: the next refresh records ' + kind + ' ' + change);
  return rows;
}

export function recordHistory(history, current) {
  let previous = '';
  for (const entry of history) {
    if (snapshotDate(entry.date) <= previous) throw new Error('Historical dates must be unique and increasing.');
    previous = entry.date;
  }
  if (snapshotDate(current.date) < previous) throw new Error('Cannot replace an earlier observation.');
  return [...history.filter(entry => entry.date !== current.date), current];
}

// Import only dates that have a published format dataset. Capability observations begin with their first probe.
export function importFormatHistory(root, revision = 'HEAD') {
  const git = args => execFileSync('git', args, { cwd: root, encoding: 'utf8', maxBuffer: 10 * 1024 * 1024 }).trim();
  const records = new Map();
  const revisions = git(['log', '--reverse', '--format=%H', revision, '--', formatPath]).split('\n').filter(Boolean);
  for (const revision of revisions) {
    const data = JSON.parse(git(['show', revision + ':' + formatPath]));
    const date = snapshotDate(data.generated_at.slice(0, 10));
    records.set(date, { date, revision, formats: formatLevels(data) });
  }
  return [...records.values()].sort((a, b) => a.date.localeCompare(b.date));
}

/**
 * Check that each history file only appends to its state at reference. A file
 * absent at the base takes its baseline from absent(commit), which throws when
 * the file is required.
 */
export function checkAppendOnly(root, reference, date, files) {
  const git = args => execFileSync('git', args, { cwd: root, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024, stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  const commit = git(['rev-parse', '--verify', '--end-of-options', reference + '^{commit}']);
  for (const [file, absent] of files) {
    const existsAtBase = git(['ls-tree', '--name-only', commit, '--', file]) === file;
    const baseline = existsAtBase ? JSON.parse(git(['show', commit + ':' + file])) : absent(commit);
    if (baseline.length && snapshotDate(baseline.at(-1).date) > date) {
      throw new Error('Snapshot predates the published history in ' + file);
    }
    const frozen = baseline.filter(entry => snapshotDate(entry.date) < date);
    const candidate = read(resolve(root, file));
    recordHistory(candidate, { date });
    if (!isDeepStrictEqual(frozen, candidate.filter(entry => entry.date < date))) {
      throw new Error('Previously published observations changed in ' + file);
    }
  }
}

export function checkPublishedHistory(root, reference, date) {
  checkAppendOnly(root, reference, date, [
    ['web/static/data/format-maturity-history.json', commit => { throw new Error('Missing format history at base ' + commit); }],
    [historyPath, commit => importFormatHistory(root, commit)],
  ]);
}

export async function run(root, args) {
  const { values } = parseArgs({ args, options: {
    check: { type: 'boolean' }, write: { type: 'boolean' }, date: { type: 'string' }, 'history-base': { type: 'string' },
  } });
  if (Boolean(values.check) === Boolean(values.write)) throw new Error('Choose --check or --write.');
  const currentFile = resolve(root, currentPath);
  const historyFile = resolve(root, historyPath);
  const date = snapshotDate(values.date ?? (values.check ? read(currentFile).date : new Date().toISOString().slice(0, 10)));
  const previous = existsSync(historyFile) ? read(historyFile) : importFormatHistory(root);
  if (values.check && !existsSync(historyFile)) throw new Error('Missing ' + historyPath);
  recordHistory(previous, { date });
  if (values['history-base']) checkPublishedHistory(root, values['history-base'], date);

  const { outputRows } = await import('./outputs.mjs');
  const committed = values.check ? read(currentFile) : undefined;
  const current = { schema_version: 1, date, outputs: settleCapabilities({ root, check: values.check, file: currentPath,
    kind: 'output', group: 'outputs', committed, fresh: await outputRows(root) }) };

  execFileSync(process.execPath, ['scripts/format-ops/bootstrap-publish.mjs', '--date', date,
    ...(values.check ? ['--check'] : [])], { cwd: root, stdio: 'inherit' });
  const formats = formatLevels(read(resolve(root, formatPath)));
  const history = recordHistory(previous, { date, formats, outputs: states(current.outputs) });
  const projections = [[historyFile, json(history)], [currentFile, json(current)]];
  for (const [file, data] of projections) {
    if (existsSync(file) && readFileSync(file, 'utf8') === data) continue;
    if (values.check) throw new Error('Stale ' + file);
    const stage = file + '.tmp-' + process.pid;
    writeFileSync(stage, data);
    renameSync(stage, file);
  }
  console.log(`Content capabilities on ${date}: ${current.outputs.length} output formats.`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    await run(DEFAULT_ROOT, process.argv.slice(2));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
