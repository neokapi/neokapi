// Record a native context operation lifecycle in a disposable workspace.
import { spawnSync, execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cpSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../..');
const args = process.argv.slice(2);
const option = (name, fallback) => {
  const i = args.indexOf(name);
  if (i < 0) return fallback;
  if (!args[i + 1]) throw new Error(`${name} requires a value`);
  return args[i + 1];
};
const binary = resolve(option('--binary', join(repo, 'bin/kapi')));
const output = resolve(option('--output', join(tmpdir(), 'context-lifecycle.json')));
const sandbox = mkdtempSync(join(tmpdir(), 'kapi-context-lifecycle-'));
const hash = (bytes) => createHash('sha256').update(bytes).digest('hex');
const env = {
  ...process.env, KAPI_NO_PROJECT: '1', KAPI_PROJECT: '', KAPI_ACTOR: 'person',
  KAPI_CONFIG_DIR: join(sandbox, 'config'), KAPI_DATA_DIR: join(sandbox, 'data'),
  XDG_DATA_HOME: join(sandbox, 'xdg'), XDG_CACHE_HOME: join(sandbox, 'cache'),
  KAPI_PLUGINS_DIR_ONLY: '1', KAPI_PLUGINS_DIR: join(sandbox, 'plugins'), KAPI_TELEMETRY: '0',
};
const normalize = (v) => JSON.parse(JSON.stringify(v).replaceAll(sandbox, '<fixture>'));
const report = {
  schema: 'neokapi-context-lifecycle/v1', recordedAt: new Date().toISOString(),
  sourceCommit: execFileSync('git', ['rev-parse', 'HEAD'], {cwd: repo, encoding: 'utf8'}).trim(),
  sourceDirty: Boolean(execFileSync('git', ['status', '--porcelain'], {cwd: repo, encoding: 'utf8'}).trim()),
  binarySha256: hash(readFileSync(binary)),
  runnerSha256: hash(readFileSync(fileURLToPath(import.meta.url))),
  recipeSha256: hash(readFileSync(join(here, 'kapi.yaml'))), steps: [],
};
function run(command, expectedExit = 0) {
  const result = spawnSync(binary, command, {cwd: sandbox, env, encoding: 'utf8', timeout: 30000});
  const record = {command, exitCode: result.status, stdout: result.stdout ?? '', stderr: result.stderr ?? ''};
  if (result.error || result.status !== expectedExit) throw new Error(JSON.stringify(record));
  return record;
}
function snapshot(label, action) {
  const context = run(['context', 'guide.json', '-p', 'kapi.yaml', '--json']);
  const log = run(['context', 'log', '-p', 'kapi.yaml', '--json']);
  const search = run(['context', 'search', 'Quickcast', '-p', 'kapi.yaml', '--json']);
  const expectedExit = ['Established', 'Source edited', 'Source read again'].includes(label) ? 3 : 0;
  const check = run(['check', 'guide.json', '-p', 'kapi.yaml', '--json'], expectedExit);
  const checked = JSON.parse(check.stdout);
  if (checked.pass !== (expectedExit === 0)) throw new Error(`Unexpected check verdict at ${label}`);
  const source = readFileSync(join(sandbox, 'guide.json'), 'utf8');
  report.steps.push({label, action, source, sourceSha256: hash(source), context, log, check, search});
}
try {
  for (const file of ['kapi.yaml', 'guide.json']) cpSync(join(here, file), join(sandbox, file));
  report.binaryVersion = run(['--version']).stdout.trim();
  report.input = JSON.parse(readFileSync(join(here, 'guide.json'), 'utf8'));
  report.recipe = readFileSync(join(here, 'kapi.yaml'), 'utf8');
  snapshot('Before observation', run(['up', '--passes', '1', '-p', 'kapi.yaml']));
  const observation = run(['context', 'observe', '--term', 'Quickcast', '--instead-of', 'Quick cast', '--seen-in', 'guide.json', '-p', 'kapi.yaml', '--json']);
  const observed = JSON.parse(observation.stdout);
  const id = observed.operation?.id ?? observed.id;
  if (!id) throw new Error(`Missing observation id: ${observation.stdout}`);
  snapshot('Suggested', observation);
  snapshot('Established', run(['context', 'keep', String(id), '-p', 'kapi.yaml', '--json']));
  writeFileSync(join(sandbox, 'guide.json'), JSON.stringify({...report.input, description: 'Quick cast forecasts the next two hours.'}, null, 2) + '\n');
  snapshot('Source edited', {command: [], kind: 'fixture-file-edit', exitCode: 0, stdout: 'Changed the source description from one hour to two hours.', stderr: ''});
  snapshot('Source read again', run(['up', '--passes', '1', '-p', 'kapi.yaml']));
  snapshot('Reverted', run(['context', 'revert', String(id), '-p', 'kapi.yaml', '--json']));
  const state = (label) => JSON.parse(report.steps.find((step) => step.label === label).context.stdout);
  if (!state('Suggested').suggestions?.some((item) => item.operation === String(id))) throw new Error('Observation was not returned as a suggestion');
  if (!state('Established').terms?.some((item) => item.term === 'Quick cast' && item.status === 'forbidden')) throw new Error('Kept rule was not established');
  if (state('Source edited').provenance?.stale !== true || state('Source read again').provenance?.stale !== false) throw new Error('Source freshness did not update after the native run');
  const searched = JSON.parse(report.steps.find((step) => step.label === 'Source read again').search.stdout);
  if (!searched.terms?.some((term) => term.uses > 0 && term.top_uses?.some((use) => use.document === 'guide.json'))) throw new Error('The refreshed graph returned no term occurrences');
  if (state('Reverted').terms?.some((item) => item.term === 'Quick cast' && item.status === 'forbidden')) throw new Error('Reverted rule is still established');
} catch (error) {
  report.failure = String(error);
  process.exitCode = 1;
} finally {
  writeFileSync(output, JSON.stringify(normalize(report), null, 2) + '\n');
  rmSync(sandbox, {recursive: true, force: true});
}
