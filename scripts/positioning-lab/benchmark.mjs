// Real CLI/MCP latency measurements; no provider, plugin or user project state.
import { spawn, spawnSync, execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { cpus, platform, arch, release, totalmem, tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { createInterface } from 'node:readline';
import { fileURLToPath } from 'node:url';
import { performance } from 'node:perf_hooks';
import { isDeepStrictEqual } from 'node:util';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const binary = resolve(process.env.POSITIONING_KAPI_BIN || join(repo, 'bin/kapi'));
const sample = resolve(process.env.POSITIONING_SAMPLE_DIR || join(repo, 'samples/audience-context'));
const output = resolve(process.env.POSITIONING_BENCH_OUTPUT || join(repo,
  'harness/out/positioning-review/benchmark.json'));
const iterations = Number(process.env.POSITIONING_BENCH_RUNS || 40);
if (!Number.isInteger(iterations) || iterations < 2 || iterations > 1000) {
  throw new Error('POSITIONING_BENCH_RUNS must be an integer from 2 to 1000');
}
if (!existsSync(join(sample, 'kapi.yaml'))) throw new Error('Build and integrate samples/audience-context first');
const sandbox = mkdtempSync(join(tmpdir(), 'neokapi-positioning-bench-'));
cpSync(sample, sandbox, { recursive: true });
const env = {
  PATH: process.env.PATH, TMPDIR: tmpdir(), LANG: 'en_US.UTF-8',
  KAPI_NO_PROJECT: '1', KAPI_PROJECT: join(sandbox, 'kapi.yaml'),
  KAPI_CONFIG_DIR: join(sandbox, '.isolated/config'),
  XDG_DATA_HOME: join(sandbox, '.isolated/data'),
  XDG_CACHE_HOME: join(sandbox, '.isolated/cache'),
  KAPI_PLUGINS_DIR_ONLY: '1', KAPI_PLUGINS_DIR: join(sandbox, '.isolated/plugins'),
  KAPI_TELEMETRY: '0',
};
const hash = value => createHash('sha256').update(value).digest('hex');
const normalize = value => JSON.parse(JSON.stringify(value).replaceAll(sandbox, '<sandbox>'));
const git = (...args) => execFileSync('git', args, { cwd: repo, encoding: 'utf8' }).trim();
const rawRuns = [];
const attempts = [];
const reports = {};
function keepReport(report) {
  const normalized = normalize(report);
  const id = hash(JSON.stringify(normalized));
  reports[id] = normalized;
  return id;
}
function cli(args) {
  const start = performance.now();
  const run = spawnSync(binary, args, { cwd: sandbox, env, encoding: 'utf8', timeout: 30000, maxBuffer: 8 * 1024 * 1024 });
  return { run, elapsedMs: performance.now() - start };
}
function parseCLI(run) {
  if (run.error || ![0, 3].includes(run.status)) throw new Error(run.error?.message || run.stderr || `Exit ${run.status}`);
  const report = JSON.parse(run.stdout);
  if (report.schema !== 'kapi.check/v1') throw new Error(`Unexpected report schema: ${report.schema}`);
  return report;
}
function startMCP() {
  const child = spawn(binary, ['mcp', '-p', 'kapi.yaml'], { cwd: sandbox, env, stdio: ['pipe', 'pipe', 'pipe'] });
  const pending = new Map();
  let sequence = 0;
  let exited = false;
  let stderr = '';
  const transcript = [];
  const openedAt = performance.now();
  child.stderr.on('data', chunk => { stderr = (stderr + chunk).slice(-8000); });
  const lines = createInterface({ input: child.stdout });
  function rejectAll(error) {
    for (const { reject, timer } of pending.values()) { clearTimeout(timer); reject(error); }
    pending.clear();
  }
  lines.on('line', line => {
    transcript.push(line);
    let message;
    try { message = JSON.parse(line); } catch { rejectAll(new Error(`Invalid MCP JSON: ${line.slice(0, 200)}`)); return; }
    const item = pending.get(message.id);
    if (!item) return;
    pending.delete(message.id); clearTimeout(item.timer);
    if (message.error) item.reject(new Error(JSON.stringify(message.error)));
    else item.resolve(message.result);
  });
  child.on('error', rejectAll);
  child.on('exit', code => { exited = true; rejectAll(new Error(`MCP exited ${code}: ${stderr}`)); });
  return {
    residentKiB() {
      if (platform() === 'win32') return null;
      try { return Number(execFileSync('ps', ['-o', 'rss=', '-p', String(child.pid)], { encoding: 'utf8' }).trim()) || null; }
      catch { return null; }
    },
    diagnostics() { return normalize({ stderr, transcript, elapsedSinceSpawnMs: performance.now() - openedAt }); },
    request(method, params) {
      return new Promise((resolveRequest, reject) => {
        const id = ++sequence;
        const timer = setTimeout(() => { pending.delete(id); reject(new Error(`MCP timeout: ${method}`)); child.kill(); }, 30000);
        pending.set(id, { resolve: resolveRequest, reject, timer });
        child.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
      });
    },
    notify(method) { child.stdin.write(JSON.stringify({ jsonrpc: '2.0', method }) + '\n'); },
    async close() {
      if (exited) return;
      const ended = new Promise(resolveExit => child.once('exit', resolveExit));
      child.stdin.end();
      const timer = setTimeout(() => child.kill(), 2000);
      await ended; clearTimeout(timer); lines.close();
    },
  };
}
function extractMCP(result) {
  if (result.isError) throw new Error(JSON.stringify(result.content));
  const report = result.structuredContent || JSON.parse(result.content.find(c => c.type === 'text').text);
  if (report.schema !== 'kapi.check/v1') throw new Error(`Unexpected MCP report schema: ${report.schema}`);
  return report;
}
const version = cli(['--version']);
if (version.run.status !== 0) throw new Error(version.run.stderr);
const workloads = [
  { id: 'small', blocks: 3, edits: false },
  { id: 'large', blocks: 300, edits: false },
  { id: 'sparse-edit', blocks: 300, edits: true },
];
const corpus = workloads.map(w => ({ ...w, file: 'site/locales/en/adult.json',
  text: Object.fromEntries(Array.from({ length: w.blocks }, (_, i) => [`paragraph_${i}`,
    'Your video appointment starts when you choose to join. You can ask the adviser to explain each step.'])) }));
function mutate(w, i) {
  if (!w.edits) return;
  const updated = { ...w.text, paragraph_0: i % 2 === 0 ? `Your appointment reference is ${i}. It is risk-free.` : `Your appointment reference is ${i}. You can ask for help before you join.` };
  writeFileSync(join(sandbox, w.file), JSON.stringify(updated, null, 2) + '\n');
}
const initializationStart = performance.now();
const client = startMCP();
let initializeMs = null;
let failure = null;
let currentCase = { phase: 'initialize' };
try {
  await client.request('initialize', { protocolVersion: '2025-03-26', capabilities: {}, clientInfo: { name: 'positioning-benchmark', version: '1' } });
  client.notify('notifications/initialized');
  initializeMs = performance.now() - initializationStart;
  // Every iteration runs both conditions. Alternate order to reduce fixed-order effects.
  for (const w of corpus) {
    writeFileSync(join(sandbox, w.file), JSON.stringify(w.text, null, 2) + '\n');
    for (let i = 0; i < iterations; i++) {
      mutate(w, i);
      let pairedSemantics;
      for (const mode of i % 2 ? ['mcp', 'cli'] : ['cli', 'mcp']) {
        currentCase = { workload: w.id, mode, iteration: i };
        const attempt = { ...currentCase, inputSha256: hash(readFileSync(join(sandbox, w.file))), status: 'started' };
        attempts.push(attempt);
        const start = performance.now();
        let report, elapsedMs, exitCode;
        if (mode === 'cli') {
          const execution = cli(['check', w.file, '--json', '-p', 'kapi.yaml']);
          attempt.transport = { exitCode: execution.run.status, signal: execution.run.signal, error: execution.run.error?.message, stdout: execution.run.stdout, stderr: execution.run.stderr };
          attempt.elapsedMs = execution.elapsedMs;
          report = parseCLI(execution.run); elapsedMs = performance.now() - start; exitCode = execution.run.status;
        } else {
          const response = await client.request('tools/call', { name: 'check_file', arguments: { file: w.file } });
          attempt.transport = response;
          attempt.elapsedMs = performance.now() - start;
          report = extractMCP(response);
          elapsedMs = performance.now() - start;
        }
        attempt.report = keepReport(report);
        if (report.target?.blocks !== w.blocks) throw new Error(`Expected ${w.blocks} extracted blocks, got ${report.target?.blocks}`);
        const semantics = normalize({ pass: report.pass, target: report.target, gate: report.gate, summary: report.summary, findings: report.findings, analyzers: report.execution?.analyzers?.map(({ duration_ms, ...analyzer }) => analyzer) });
        const expected = w.edits && i % 2 === 0 ? 1 : 0;
        const actual = (report.findings ?? []).filter(f => f.metadata?.constraint_id === 'harbor-help/no-unsupported-assurance').length;
        if (actual !== expected) throw new Error(`Expected ${expected} constraint findings, got ${actual}`);
        if (!report.execution?.analyzers?.some(a => a.id === 'voice.guidance' && a.status === 'unsupported' && a.required === false)) {
          throw new Error('Missing explicit unsupported guidance coverage');
        }
        if (pairedSemantics !== undefined && !isDeepStrictEqual(semantics, pairedSemantics)) throw new Error('CLI/MCP report semantics differ');
        pairedSemantics = semantics;
        attempt.status = 'verified';
        rawRuns.push({ workload: w.id, mode, iteration: i, firstForWorkload: i === 0,
          elapsedMs, ...(exitCode !== undefined ? { exitCode } : {}),
          inputSha256: hash(readFileSync(join(sandbox, w.file))), report: keepReport(report),
          timings: report.execution?.timings || null,
          ...(mode === 'mcp' ? { residentKiBAfterRequest: client.residentKiB() } : {}) });
      }
    }
    console.log(`Measured ${w.id}: ${iterations} CLI + ${iterations} MCP calls`);
  }
} catch (error) {
  failure = normalize({ ...currentCase, message: error.message });
  if (attempts.length) attempts[attempts.length - 1].status = 'failed';
} finally { await client.close(); }
function percentile(values, fraction) {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.max(0, Math.ceil(sorted.length * fraction) - 1)];
}
const summary = corpus.flatMap(w => ['cli', 'mcp'].map(mode => {
  const all = rawRuns.filter(r => r.workload === w.id && r.mode === mode);
  const warm = all.filter(r => !r.firstForWorkload);
  const values = warm.map(r => r.elapsedMs);
  if (!values.length) return { workload: w.id, mode, warmSamples: 0, status: 'incomplete' };
  return { workload: w.id, mode, firstMs: all[0].elapsedMs, warmSamples: values.length,
    p50Ms: percentile(values, .5), p95Ms: percentile(values, .95), maxMs: Math.max(...values),
    completeReportPerSecond: 1000 / (values.reduce((a, b) => a + b, 0) / values.length) };
}));
const evidence = {
  schema: 'neokapi-positioning-benchmark/v1', recordedAt: new Date().toISOString(),
  sourceCommit: git('rev-parse', 'HEAD'), sourceDiffSha256: hash(git('diff', 'HEAD')), trackedDirty: git('status', '--porcelain', '--untracked-files=no') !== '',
  binaryVersion: version.run.stdout.trim(), binarySha256: hash(readFileSync(binary)),
  machine: { platform: platform(), arch: arch(), release: release(), cpu: cpus()[0]?.model, logicalCPUs: cpus().length, totalMemoryBytes: totalmem() },
  scope: 'Offline configured checks. Fresh CLI processes and one persistent MCP session. No ML plugin or provider.',
  limitations: [
    'Fresh process does not imply cold OS page cache. Each workload first invocation is separated; warm statistics exclude it. MCP startup/initialization is separately measured once; later workloads share that session.',
    'CLI and MCP share the same fixture but have different transport overhead. All requests are sequential, not a load test.',
    'Sparse edit alternates a planted prohibited phrase and its removal in one field; parity and updated findings are asserted. This does not imply incremental analysis is implemented.',
    'Whole-report wall time includes transport, serialization and client JSON parsing. There is no streaming first-finding measure in this synchronous path.',
    'Internal phase timings are recorded only when reported by the tested implementation; missing timings are null, never estimated.',
    'MCP resident memory snapshots are sampled after responses, outside latency measurement; they are not peak memory. CLI peak memory is unmeasured.',
    'Authored fixtures measure this workflow on this machine, not human quality, customer value or a general performance guarantee.',
  ],
  initialization: { elapsedMs: initializeMs, scope: 'Process spawn through MCP initialize/initialized, excluded from per-request warm latency.' },
  failure, ...(failure ? { mcpDiagnostics: client.diagnostics() } : {}),
  attempts,
  corpus: corpus.map(({ text, ...w }) => ({ ...w, initialInput: text })), summary, rawRuns, reports,
};
mkdirSync(dirname(output), { recursive: true });
writeFileSync(output, JSON.stringify(normalize(evidence), null, 2) + '\n');
console.log(JSON.stringify(summary, null, 2));
console.log(`Saved ${output}`);
rmSync(sandbox, { recursive: true, force: true });

if (failure) { console.error(JSON.stringify(failure)); process.exitCode = 1; }
