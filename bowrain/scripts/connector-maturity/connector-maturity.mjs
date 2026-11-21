// Measure the platform's content connectors and check the published report.
//
// The connectors measured here are platform integrations, so the measurement
// and its dataset belong to the platform. The dataset schema, the evidence rules
// and the history contract are the framework's, imported from
// scripts/format-ops/content-maturity.mjs. See docs/internals/content-maturity.md.
//
//   --check  verify the committed rows against a fresh probe; a weaker status
//            fails, and a stronger one is left for the scheduled refresh
//   --write  record a new observation for today, or for --date
import { existsSync, readFileSync, renameSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';
import { DEFAULT_ROOT } from '../../../scripts/format-ops/lib.mjs';
import { checkAppendOnly, recordHistory, settleCapabilities, snapshotDate, states } from '../../../scripts/format-ops/content-maturity.mjs';

export const currentPath = 'bowrain/web/docs/static/data/connector-capabilities.json';
export const historyPath = 'bowrain/web/docs/static/data/connector-capabilities-history.json';
const json = value => JSON.stringify(value, null, 2) + '\n';
const read = file => JSON.parse(readFileSync(file, 'utf8'));

export async function run(root, args, probe) {
  const { values } = parseArgs({ args, options: {
    check: { type: 'boolean' }, write: { type: 'boolean' }, date: { type: 'string' }, 'history-base': { type: 'string' },
  } });
  if (Boolean(values.check) === Boolean(values.write)) throw new Error('Choose --check or --write.');
  const currentFile = resolve(root, currentPath);
  const historyFile = resolve(root, historyPath);
  if (values.check && !existsSync(historyFile)) throw new Error('Missing ' + historyPath);
  const committed = values.check ? read(currentFile) : undefined;
  const date = snapshotDate(values.date ?? committed?.date ?? new Date().toISOString().slice(0, 10));
  const previous = existsSync(historyFile) ? read(historyFile) : [];
  recordHistory(previous, { date });
  if (values['history-base']) checkAppendOnly(root, values['history-base'], date, [[historyPath, () => []]]);

  if (!probe) ({ connectorRows: probe } = await import('./connectors.mjs'));
  const current = { schema_version: 1, date, connectors: settleCapabilities({ root, check: values.check, file: currentPath,
    kind: 'connector', group: 'connectors', committed, fresh: probe(root) }) };
  const history = recordHistory(previous, { date, connectors: states(current.connectors) });
  for (const [file, data] of [[historyFile, json(history)], [currentFile, json(current)]]) {
    if (existsSync(file) && readFileSync(file, 'utf8') === data) continue;
    if (values.check) throw new Error('Stale ' + file);
    const stage = file + '.tmp-' + process.pid;
    writeFileSync(stage, data);
    renameSync(stage, file);
  }
  console.log(`Connector capabilities on ${date}: ${current.connectors.length} connectors.`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    await run(DEFAULT_ROOT, process.argv.slice(2));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
