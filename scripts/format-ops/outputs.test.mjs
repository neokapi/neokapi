import test from 'node:test';
import assert from 'node:assert/strict';
import { outputCapabilities } from './outputs.mjs';

function event(pkg, Test, Action, Output) {
  return { Package: 'github.com/neokapi/neokapi/' + pkg, Test, Action, Output };
}

function fixture() {
  const events = [event('host', 'TestOutputMaturity', 'pass')];
  for (const id of ['markdown', 'html', 'doclang', 'xml']) {
    const name = 'TestOutputMaturity/' + id;
    const observation = { id: 'project-text', status: id === 'xml' ? 'failed' : 'verified', detail: 'Fixture result.' };
    events.push(event('host', name, 'run'),
      event('host', name, 'output', '    probe.go:1: KAPI_OUTPUT ' + JSON.stringify(observation)),
      event('host', name, 'pass'));
  }
  for (const name of ['TestConvDocLangToMarkdown', 'TestConvDocLangToHTML', 'TestConvMarkdownToDocLang']) {
    events.push(event('host', name, 'pass'));
  }
  for (const ext of ['md', 'html', 'dclg.xml']) events.push(event('cli', 'TestEmailConversion/' + ext, 'pass'));
  return events;
}

test('a completed projection can report lost content', () => {
  const rows = outputCapabilities(fixture());
  const xml = rows.find(row => row.id === 'xml');
  assert.equal(xml.capabilities[0].status, 'failed');
  assert.deepEqual(rows.find(row => row.id === 'doclang').capabilities.map(c => c.id), ['project-text', 'structure', 'email']);
});

test('the audit refuses incomplete or duplicate measurements', () => {
  const events = fixture();
  const observation = events.find(e => e.Test === 'TestOutputMaturity/html' && e.Action === 'output');
  assert.throws(() => outputCapabilities(events.filter(e => e !== observation)), /Incomplete output observation/);
  assert.throws(() => outputCapabilities([...events, observation]), /Duplicate output observation/);
  const skipped = events.map(e => e.Test === 'TestEmailConversion/md' ? { ...e, Action: 'skip' } : e);
  assert.throws(() => outputCapabilities(skipped), /Missing passing evidence/);
});
