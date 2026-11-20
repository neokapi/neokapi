import assert from 'node:assert/strict';
import test from 'node:test';
import { connectorCapabilities } from './connectors.mjs';

const wordpress = [{id: 'wordpress', category: 'cms', surfaces: ['server']}];
const event = (name, action) => ({Package: 'github.com/neokapi/neokapi/bowrain/connector', Test: name, Action: action});

test('scores each operation from its own test outcome', () => {
  const [row] = connectorCapabilities(wordpress, [event('TestWordPressPublish', 'pass'), event('TestWordPressFetch', 'skip')]);
  const states = Object.fromEntries(row.capabilities.map(c => [c.id, c.status]));
  assert.deepEqual(states, {registration: 'declared', list: 'unverified', fetch: 'unverified', publish: 'verified'});
});

test('a missing or failing test invalidates the publication', () => {
  for (const action of ['fail', 'run']) {
    assert.throws(() => connectorCapabilities(wordpress, [event('TestWordPressFetch', action)]), /did not pass/);
  }
});

test('discovers a newly registered connector with open evidence gaps', () => {
  const [row] = connectorCapabilities([{id:'new-cms', category:'cms', surfaces:['remote']}], []);
  assert.deepEqual(row.capabilities.slice(1).map(c => c.status), ['unverified', 'unverified', 'unverified']);
});
