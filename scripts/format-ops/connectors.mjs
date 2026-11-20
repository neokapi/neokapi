import { execFileSync, spawnSync } from 'node:child_process';

const integrations = 'bowrain/connector';
const project = 'host/venue/source';
const probes = {
  file: {
    list: ['file_test.go', 'TestFileConnectorList', 'Discover a file and identify its format.'],
    fetch: ['file_test.go', 'TestFileConnectorFetch', 'Extract blocks from a local HTML file.'],
    publish: ['eml_delivery_test.go', 'TestEmailDelivery', 'Deliver an edited message using its source skeleton.'],
  },
  forge: {
    publish: ['forge_test.go', 'TestForgeConnectorPublish_BranchAndPR', 'Publish a delivery branch and request a review through a fixture forge.'],
  },
  wordpress: {
    fetch: ['wordpress_test.go', 'TestWordPressFetch', 'Read posts into blocks from a fixture REST endpoint.'],
    publish: ['wordpress_test.go', 'TestWordPressPublish', 'Send reviewed fields to the original post at a fixture REST endpoint.'],
  },
  hubspot: {
    fetch: ['hubspot_test.go', 'TestHubSpotExchange', 'Extract a page title and description from a fixture REST endpoint.'],
    publish: ['hubspot_test.go', 'TestHubSpotExchange', 'Update the title and preserve the description on the original page.'],
  },
  figma: {
    fetch: ['figma_test.go', 'TestFigmaFetch', 'Read design text through a fixture REST endpoint.'],
    publish: ['figma_test.go', 'TestFigmaPublishIsReadOnly', 'The connector reports that design nodes are read-only.', 'unsupported'],
  },
  'bowrain-source': {
    push: ['push_read_wiring_test.go', 'TestPushReadsTheBlocksACheckReads', 'Compare the block identities sent to the venue with those read by the local checker.'],
    pull: ['pull_test.go', 'TestPull_CrossFormatTargetWritesTheTargetFormat', 'Write a translation from a fixture venue in the format of the target.'],
  },
};

function packageFor(row) {
  return row.surfaces.includes('project') ? project : integrations;
}

export function connectorCapabilities(rows, events) {
  const outcomes = new Map(events.filter(e => e.Test && ['pass', 'skip', 'fail'].includes(e.Action))
    .map(e => [e.Package + '#' + e.Test, e.Action]));
  return rows.map(row => {
    const pkg = packageFor(row);
    const subject = pkg === project ? 'source' : 'integration';
    const capabilities = [{
      id: 'registration',
      status: 'declared',
      evidence: [pkg + (pkg === project ? '/source.go' : '/register.go')],
      detail: row.category + ' ' + subject + ' connector (' + row.surfaces.join(', ') + ').',
    }];
    const operations = pkg === project ? ['push', 'pull'] : ['list', 'fetch', 'publish'];
    for (const id of operations) {
      const probe = probes[row.id]?.[id];
      if (!probe) {
        capabilities.push({ id, status: 'unverified', evidence: [], detail: 'No operation test is selected for this audit.' });
        continue;
      }
      const [file, test, detail, status = 'verified'] = probe;
      const result = outcomes.get('github.com/neokapi/neokapi/' + pkg + '#' + test);
      if (result !== 'pass' && result !== 'skip') throw new Error(pkg + '#' + test + ' did not pass or report a skip');
      capabilities.push({ id, status: result === 'skip' ? 'unverified' : status,
        evidence: [pkg + '/' + file + '#' + test], detail: result === 'skip' ? 'The test was skipped in this run.' : detail });
    }
    return { id: row.id, capabilities };
  });
}

export function connectorRows(root) {
  const rows = JSON.parse(execFileSync('go', ['run', '-tags', 'fts5', './bowrain/scripts/connector-inventory'],
    { cwd: root, encoding: 'utf8' }));
  const projectIdentity = 'TestNewSourceConnector_ClaimTokenFromCache';
  const tests = Object.values(probes).flatMap(ops => Object.values(ops).map(([, test]) => test));
  tests.push(projectIdentity);
  const run = spawnSync('go', ['test', '-tags', 'fts5', '-count=1', '-json', '-run', '^(' + tests.join('|') + ')$',
    './' + integrations, './' + project], { cwd: root, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024 });
  if (run.error) throw run.error;
  if (run.status !== 0) throw new Error('Connector probes failed\n' + run.stderr + run.stdout);
  const events = run.stdout.trim().split('\n').filter(Boolean).map(line => JSON.parse(line));
  const identityEvents = events.filter(e => e.Package === 'github.com/neokapi/neokapi/' + project && e.Test === projectIdentity);
  const identities = identityEvents.filter(e => e.Action === 'output')
    .map(e => e.Output.match(/: KAPI_PROJECT_CONNECTOR ([\w-]+) ([\w-]+)\s*$/)).filter(Boolean);
  if (identities.length !== 1 || !identityEvents.some(e => e.Action === 'pass')) {
    throw new Error('Missing verified project connector identity.');
  }
  const [, id, category] = identities[0];
  rows.push({ id, category, surfaces: ['project'] });
  for (const id of Object.keys(probes)) {
    if (!rows.some(row => row.id === id)) throw new Error('Connector evidence refers to missing connector ' + id);
  }
  return connectorCapabilities(rows, events);
}
