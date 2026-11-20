import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, readdirSync, renameSync, writeFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

const environment = 'github-actions';
const workflow = '.github/workflows/format-acceptance.yml';
const packagePrefix = 'github.com/neokapi/neokapi/core/formats/';
const consumers = {
  androidxml: { XmllintWellFormed: 'fixtures', Aapt2: 'fixtures' },
  applestrings: { PlutilLint: 'fixtures' },
  arb: { TranslatedOutputValidatesAgainstSchema: 'jq-ajv' },
  designtokens: { JQWellFormed: 'fixtures', SchemaEquivalence: 'fixtures' },
  eml: { PythonEmail: 'single' },
  i18next: { JQWellFormed: 'fixtures', SchemaValid: 'fixtures' },
  mdx: { TranslatedMDXCompiles: 'fixtures', UntranslatedRoundTripCompiles: 'fixtures' },
  resx: { ResgenCompiles: 'fixtures', XmllintWellFormed: 'fixtures' },
  xcstrings: { TranslatedOutputValidatesAgainstSchema: 'jq-ajv' },
};

function measuredFormats() {
  return readdirSync('core/formats').filter(id => existsSync('core/formats/' + id + '/acceptance_test.go')).sort();
}

function terminal(action) {
  return ['pass', 'fail', 'skip'].includes(action);
}

function eligible(format, test) {
  const [suite] = test.split('/');
  if (suite.startsWith('TestAcceptance')) return consumers[format]?.[suite.slice('TestAcceptance'.length)];
  return undefined;
}

function verdict(checks, conclusion) {
  if (conclusion === 'failure' || checks.some(c => c.outcome === 'fail')) return 'fail';
  if (!checks.some(c => c.outcome === 'pass')) return 'skip';
  return checks.some(c => c.outcome === 'skip') ? 'partial' : 'pass';
}

function digest(checks) {
  return createHash('sha256').update(JSON.stringify(checks)).digest('hex');
}

export function collect(events, formats) {
  const packages = new Map(formats.map(id => [id, { tests: new Map() }]));
  for (const e of events) {
    if (!e.Package?.startsWith(packagePrefix)) continue;
    const id = e.Package.slice(packagePrefix.length);
    const pkg = packages.get(id);
    if (!pkg) continue;
    if (!e.Test && terminal(e.Action)) pkg.outcome = e.Action;
    if (e.Test && eligible(id, e.Test) && (e.Action === 'run' || terminal(e.Action))) {
      pkg.tests.set(e.Test, e.Action);
    }
  }
  return formats.map(format => {
    const { tests, outcome } = packages.get(format);
    if (!['pass', 'fail'].includes(outcome)) throw new Error('Incomplete acceptance package: ' + format);
    const conclusion = outcome === 'pass' ? 'success' : 'failure';
    const names = [...tests.keys()].sort();
    const checks = [];
    for (const suite of Object.keys(consumers[format] || {})) {
      const root = 'TestAcceptance' + suite;
      if (!terminal(tests.get(root)) && conclusion === 'success') throw new Error('Missing validator: ' + root);
    }
    for (const test of names) {
      const result = tests.get(test);
      if (!terminal(result)) throw new Error('Incomplete acceptance test: ' + test);
      if (names.some(child => child.startsWith(test + '/'))) continue;
      if (result === 'pass' && !test.includes('/') && eligible(format, test) !== 'single') {
        throw new Error('Consumer suite passed without running fixtures: ' + test);
      }
      if (result === 'pass' && eligible(format, test) === 'jq-ajv' &&
          !/\/(jq|ajv)$/.test(test)) throw new Error('Missing validator subtest: ' + test);
      checks.push({ test, outcome: result });
    }
    return { format, conclusion, test_outcome: verdict(checks, conclusion), checks, output_sha: digest(checks) };
  });
}

function validRun(id) {
  return Number.isSafeInteger(id) && id > 0;
}

export function ingest(ledger, report, context, formats) {
  assert.equal(report.schema_version, 1);
  assert.equal(report.workflow, workflow);
  assert.equal(report.environment, environment);
  assert.match(context.head_sha, /^[a-f0-9]{40}$/);
  assert.ok(validRun(context.run_id) && validRun(context.run_attempt));
  for (const key of ['head_sha', 'run_id', 'run_attempt']) assert.equal(report[key], context[key], key);
  assert.deepEqual(report.formats.map(r => r.format), formats);
  const evidence = report.formats.map(({ format, conclusion, checks, test_outcome, output_sha }) => {
    assert.ok(['success','failure'].includes(conclusion));
    assert.ok(Array.isArray(checks));
    const seen = new Set();
    for (const {test, outcome} of checks) {
      assert.ok(eligible(format, test) && terminal(outcome) && !seen.has(test));
      seen.add(test);
    }
    if (conclusion === 'success') {
      assert.ok(!checks.some(c => c.outcome === 'fail'));
      for (const c of checks.filter(c => c.outcome === 'pass')) {
        assert.ok(c.test.includes('/') || eligible(format, c.test) === 'single', 'No executed fixture: ' + c.test);
        if (eligible(format, c.test) === 'jq-ajv') {
          assert.match(c.test, /\/(jq|ajv)$/);
          const fixture = c.test.slice(0, c.test.lastIndexOf('/'));
          for (const validator of ['jq', 'ajv']) assert.ok(seen.has(fixture + '/' + validator));
        }
      }
      for (const name of Object.keys(consumers[format] || {})) {
        const suite = 'TestAcceptance' + name;
        assert.ok(checks.some(c => c.test.split('/')[0] === suite), 'Missing validator: ' + suite);
      }
    }
    assert.equal(digest(checks), output_sha);
    assert.equal(verdict(checks, conclusion), test_outcome);
    return { check: 'format-acceptance', format, workflow, environment, ...context, conclusion,
      test_outcome, checks, exit: conclusion === 'success' ? 0 : 1, output_sha };
  });
  const same = r => r.workflow === workflow && r.run_id === context.run_id;
  const prior = ledger.runs.filter(same);
  if (prior.some(r => r.run_attempt > context.run_attempt)) throw new Error('Cannot import an older run attempt.');
  const duplicate = prior.find(r => r.run_attempt === context.run_attempt);
  if (duplicate) {
    assert.deepEqual(duplicate.evidence, evidence, 'The same run attempt produced different evidence.');
    return ledger;
  }
  return { ...ledger, runs: [...ledger.runs, {
    date: report.date, ritual: 'triage-score', commit: context.head_sha,
    model_id: 'github-actions', workflow, run_id: context.run_id, run_attempt: context.run_attempt,
    outcome: 'Measured external consumer acceptance on the hosted macOS runner.',
    evidence, followups: [],
  }] };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const [mode, artifact, input] = process.argv.slice(2);
  const head_sha = execFileSync('git', ['rev-parse', 'HEAD'], {encoding:'utf8'}).trim();
  assert.equal(process.env.GITHUB_ACTIONS, 'true', 'Hosted receipts require GitHub Actions.');
  assert.equal(process.env.GITHUB_SHA, head_sha, 'The checkout must match the workflow revision.');
  const context = { head_sha, run_id: Number(process.env.GITHUB_RUN_ID), run_attempt: Number(process.env.GITHUB_RUN_ATTEMPT) };
  assert.ok(validRun(context.run_id) && validRun(context.run_attempt));
  const formats = measuredFormats();
  const json = data => JSON.stringify(data, null, 2) + '\n';
  if (mode === 'collect' && input) {
    const events = readFileSync(input,'utf8').trim().split('\n').map(line => JSON.parse(line));
    const report = { schema_version: 1, workflow, environment, ...context, date: new Date().toISOString().slice(0,10),
      formats: collect(events, formats) };
    writeFileSync(artifact, json(report));
  } else if (mode === 'ingest' && !input) {
    const file = 'docs/internals/format-ops-ledger.json';
    const before = readFileSync(file,'utf8');
    const report = JSON.parse(readFileSync(artifact,'utf8'));
    assert.match(report.date, /^\d{4}-\d{2}-\d{2}$/);
    assert.equal(new Date(report.date).toISOString().slice(0,10), report.date);
    assert.ok(validRun(report.run_attempt) && report.run_attempt <= context.run_attempt);
    if (process.env.ACCEPTANCE_JOB_RESULT === 'failure') assert.equal(report.run_attempt, context.run_attempt);
    const updated = json(ingest(JSON.parse(before), report, { ...context, run_attempt: report.run_attempt }, formats));
    if (updated !== before) {
      writeFileSync(file + '.tmp-' + process.pid, updated);
      renameSync(file + '.tmp-' + process.pid, file);
    }
  } else {
    throw new Error('Use collect RECEIPT.json GO-EVENTS.jsonl or ingest RECEIPT.json.');
  }
}
