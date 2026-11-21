import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { currentPath, historyPath, run } from './connector-maturity.mjs';

test('the check replays committed connector evidence and fails only a weaker probe', async t => {
  const root = mkdtempSync(path.join(tmpdir(), 'connector-maturity-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(path.join(root, path.dirname(currentPath)), { recursive: true });
  writeFileSync(path.join(root, 'wordpress_test.go'), 'func TestWordPressFetch(t *testing.T) {}');
  const probe = status => () => [{ id: 'wordpress', capabilities: [
    { id: 'fetch', status, evidence: ['wordpress_test.go#TestWordPressFetch'], detail: 'Read posts from a fixture.' }] }];
  const quiet = t.mock.method(console, 'log', () => {});
  await run(root, ['--write', '--date', '2026-11-20'], probe('unverified'));
  const published = readFileSync(path.join(root, currentPath), 'utf8');
  assert.deepEqual(JSON.parse(readFileSync(path.join(root, historyPath), 'utf8')),
    [{ date: '2026-11-20', connectors: { wordpress: { fetch: 'unverified' } } }]);
  await run(root, ['--check'], probe('unverified'));
  await run(root, ['--check'], probe('verified'));
  await assert.rejects(run(root, ['--check'], probe('failed')), /weakens committed connector evidence/);
  assert.equal(readFileSync(path.join(root, currentPath), 'utf8'), published);
  quiet.mock.restore();
});
