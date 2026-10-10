#!/usr/bin/env node
// scripts/learn-verify/verify.ts
//
// The learning labs' CI gate: boots kapi-cli.wasm in Node, seeds each lab's
// sample project into its own sandbox, runs every chapter through the same
// lab shell the browser player uses, and holds each command to the exit code
// and output substrings the chapter declares. A lab that drifts from the
// engine fails here before it fails for a reader.
//
// Usage:
//   node --experimental-strip-types scripts/learn-verify/verify.ts            # every lab
//   node --experimental-strip-types scripts/learn-verify/verify.ts <lab-id>…  # some labs
//   VERBOSE=1 … prints every command's output
// Or: make learn-verify

import { readFileSync, existsSync } from "node:fs";
import { resolve as pathResolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { runInThisContext } from "node:vm";
import { createMemFS } from "../verify-snippets/memfs.ts";
import { installSQLiteBridge, loadSQLite } from "../../packages/engine/src/sqlite.ts";
import { runLine, type ShellHost, type ShellSinks } from "../../packages/kapi-learn/src/shell/index.ts";
import { LABS } from "../../packages/kapi-learn/src/curriculum/index.ts";
import type { Lab } from "../../packages/kapi-learn/src/curriculum/index.ts";
import { SAMPLE_TREES } from "../../packages/kapi-learn/src/samples.gen.ts";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = pathResolve(__dirname, "../..");
const WASM_DIR = join(REPO_ROOT, "web/static/wasm");
const VERBOSE = !!process.env.VERBOSE;

const isTTY = process.stdout.isTTY;
const c = {
  green: (s: string) => (isTTY ? `\x1b[32m${s}\x1b[0m` : s),
  red: (s: string) => (isTTY ? `\x1b[31m${s}\x1b[0m` : s),
  dim: (s: string) => (isTTY ? `\x1b[2m${s}\x1b[0m` : s),
  bold: (s: string) => (isTTY ? `\x1b[1m${s}\x1b[0m` : s),
};

const dec = new TextDecoder();
const enc = new TextEncoder();

// eslint-disable-next-line no-control-regex
const ANSI = /\x1b\[[0-9;]*m/g;
const stripAnsi = (s: string) => s.replace(ANSI, "");

// ── Boot ─────────────────────────────────────────────────────────────────────

let currentOut: (s: string) => void = () => {};
let currentErr: (s: string) => void = () => {};

const mem = createMemFS({
  onStdout: (chunk: Uint8Array) => currentOut(dec.decode(chunk)),
  onStderr: (chunk: Uint8Array) => currentErr(dec.decode(chunk)),
});

async function boot(): Promise<void> {
  const wasmPath = join(WASM_DIR, "kapi-cli.wasm");
  if (!existsSync(wasmPath)) {
    console.error(c.red(`${wasmPath} not found; run \`make web-wasm-cli\` first`));
    process.exit(1);
  }
  (globalThis as any).fs = mem.fs;
  (globalThis as any).process = Object.assign({}, process, mem.process, { env: process.env });
  runInThisContext(readFileSync(join(WASM_DIR, "wasm_exec.js"), "utf8"));
  installSQLiteBridge(await loadSQLite());
  const Go = (globalThis as any).Go;
  const go = new Go();
  go.env = { CLICOLOR_FORCE: "0" };
  const ready = new Promise<void>((res) => ((globalThis as any).__kapiCliReady = res));
  const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
  void go.run(instance);
  await ready;
}

// ── The shell host over the engine's memfs ───────────────────────────────────

function hostFor(sandbox: string): ShellHost {
  const vol = mem.vol;
  return {
    runKapi: async (argv, sinks: ShellSinks) => {
      currentOut = sinks.out;
      currentErr = sinks.err;
      try {
        return (await (globalThis as any).kapiRun(argv)) as number;
      } finally {
        currentOut = () => {};
        currentErr = () => {};
      }
    },
    readFile: (p) => vol.readFile(p),
    writeFile: (p, d) => vol.writeFile(p, d),
    exists: (p) => vol.exists(p),
    isDir: (p) => vol.isDir(p),
    readdir: (p) => vol.readdir(p),
    mkdirp: (p) => vol.mkdirp(p),
    remove: (p) => vol.remove(p),
    cwd: () => mem.process.cwd?.() ?? sandbox,
    chdir: (d) => mem.process.chdir(d),
  };
}

function seed(lab: Lab, sandbox: string): void {
  mem.vol.mkdirp(sandbox);
  const tree = (SAMPLE_TREES as Record<string, readonly { path: string; content: string }[]>)[
    lab.sample
  ];
  for (const f of tree ?? []) {
    const abs = `${sandbox}/${f.path}`;
    const slash = abs.lastIndexOf("/");
    if (slash > 0) mem.vol.mkdirp(abs.slice(0, slash));
    mem.vol.writeFile(abs, enc.encode(f.content));
  }
  for (const f of lab.files ?? []) {
    const abs = `${sandbox}/${f.path}`;
    const slash = abs.lastIndexOf("/");
    if (slash > 0) mem.vol.mkdirp(abs.slice(0, slash));
    mem.vol.writeFile(abs, enc.encode(f.content));
  }
}

// ── Run one lab ──────────────────────────────────────────────────────────────

interface Failure {
  lab: string;
  chapter: string;
  reason: string;
  output: string;
}

async function runLab(lab: Lab): Promise<Failure[]> {
  const failures: Failure[] = [];
  const sandbox = `/learn/${lab.id}`;
  seed(lab, sandbox);
  const host = hostFor(sandbox);
  host.chdir(sandbox);

  const run = async (line: string) => {
    let out = "";
    const sinks: ShellSinks = {
      out: (s) => (out += s),
      err: (s) => (out += s),
    };
    const code = await runLine(host, line, sinks);
    return { code, output: stripAnsi(out) };
  };

  for (const line of lab.setup ?? []) {
    const r = await run(line);
    if (r.code !== 0) {
      failures.push({ lab: lab.id, chapter: "(setup)", reason: `${line} exited ${r.code}`, output: r.output });
      return failures;
    }
  }

  for (const ch of lab.chapters) {
    for (const f of ch.files ?? []) {
      const abs = `${sandbox}/${f.path}`;
      const slash = abs.lastIndexOf("/");
      if (slash > 0) mem.vol.mkdirp(abs.slice(0, slash));
      mem.vol.writeFile(abs, enc.encode(f.content));
    }
    if (ch.look?.file) {
      const abs = `${sandbox}/${ch.look.file}`;
      // A chapter may point at a file its own command creates; check after.
      if (!ch.command && !mem.vol.exists(abs)) {
        failures.push({ lab: lab.id, chapter: ch.id, reason: `look.file ${ch.look.file} does not exist`, output: "" });
      }
    }
    if (!ch.command) continue;
    const t0 = Date.now();
    const r = await run(ch.command);
    const ms = Date.now() - t0;
    const want = ch.exit ?? 0;
    const problems: string[] = [];
    if (r.code !== want) problems.push(`exit ${r.code}, expected ${want}`);
    for (const s of ch.expect ?? []) {
      if (!r.output.includes(s)) problems.push(`output lacks ${JSON.stringify(s)}`);
    }
    if (ch.look?.file && !mem.vol.exists(`${sandbox}/${ch.look.file}`)) {
      problems.push(`look.file ${ch.look.file} does not exist after the command`);
    }
    const label = `${lab.id}/${ch.id}`;
    if (problems.length) {
      console.log(`  ${c.red("✗")} ${label} ${c.dim(`(${ms} ms)`)}  ${c.dim(ch.command)}`);
      for (const p of problems) console.log(`      ${c.red(p)}`);
      failures.push({ lab: lab.id, chapter: ch.id, reason: problems.join("; "), output: r.output });
    } else {
      console.log(`  ${c.green("✓")} ${label} ${c.dim(`(${ms} ms)`)}  ${c.dim(ch.command)}`);
    }
    if (VERBOSE || problems.length) {
      const shown = r.output.split("\n").slice(0, VERBOSE ? 400 : 30).join("\n      ");
      if (shown.trim()) console.log(c.dim("      " + shown));
    }
  }
  return failures;
}

// ── Main ─────────────────────────────────────────────────────────────────────

async function main(): Promise<void> {
  const wanted = process.argv.slice(2);
  // Explorer labs drive a browser component rather than the shell; their
  // chapters carry no command to hold to.
  const terminalLabs = LABS.filter((l) => l.kind !== "explorer" && l.kind !== "playground");
  const labs = wanted.length ? terminalLabs.filter((l) => wanted.includes(l.id)) : terminalLabs;
  if (wanted.length && labs.length !== wanted.length) {
    const known = new Set(LABS.map((l) => l.id));
    for (const w of wanted) if (!known.has(w)) console.error(c.red(`unknown lab: ${w}`));
    process.exit(2);
  }
  await boot();
  const all: Failure[] = [];
  let chapters = 0;
  const t0 = Date.now();
  for (const lab of labs) {
    console.log(`\n${c.bold(lab.title)} ${c.dim(`(${lab.id}, ${lab.sample})`)}`);
    chapters += lab.chapters.filter((ch) => ch.command).length;
    all.push(...(await runLab(lab)));
  }
  const secs = ((Date.now() - t0) / 1000).toFixed(1);
  console.log("");
  if (all.length === 0) {
    console.log(c.green(`learn-verify: ${labs.length} lab(s), ${chapters} command(s) green in ${secs}s`));
    return;
  }
  console.log(c.red(`learn-verify: ${all.length} failure(s) in ${secs}s`));
  for (const f of all) console.log(`  ${f.lab}/${f.chapter}: ${f.reason}`);
  process.exit(1);
}

await main();
