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
            extra !== undefined || !statSync(resolve(root, file)).isFile()) throw new Error('Invalid evidence file: ' + ref);
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
const states = rows => Object.fromEntries(rows.map(row =>
  [row.id, Object.fromEntries(row.capabilities.map(cap => [cap.id, cap.status]))]));

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

export function checkPublishedHistory(root, reference, date) {
  const git = args => execFileSync('git', args, { cwd: root, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024, stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  const commit = git(['rev-parse', '--verify', '--end-of-options', reference + '^{commit}']);
  const formatHistory = 'web/static/data/format-maturity-history.json';
  for (const file of [formatHistory, historyPath]) {
    const existsAtBase = git(['ls-tree', '--name-only', commit, '--', file]) === file;
    let baseline;
    if (existsAtBase) baseline = JSON.parse(git(['show', commit + ':' + file]));
    else if (file === historyPath) baseline = importFormatHistory(root, commit);
    else throw new Error('Missing format history at base ' + commit);
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
  const { connectorRows } = await import('./connectors.mjs');
  const current = { schema_version: 1, date,
    outputs: capabilityRows(root, await outputRows(root), 'output'),
    connectors: capabilityRows(root, await connectorRows(root), 'connector') };
  if (values.check && json(current) !== readFileSync(currentFile, 'utf8')) throw new Error('Stale ' + currentPath);

  execFileSync(process.execPath, ['scripts/format-ops/bootstrap-publish.mjs', '--date', date,
    ...(values.check ? ['--check'] : [])], { cwd: root, stdio: 'inherit' });
  const formats = formatLevels(read(resolve(root, formatPath)));
  const history = recordHistory(previous, { date, formats, outputs: states(current.outputs), connectors: states(current.connectors) });
  const projections = [[historyFile, json(history)], [currentFile, json(current)]];
  for (const [file, data] of projections) {
    if (existsSync(file) && readFileSync(file, 'utf8') === data) continue;
    if (values.check) throw new Error('Stale ' + file);
    const stage = file + '.tmp-' + process.pid;
    writeFileSync(stage, data);
    renameSync(stage, file);
  }
  console.log(`Content capabilities on ${date}: ${current.outputs.length} output formats, ${current.connectors.length} connectors.`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    await run(DEFAULT_ROOT, process.argv.slice(2));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
