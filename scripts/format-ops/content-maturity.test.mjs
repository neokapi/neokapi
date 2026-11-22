import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { execFileSync } from 'node:child_process';
import { assertTierMinimum, formatRegressions, formatTreeDigest, nextAxisGaps } from './lib.mjs';
import { capabilityRegressions, capabilityRows, checkPublishedHistory, importFormatHistory, recordHistory, settleCapabilities, snapshotDate } from './content-maturity.mjs';

function directory(t) {
  const root = mkdtempSync(path.join(tmpdir(), 'maturity-test-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}

test('published observations require a unique subject, operation and resolvable evidence', t => {
  const root = directory(t);
  writeFileSync(path.join(root, 'format_test.go'), 'func TestProjection(t *testing.T) {}');
  const cap = { id: 'project', status: 'verified', detail: 'Projected both fields.', evidence: ['format_test.go#TestProjection'] };
  const subject = { id: 'markdown', capabilities: [cap] };
  const rows = (...caps) => capabilityRows(root, [{ ...subject, capabilities: caps }], 'output');
  assert.deepEqual(rows(cap), [subject]);
  assert.throws(() => rows({ ...cap, evidence: [] }), /evidence/);
  assert.throws(() => rows({ ...cap, status: 'passed' }), /evidence/);
  assert.throws(() => rows({ ...cap, evidence: ['../format_test.go'] }), /Invalid evidence file/);
  assert.throws(() => rows({ ...cap, evidence: ['format_test.go#TestAbsent'] }), /Missing evidence anchor/);
  assert.throws(() => rows(cap, cap), /duplicate/);
  assert.throws(() => capabilityRows(root, [subject, subject], 'output'), /duplicate/);
  assert.throws(() => capabilityRows(root, [], 'connector'), /Empty inventory/);
  assert.equal(rows({ ...cap, status: 'unverified', evidence: [] })[0].capabilities[0].status, 'unverified');
});

test('history preserves earlier measurements and replaces only a same-day rerun', () => {
  const first = { date: '2026-06-20', formats: { json: 'L1' } };
  const next = { date: '2026-06-21', formats: { json: 'L2' } };
  const history = recordHistory([first], next);
  assert.deepEqual(recordHistory(history, next), history);
  assert.deepEqual(history[0], first);
  assert.equal(history[0].outputs, undefined);
  assert.throws(() => recordHistory(history, first), /earlier observation/);
  assert.throws(() => recordHistory([first, first], next), /unique/);
  for (const invalid of ['2026-02-29', '2026-01-32', '2026-01-01T00:00:00Z', undefined]) {
    assert.throws(() => snapshotDate(invalid), /Invalid/);
  }
});

test('Git import uses the last actual publication per day and keeps unmeasured profiles absent', t => {
  const root = directory(t);
  const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8' }).trim();
  git('init', '-q');
  git('config', 'user.name', 'Publisher');
  git('config', 'user.email', 'test@example.invalid');
  git('config', 'commit.gpgsign', 'false');
  const dir = path.join(root, 'web/static/data');
  mkdirSync(dir, { recursive: true });
  function publish(date, level) {
    writeFileSync(path.join(dir, 'format-maturity.json'), JSON.stringify({
      generated_at: date, formats: [{ id: 'json', level }],
    }));
    git('add', 'web');
    git('commit', '-qm', 'Record format evidence');
    return git('rev-parse', 'HEAD');
  }
  publish('2026-06-20', 'L0');
  const corrected = publish('2026-06-20', 'L1');
  const next = publish('2026-06-21', 'L2');
  assert.deepEqual(importFormatHistory(root), [
    { date: '2026-06-20', revision: corrected, formats: { json: 'L1' } },
    { date: '2026-06-21', revision: next, formats: { json: 'L2' } },
  ]);
});

test('the tier minimum uses each required axis and honors explicit grandfathering', () => {
  const levels = { engine: 'L3', corpus: 'C2', knowledge: 'K2' };
  const check = (tier, grades) => assertTierMinimum('test', { tier }, grades);
  check('available', { engine: 'L0' });
  check('maintained', { engine: 'L2' });
  check('supported', levels);
  for (const [axis, grade] of [['engine', 'L2'], ['corpus', 'C1'], ['knowledge', 'K1'], ['knowledge', undefined]]) {
    assert.throws(() => check('supported', { ...levels, [axis]: grade }), new RegExp('requires ' + axis));
  }
  assert.throws(() => check('maintained', { engine: 'L1' }), /requires engine L2/);
  assert.throws(() => check('maintained', { engine: 'K2' }), /requires engine L2/);
  assert.doesNotThrow(() => assertTierMinimum('test', { tier: 'maintained', grandfathered: true }, { engine: 'L1' }));
});

test('a format watermark hashes tracked files only', t => {
  const root = directory(t);
  const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8' }).trim();
  git('init', '-q');
  const dir = path.join(root, 'core/formats');
  mkdirSync(dir, { recursive: true });
  writeFileSync(path.join(dir, 'reader.go'), 'package test');
  git('add', 'core');
  git('-c', 'user.name=Publisher', '-c', 'user.email=test@example.invalid', '-c', 'commit.gpgsign=false', 'commit', '-qm', 'Add format');
  const committed = formatTreeDigest(root);
  writeFileSync(path.join(dir, 'scratch.go'), 'package scratch');
  assert.equal(formatTreeDigest(root), committed);
  writeFileSync(path.join(dir, 'reader.go'), 'package changed');
  assert.notEqual(formatTreeDigest(root), committed);
});

test('check mode fails a weaker capability and leaves a stronger one to the refresh', () => {
  const row = (id, ...caps) => ({ id, capabilities: caps.map(([cap, status]) => ({ id: cap, status, evidence: ['a_test.go#TestA'], detail: 'Fixture.' })) });
  const committed = [row('markdown', ['project-text', 'verified'], ['structure', 'unverified']), row('xml', ['project-text', 'failed'])];
  assert.deepEqual(capabilityRegressions(committed, committed), { regressions: [], changes: [] });
  assert.deepEqual(capabilityRegressions(committed, [row('markdown', ['project-text', 'failed'], ['structure', 'unverified']), committed[1]]).regressions,
    ['markdown/project-text verified -> failed']);
  assert.deepEqual(capabilityRegressions(committed, [row('markdown', ['project-text', 'verified']), committed[1]]).regressions,
    ['markdown/structure is no longer measured']);
  const better = capabilityRegressions(committed, [row('markdown', ['project-text', 'verified'], ['structure', 'verified']),
    row('xml', ['project-text', 'verified']), row('eml', ['email', 'verified'])]);
  assert.deepEqual(better.regressions, []);
  assert.deepEqual(better.changes, ['markdown/structure unverified -> verified', 'xml/project-text failed -> verified', 'eml is newly measured']);
});

test('check mode settles on the committed rows, never on a fresh probe', t => {
  const root = directory(t);
  writeFileSync(path.join(root, 'a_test.go'), 'func TestA(t *testing.T) {}');
  const cap = status => ({ id: 'project-text', status, evidence: ['a_test.go#TestA'], detail: 'Fixture.' });
  const committed = { outputs: [{ id: 'markdown', capabilities: [cap('unverified')] }] };
  const settle = (fresh, published = committed) => settleCapabilities({ root, check: true, file: 'data.json', kind: 'output',
    group: 'outputs', committed: published, fresh: [{ id: 'markdown', capabilities: [cap(fresh)] }] });
  assert.deepEqual(settle('verified'), committed.outputs);
  assert.throws(() => settle('failed'), /weakens committed output evidence/);
  assert.throws(() => settle('unverified', { outputs: [{ id: 'markdown', capabilities: [{ ...cap('verified'), evidence: ['gone_test.go'] }] }] }), /Invalid evidence file/);
  assert.deepEqual(settleCapabilities({ root, check: false, kind: 'output', fresh: [{ id: 'markdown', capabilities: [cap('failed')] }] })[0].capabilities[0].status, 'failed');
});

test('check mode fails a lower format grade and reports a higher one', () => {
  const axes = { engine: ['L0', 'L1', 'L2'], corpus: ['C0', 'C1'] };
  const row = (id, engine, corpus, gaps = []) => ({ id, levels: { engine, corpus }, blocking_gaps: gaps });
  const committed = [row('eml', 'L1', 'C1'), row('srt', 'L2', 'C0')];
  assert.deepEqual(formatRegressions(committed, committed, axes), { regressions: [], changes: [] });
  assert.deepEqual(formatRegressions(committed, [row('eml', 'L0', 'C1'), committed[1]], axes).regressions, ['eml engine L1 -> L0']);
  const raised = formatRegressions(committed, [row('eml', 'L2', 'C1', ['add a test']), committed[1], row('msg', 'L0', 'C0')], axes);
  assert.deepEqual(raised.regressions, []);
  assert.deepEqual(raised.changes, ['eml engine L1 -> L2', 'eml blocking gaps changed', 'msg is newly measured']);
});

test('a review preserves prior observations across both histories and accepts a same-day refresh', t => {
  const root = directory(t);
  const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8' }).trim();
  git('init', '-q');
  const dir = path.join(root, 'web/static/data');
  mkdirSync(dir, { recursive: true });
  const paths = ['format-maturity-history.json', 'content-capabilities-history.json'];
  const put = (file, data) => writeFileSync(path.join(dir, file), JSON.stringify(data));
  const first = { date: '2026-11-01', total: 1 };
  const latest = { date: '2026-11-02', total: 2 };
  put(paths[0], [first, latest]);
  put('format-maturity.json', { generated_at: latest.date, formats: [{ id: 'eml', level: 'L1' }] });
  git('add', 'web');
  const commit = () => git('-c', 'user.name=Publisher', '-c', 'user.email=test@example.invalid', '-c', 'commit.gpgsign=false', 'commit', '-qm', 'Publish history');
  commit();
  let base = git('rev-parse', 'HEAD');
  put(paths[1], importFormatHistory(root));
  checkPublishedHistory(root, base, '2026-11-03');
  git('add', 'web');
  commit();
  base = git('rev-parse', 'HEAD');
  for (const file of paths) {
    const old = JSON.parse(git('show', base + ':web/static/data/' + file));
    put(file, [...old, { date: '2026-11-03', invented: true }]);
    assert.throws(() => checkPublishedHistory(root, base, '2026-11-04'), /Previously published/);
    put(file, old.slice(0, -1));
    assert.throws(() => checkPublishedHistory(root, base, '2026-11-03'), /Previously published/);
    put(file, [...old.slice(0, -1), { ...old.at(-1), changed: true }]);
    assert.throws(() => checkPublishedHistory(root, base, '2026-11-03'), /Previously published/);
    checkPublishedHistory(root, base, latest.date);
    put(file, old);
  }
  assert.throws(() => checkPublishedHistory(root, 'invalid-reference', latest.date));
});

test('the gap names the next corpus rung and distinguishes an existing validator from its CI receipt', () => {
  const floor = { axes: { corpus: { signals: { acceptance: 'partial', externally_verified_wild: 1 } } } };
  const dims = { fetchwiring: 'partial', acceptance: 'partial', corpus: 'complete', sweep: 'complete' };
  assert.deepEqual(nextAxisGaps(floor, 'corpus', 'C1', dims), ['reference a fetched Tier B fixture in spec.yaml or record a reviewed exemption']);
  assert.deepEqual(nextAxisGaps(floor, 'corpus', 'C2', dims), ['record a passing CI result for the existing acceptance test']);
});
