import test from 'node:test';
import assert from 'node:assert/strict';
import { collect, ingest } from './acceptance-receipts.mjs';

function result(format, Action, Test) {
  return { Package: 'github.com/neokapi/neokapi/core/formats/' + format, Action, Test };
}

function email(action) {
  return collect([result('eml', action, 'TestAcceptancePythonEmail'),
    result('eml', action === 'fail' ? 'fail' : 'pass')], ['eml']);
}

function sample(formats, attempt = 1) {
  const context = { run_id: 42, run_attempt: attempt, head_sha: 'a'.repeat(40) };
  const report = { schema_version: 1, date: '2026-01-01', workflow: '.github/workflows/format-acceptance.yml',
    environment: 'github-actions', ...context, formats };
  return { report, context };
}

test('a passing or skipped consumer produces the corresponding scoped receipt', () => {
  for (const outcome of ['pass', 'skip', 'fail']) {
    const [row] = email(outcome);
    assert.deepEqual(row.checks, [{ test: 'TestAcceptancePythonEmail', outcome }]);
    assert.equal(row.test_outcome, outcome);
  }
});

test('a passing parent or internal reparse cannot qualify a missing validator', () => {
  const root = 'TestAcceptanceTranslatedOutputValidatesAgainstSchema';
  const events = [root, root + '/simple.arb', root + '/simple.arb/jq'].map(name => result('arb','pass',name));
  events.push(result('arb','skip',root + '/simple.arb/ajv'), result('arb','pass'));
  const [row] = collect(events,['arb']);
  assert.equal(row.test_outcome, 'partial');
  assert.deepEqual(row.checks.map(c => c.outcome), ['skip','pass']);
  const internal = collect([result('newfmt','pass','TestAcceptanceStructuralReparse'), result('newfmt','pass')], ['newfmt']);
  assert.deepEqual(internal[0].checks, []);
  assert.equal(internal[0].test_outcome, 'skip');
  assert.throws(() => collect([result('arb','pass',root), result('arb','pass')], ['arb']), /without running fixtures/);
  assert.throws(() => collect([], ['eml']), /Incomplete acceptance package/);
});

test('import preserves earlier observations and reproduces a newer attempt exactly once', () => {
  const first = sample(email('pass'));
  const initial = ingest({runs:[]}, first.report, first.context, ['eml']);
  const next = sample(email('skip'), 2);
  const updated = ingest(initial, next.report, next.context, ['eml']);
  assert.deepEqual(updated.runs.map(r => r.evidence[0].test_outcome), ['pass','skip']);
  assert.deepEqual(ingest(updated, next.report, next.context, ['eml']), updated);
  assert.throws(() => ingest(updated, first.report, first.context, ['eml']), /older run attempt/);
  const different = sample(email('pass'), 2);
  assert.throws(() => ingest(updated, different.report, different.context, ['eml']), /different evidence/);
  assert.throws(() => ingest(updated, next.report, { ...next.context, head_sha: 'b'.repeat(40) }, ['eml']));
  assert.throws(() => ingest(updated, { ...next.report, environment:'local' }, next.context, ['eml']));
  next.report.formats[0].checks[0].outcome = 'pass';
  assert.throws(() => ingest(updated, next.report, next.context, ['eml']));
});
