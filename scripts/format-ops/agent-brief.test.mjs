import test from 'node:test';
import assert from 'node:assert/strict';
import { brief } from './agent-brief.mjs';

const formats = { formats: [{ id: 'eml', tier: {declared: 'available'}, levels: {engine: 'L2'},
  axes: {corpus: {blocking_gaps: ['Add an upstream sample.']}} }] };
const targets = { outputs: [{ id: 'md', capabilities: [
  {id: 'readback', status: 'failed', detail: 'Heading lost.', evidence: ['conversion.go']},
]}], connectors: [{ id: 'cms', capabilities: [
  {id: 'publish', status: 'unverified', detail: 'No operation test.', evidence: []},
]}] };

test('selects a known subject and its cited evidence', () => {
  assert.match(brief('format:eml', formats, targets), /Add an upstream sample/);
  assert.match(brief('output:md', formats, targets), /Heading lost.*conversion.go/);
  assert.match(brief('', formats, targets), /output:md.*readback \(failed\)/);
  assert.match(brief('', formats, targets), /format:eml.*corpus.*Add an upstream sample/);
  assert.match(brief('', formats, targets), /connector:cms.*publish \(unverified\)/);
  assert.match(brief('connector:cms', formats, targets), /make -C bowrain check-connector-maturity[\s\S]*connector-capabilities\.json/);
});

test('distinguishes inception from a misspelled or duplicate subject', () => {
  assert.match(brief('new:connector:wiki', formats, targets), /representative fixture/);
  for (const value of ['new:connector:cms', 'new:format:eml', 'format:missing', 'format:../foo', 'tool:md']) {
    assert.throws(() => brief(value, formats, targets));
  }
});
