import { spawnSync } from 'node:child_process';

const probe = 'TestOutputMaturity';
const probePath = 'host/toolbox_conv_maturity_test.go';
const structurePath = 'host/toolbox_conv_test.go';
const emailPath = 'cli/toolbox_conv_eml_test.go';
const checks = [
  ['markdown', 'structure', structurePath, 'TestConvDocLangToMarkdown', 'DocLang headings, bold text and list items retain their roles in Markdown.'],
  ['html', 'structure', structurePath, 'TestConvDocLangToHTML', 'A DocLang heading and table render as HTML elements.'],
  ['doclang', 'structure', structurePath, 'TestConvMarkdownToDocLang', 'A Markdown heading and bold text retain their roles in DocLang.'],
  ['markdown', 'email', emailPath, 'TestEmailConversion/md', 'kconv writes the message subject as a Markdown heading and retains the body.'],
  ['html', 'email', emailPath, 'TestEmailConversion/html', 'kconv writes the message subject as an HTML heading and retains the body.'],
  ['doclang', 'email', emailPath, 'TestEmailConversion/dclg.xml', 'kconv infers DocLang from the output path and projects the subject and body.'],
];

export function outputCapabilities(events) {
  const outcomes = new Map();
  const observations = new Map();
  const targets = new Set();
  for (const event of events) {
    if (!event.Test) continue;
    const pkg = event.Package?.split('/').at(-1);
    if (pkg !== 'host' && pkg !== 'cli') continue;
    const key = pkg + '#' + event.Test;
    if (['pass', 'skip', 'fail'].includes(event.Action)) outcomes.set(key, event.Action);
    const [suite, id] = event.Test.split('/');
    if (pkg !== 'host' || suite !== probe || !id) continue;
    if (event.Action === 'run') targets.add(id);
    const json = event.Action === 'output' && event.Output?.match(/: KAPI_OUTPUT (\{.*\})\s*$/)?.[1];
    if (json) {
      if (observations.has(id)) throw new Error('Duplicate output observation for ' + id);
      observations.set(id, JSON.parse(json));
    }
  }
  if (outcomes.get('host#' + probe) !== 'pass' || targets.size === 0) {
    throw new Error('The output inventory did not finish.');
  }
  const rows = new Map();
  for (const id of targets) {
    const observation = observations.get(id);
    if (!observation || outcomes.get('host#' + probe + '/' + id) !== 'pass') {
      throw new Error('Incomplete output observation for ' + id);
    }
    rows.set(id, { id, capabilities: [{ ...observation, evidence: [probePath + '#' + probe] }] });
  }
  for (const [id, capability, file, test, detail] of checks) {
    const row = rows.get(id);
    if (!row) throw new Error('Missing output target ' + id);
    const key = file.split('/')[0] + '#' + test;
    if (outcomes.get(key) !== 'pass') throw new Error('Missing passing evidence for ' + key);
    row.capabilities.push({ id: capability, status: 'verified', detail, evidence: [file + '#' + test.split('/')[0]] });
  }
  return [...rows.values()].sort((a, b) => a.id.localeCompare(b.id, 'en'));
}

export function outputRows(root) {
  const tests = [...new Set([probe, ...checks.map(([, , , test]) => test.split('/')[0])])];
  const run = spawnSync('go', ['test', '-tags', 'fts5', '-count=1', '-json', '-run',
    '^(' + tests.join('|') + ')$', './host', './cli'],
    { cwd: root, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024 });
  if (run.error) throw run.error;
  if (run.status !== 0) throw new Error('Output probes failed\n' + run.stderr + run.stdout);
  return outputCapabilities(run.stdout.trim().split('\n').map(line => JSON.parse(line)));
}
