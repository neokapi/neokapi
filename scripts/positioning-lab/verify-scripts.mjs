// Execute selected offline demo commands as tests. Never capture, narrate or render.
import { spawnSync, execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadManifest } from '../../harness/src/lib/manifest.ts';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const ids = process.argv.slice(2);
const selected = ids.length ? ids : ['s0-northsea-context', 's0-northsea-checks', '05-ai-checks-guardrail', '10-cli-points-and-voice', 'audience-constraints'];
const allowed = new Set([...selected.filter(id => ['s0-northsea-context', 's0-northsea-checks', '05-ai-checks-guardrail', '10-cli-points-and-voice', 'audience-constraints'].includes(id))]);
if (selected.some(id => !allowed.has(id))) throw new Error('Only the reviewed offline positioning demos may be verified');
const hash = bytes => createHash('sha256').update(bytes).digest('hex');
const results = [];
for (const id of selected) {
  const manifest = loadManifest(id);
  if (manifest.terminal !== 'shell' || manifest.needsAi || manifest.brand === 'bowrain') throw new Error(`Not an offline shell fixture: ${id}`);
  const sandbox = mkdtempSync(join(tmpdir(), 'neokapi-script-test-'));
  const fixture = manifest.fixturesFrom ? join(repo, manifest.fixturesFrom) : join(repo, 'harness/demos', id, 'fixtures');
  const env = {
    PATH: `${join(repo, 'bin')}:${process.env.PATH}`, TMPDIR: tmpdir(), LANG: 'en_US.UTF-8',
    GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1',
    KAPI_NO_PROJECT: '1', ...(manifest.project ? { KAPI_PROJECT: join(sandbox, 'kapi.yaml') } : {}),
    KAPI_CONFIG_DIR: join(sandbox, '.isolated/config'), XDG_DATA_HOME: join(sandbox, '.isolated/data'),
    XDG_CACHE_HOME: join(sandbox, '.isolated/cache'), KAPI_PLUGINS_DIR_ONLY: '1',
    KAPI_PLUGINS_DIR: join(sandbox, '.isolated/plugins'), KAPI_TELEMETRY: '0',
  };
  const commands = [];
  const normalize = value => JSON.parse(JSON.stringify(value).replaceAll(sandbox, '<fixture>').replaceAll(repo, '<repo>'));
  function run(command, expected, phase) {
    // Explicit -p wins over KAPI_NO_PROJECT; the recording harness discovers this
    // same isolated fixture, while this test keeps discovery disabled.
    const effectiveCommand = manifest.project && command.startsWith('kapi ')
      ? `kapi -p kapi.yaml ${command.slice(5)}` : command;
    const result = spawnSync('/bin/sh', ['-c', effectiveCommand], { cwd: sandbox, env, encoding: 'utf8', timeout: 120000, maxBuffer: 8 * 1024 * 1024 });
    const passed = !result.error && expected.includes(result.status);
    commands.push(normalize({ phase, command, effectiveCommand, expectedExitCodes: expected, exitCode: result.status, passed,
      stdout: result.stdout, stderr: result.stderr, error: result.error?.message ?? null }));
    if (!passed) throw new Error(`${id}: ${phase} expected ${expected}, got ${result.status}: ${result.stderr?.slice(0, 300)}`);
  }
  let failure = null;
  try {
    cpSync(fixture, sandbox, { recursive: true });
    for (const command of manifest.setup ?? []) run(command, [0], 'setup');
    for (const step of manifest.script) {
      if (!step.command) continue;
      run(step.command, Array.isArray(step.expectExit) ? step.expectExit : [step.expectExit ?? 0], 'script');
    }
  } catch (error) { failure = normalize(error.message); }
  finally { rmSync(sandbox, { recursive: true, force: true }); }
  results.push({ id, manifestSha256: hash(readFileSync(join(repo, 'harness/demos', id, 'demo.yaml'))), passed: !failure, failure, commands });
  console.log(`${failure ? 'FAIL' : 'PASS'} ${id}: ${commands.length} commands${failure ? `: ${failure}` : ''}`);
}
const output = join(repo, 'harness/out/positioning-review/script-tests.json');
mkdirSync(dirname(output), { recursive: true });
writeFileSync(output, JSON.stringify({ schema: 'neokapi-script-tests/v1', testedAt: new Date().toISOString(),
  sourceCommit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim(),
  sourceDiffSha256: hash(execFileSync('git', ['diff', 'HEAD'], { cwd: repo })),
  binarySha256: hash(readFileSync(join(repo, 'bin/kapi'))),
  scope: 'Command and exit-code verification only. No video, audio, screenshot, capture timeline or narration was produced.', results }, null, 2) + '\n');
if (results.some(result => !result.passed)) process.exitCode = 1;
