import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { DEFAULT_ROOT } from './lib.mjs';

const pathFor = {
  format: 'web/static/data/format-maturity.json',
  output: 'web/static/data/content-capabilities.json',
  connector: 'bowrain/web/docs/static/data/connector-capabilities.json',
};
// Connectors are platform integrations, measured by make -C bowrain check-connector-maturity.
const reproduce = {
  format: 'make check-content-maturity',
  output: 'make check-content-maturity',
  connector: 'make -C bowrain check-connector-maturity',
};
const sources = {
  format: '.skills/implement-format/SKILL.md and core/formats/support.yaml',
  output: 'host/toolbox_conv.go, host/toolbox_conv_maturity_test.go and scripts/format-ops/outputs.mjs',
  connector: 'core/venue/connector, bowrain/core/connector, bowrain/connector and bowrain/scripts/connector-maturity/connectors.mjs',
};

export function brief(select, formats, capabilities) {
  const lines = [
    '# Content maturity review',
    '',
    'Work in this checkout. Read AGENTS.md and docs/internals/content-maturity.md.',
    'Run make check-content-maturity to reproduce the format and output evidence,',
    'and make -C bowrain check-connector-maturity to reproduce the connector evidence.',
  ];
  if (select) {
    const match = /^(new:)?(format|output|connector):([a-z0-9][a-z0-9._-]*)$/.exec(select);
    if (!match) throw new Error('Use [new:]format:<id>, [new:]output:<id> or [new:]connector:<id>.');
    const [, isNew, kind, id] = match;
    const inventory = kind === 'format' ? formats.formats : capabilities[kind + 's'];
    const row = inventory.find(r => r.id === id);
    lines.push('', '## ' + kind + ' ' + id, '', 'Inspect ' + sources[kind] + '.', 'Reproduce with ' + reproduce[kind] + '.');
    if (isNew) {
      if (row) throw new Error(select + ' already has an observation. Omit new: to review it.');
      lines.push('Document demand, scope, a representative fixture, the upstream contract and the operations to test.',
        'Check the radar and existing registrations. Implement the accepted scope. Record gaps and the first observation.');
    } else {
      if (!row) throw new Error('Unknown subject ' + select + '. Use new: for an inception review.');
      lines.push('Published evidence in ' + pathFor[kind] + '.', '');
      if (kind === 'format') {
        lines.push('Declared tier: ' + (row.tier?.declared ?? 'unlisted'), 'Measured axes: ' + JSON.stringify(row.levels));
        for (const [axis, evidence] of Object.entries(row.axes ?? {})) {
          for (const gap of evidence.blocking_gaps ?? []) lines.push('- ' + axis + ': ' + gap);
        }
      } else {
        for (const cap of row.capabilities) {
          lines.push('- ' + cap.id + ' (' + cap.status + '): ' + cap.detail + ' ' + cap.evidence.join(', '));
        }
      }
    }
  } else {
    lines.push('', 'Run node .skills/format-ops/scripts/due.mjs to inspect the existing audit ledger.',
      'Review measured failures first, then select an uncovered operation or a format gap.', '');
    const inventories = [['output', capabilities.outputs], ['connector', capabilities.connectors ?? []]];
    for (const status of ['failed', 'unverified']) {
      for (const [kind, rows] of inventories) {
        for (const row of rows) {
          const gaps = row.capabilities.filter(cap => cap.status === status);
          if (gaps.length) lines.push('- ' + kind + ':' + row.id + ' · ' + gaps.map(cap => cap.id + ' (' + status + ')').join(', '));
        }
      }
    }
    lines.push('', '## Input format evidence', '',
      'The following rows show the first open Engine, Corpus or Knowledge gap for each format.', '');
    for (const row of formats.formats) {
      for (const axis of ['engine', 'corpus', 'knowledge']) {
        const gap = row.axes?.[axis]?.blocking_gaps?.[0];
        if (gap) {
          lines.push('- format:' + row.id + ' · ' + axis + ' · ' + gap);
          break;
        }
      }
    }
    lines.push('', 'Inspect all axes with node scripts/format-ops/agent-brief.mjs format:<id>.');
  }
  lines.push('', 'Add a failing behavioral test for the selected gap, implement the change, and rerun the test.',
    'Run make refresh-content-maturity and make check-content-maturity, or the bowrain connector targets for a connector.',
    'Review the history diff and cite the resulting evidence.',
    'Repeat for the remaining gates in scope. Identify any external evidence needed to proceed.',
    'Submit eligible tier changes through the tier-review process with the named CI gates and remaining limitations.');
  return lines.join('\n') + '\n';
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    if (process.argv.length > 3) throw new Error('Pass at most one subject.');
    const read = p => JSON.parse(readFileSync(resolve(DEFAULT_ROOT, p), 'utf8'));
    const connectors = existsSync(resolve(DEFAULT_ROOT, pathFor.connector)) ? read(pathFor.connector).connectors : [];
    process.stdout.write(brief(process.argv[2], read(pathFor.format), { outputs: read(pathFor.output).outputs, connectors }));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
