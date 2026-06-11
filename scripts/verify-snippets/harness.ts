#!/usr/bin/env node
// verify-snippets/harness.ts
//
// W7 CI gate: boots kapi-cli.wasm in Node, discovers every non-editable
// <RunnableSnippet cmd> in the docs MDX files and every smoke_contract entry
// in web/walkthroughs/*.md scene specs, then runs each via kapiRun and
// asserts a zero exit code.
//
// Editable snippets (prop `editable` present) are captured but NOT
// exit-code-asserted — they're starting-point templates the reader completes.
//
// Usage:  node --experimental-strip-types scripts/verify-snippets/harness.ts
// Or:     make docs-verify-snippets

import { readFileSync, readdirSync, existsSync } from "node:fs";
import { resolve as pathResolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { createMemFS } from "./memfs.ts";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const REPO_ROOT = pathResolve(__dirname, "../..");

// ── ANSI colours ─────────────────────────────────────────────────────────────

const isTTY = process.stdout.isTTY;
const c = {
  green: (s: string) => (isTTY ? `\x1b[32m${s}\x1b[0m` : s),
  red:   (s: string) => (isTTY ? `\x1b[31m${s}\x1b[0m` : s),
  yellow:(s: string) => (isTTY ? `\x1b[33m${s}\x1b[0m` : s),
  bold:  (s: string) => (isTTY ? `\x1b[1m${s}\x1b[0m`  : s),
  dim:   (s: string) => (isTTY ? `\x1b[2m${s}\x1b[0m`  : s),
};

// ── Fixture library ───────────────────────────────────────────────────────────

const enc = new TextEncoder();

const FIXTURES: Record<string, string> = {
  "messages.json": JSON.stringify(
    { greeting: "Hello, World!", farewell: "See you tomorrow", items: { cart: "Your cart is empty" } },
    null, 2,
  ),
  "app.xliff": `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="app.json" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="greeting"><source>Hello, World!</source></trans-unit>
      <trans-unit id="farewell"><source>See you tomorrow</source></trans-unit>
      <trans-unit id="cart.empty"><source>Your cart is empty</source></trans-unit>
    </body>
  </file>
</xliff>`,
  "page.html": `<!doctype html>
<html lang="en">
  <head><meta charset="utf-8" /><title>Welcome</title></head>
  <body>
    <h1>Welcome aboard</h1>
    <p>Thanks for trying <strong>kapi</strong>.</p>
    <a href="/docs">Read the documentation</a>
  </body>
</html>`,
  "README.md": `# Project Title\n\nThanks for trying **kapi**.\n\n## Getting started\n\n- Install the CLI\n- Run \`kapi word-count README.md\`\n`,
  "app.properties": `# Application strings\napp.title = Welcome aboard\napp.greeting = Hello, World!\napp.farewell = See you tomorrow\ncart.empty = Your cart is empty\n`,
  "strings.xml": `<?xml version="1.0" encoding="utf-8"?>\n<resources>\n  <string name="app_name">Welcome aboard</string>\n  <string name="greeting">Hello, World!</string>\n  <string name="farewell">See you tomorrow</string>\n  <string name="cart_empty">Your cart is empty</string>\n</resources>\n`,
  "Localizable.xcstrings": JSON.stringify(
    {
      sourceLanguage: "en",
      strings: {
        greeting: { localizations: { en: { stringUnit: { state: "translated", value: "Hello, World!" } } } },
        farewell:  { localizations: { en: { stringUnit: { state: "translated", value: "See you tomorrow" } } } },
        "cart.empty": { localizations: { en: { stringUnit: { state: "translated", value: "Your cart is empty" } } } },
      },
      version: "1.0",
    },
    null, 2,
  ),
};

// ── Types ─────────────────────────────────────────────────────────────────────

interface SnippetEntry {
  source:   string;   // display label (file:line or "walkthrough:file#scene")
  cmd:      string;   // full command string (may start with "kapi ")
  seed:     string[]; // fixture names to write before running
  editable: boolean;  // if true: capture but don't assert exit code
}

// ── Discovery: RunnableSnippet in MDX ────────────────────────────────────────

function findMdxFiles(dir: string): string[] {
  const results: string[] = [];
  if (!existsSync(dir)) return results;
  for (const entry of readdirSync(dir, { recursive: true, withFileTypes: true })) {
    if (entry.isFile() && (entry.name.endsWith(".mdx") || entry.name.endsWith(".md"))) {
      const parent = (entry as any).parentPath ?? (entry as any).path;
      results.push(join(parent, entry.name));
    }
  }
  return results;
}

function extractRunnableSnippets(filePath: string): SnippetEntry[] {
  const content = readFileSync(filePath, "utf8");
  const entries: SnippetEntry[] = [];
  const tagPattern = /<RunnableSnippet/g;
  let match: RegExpExecArray | null;

  while ((match = tagPattern.exec(content)) !== null) {
    const start = match.index;
    // Self-closing /> takes priority; if the > comes first it's an open tag.
    const selfClose = content.indexOf("/>", start);
    const openClose = content.indexOf(">", start);
    const end = (selfClose !== -1 && (openClose === -1 || selfClose <= openClose))
      ? selfClose + 2
      : openClose + 1;
    if (end < 2) continue;

    const tagText = content.slice(start, end);

    // cmd="..." (required)
    const cmdMatch = tagText.match(/cmd=\{?"([^"]+)"\}?/);
    if (!cmdMatch) continue;
    const cmd = cmdMatch[1];

    // seed={["a", "b"]} (optional)
    const seedMatch = tagText.match(/seed=\{\[([^\]]*)\]\}/);
    const seed: string[] = seedMatch
      ? seedMatch[1].split(",").map((s) => s.trim().replace(/^["']|["']$/g, "")).filter(Boolean)
      : [];

    // editable (bare boolean prop — presence means true)
    const editable = /\beditable\b/.test(tagText);

    const lineNo = content.slice(0, start).split("\n").length;
    const relPath = filePath.replace(REPO_ROOT + "/", "");
    entries.push({ source: `${relPath}:${lineNo}`, cmd, seed, editable });
  }

  return entries;
}

// ── Discovery: smoke_contract in walkthrough front matter ─────────────────────

interface WalkthroughScene {
  id:              string;
  fixtures?:       string[];
  smoke_contract?: string[];
}

// Minimal line-by-line YAML parser for our specific scene-spec schema.
function parseWalkthroughFrontMatter(content: string): { id: string; scenes: WalkthroughScene[] } | null {
  const fmMatch = content.match(/^---\n([\s\S]*?)\n---/);
  if (!fmMatch) return null;
  const fm = fmMatch[1];

  const idMatch = fm.match(/^id:\s*(.+)$/m);
  const id = idMatch ? idMatch[1].trim() : "unknown";

  // Extract everything after "scenes:" — it's the rest of the front matter.
  const scenesStartMatch = fm.match(/^scenes:\n([\s\S]+)/m);
  if (!scenesStartMatch) return { id, scenes: [] };

  const scenesBlock = scenesStartMatch[1];
  const scenes: WalkthroughScene[] = [];

  // Split on scene items ("  - id: ...")
  const sceneChunks = scenesBlock.split(/\n(?=  - id:)/);

  for (const chunk of sceneChunks) {
    const sceneIdMatch = chunk.match(/- id:\s*(.+)/);
    if (!sceneIdMatch) continue;
    const sceneId = sceneIdMatch[1].trim();

    const lines = chunk.split("\n");

    // Extract list values under a given key by parsing line-by-line.
    function extractList(key: string): string[] {
      const result: string[] = [];
      let capturing = false;
      for (const line of lines) {
        if (line.match(new RegExp(`^\\s+${key}:`))) { capturing = true; continue; }
        if (capturing) {
          const m = line.match(/^\s+-\s+(.+)/);
          if (m) result.push(m[1].trim());
          else if (line.match(/^\s+\w+:/) && !line.match(/^\s{6,}/)) capturing = false;
        }
      }
      return result;
    }

    scenes.push({
      id:              sceneId,
      fixtures:        extractList("fixtures"),
      smoke_contract:  extractList("smoke_contract"),
    });
  }

  return { id, scenes };
}

function loadWalkthroughSnippets(walkthroughsDir: string): SnippetEntry[] {
  const entries: SnippetEntry[] = [];
  if (!existsSync(walkthroughsDir)) return entries;

  for (const file of readdirSync(walkthroughsDir)) {
    if (!file.endsWith(".md") && !file.endsWith(".yaml") && !file.endsWith(".yml")) continue;
    const filePath = join(walkthroughsDir, file);
    const content = readFileSync(filePath, "utf8");
    const fm = parseWalkthroughFrontMatter(content);
    if (!fm) continue;

    for (const scene of fm.scenes) {
      if (!scene.smoke_contract || scene.smoke_contract.length === 0) continue;
      // Only seed fixtures that exist in our fixture library.
      const seed = (scene.fixtures ?? []).filter((f) => f in FIXTURES);
      for (const cmd of scene.smoke_contract) {
        entries.push({
          source:   `walkthroughs/${file}#${scene.id}`,
          cmd,
          seed,
          editable: false, // smoke_contract entries are always auto-run
        });
      }
    }
  }

  return entries;
}

// ── WASM boot (once per process) ─────────────────────────────────────────────

let wasmBooted = false;

async function bootWasm(wasmExecPath: string, wasmPath: string): Promise<void> {
  if (wasmBooted) return;

  const dec = new TextDecoder();

  const mem = createMemFS({
    onStdout: (chunk) => (globalThis as any).__kapiStdout?.(dec.decode(chunk)),
    onStderr: (chunk) => (globalThis as any).__kapiStderr?.(dec.decode(chunk)),
  });

  // Install BEFORE wasm_exec.js so its `if (!globalThis.fs)` guards leave ours.
  (globalThis as any).fs = mem.fs;
  const existingProc = (globalThis as any).process || {};
  (globalThis as any).process = Object.assign({}, existingProc, mem.process, {
    env: (existingProc.env ?? {}),
  });
  (globalThis as any).__kapiVol        = mem.vol;
  (globalThis as any).__kapiMemProcess = mem.process;

  // wasm_exec.js sets globalThis.Go when executed.
  const wasmExecSrc = readFileSync(wasmExecPath, "utf8");
  new Function(wasmExecSrc)();

  const Go = (globalThis as any).Go;
  if (!Go) throw new Error("wasm_exec.js did not define globalThis.Go");

  const go = new Go();
  go.env = { CLICOLOR_FORCE: "1" };

  const ready = new Promise<void>((res) => {
    (globalThis as any).__kapiCliReady = res;
  });

  const wasmBytes = readFileSync(wasmPath);
  const result = await WebAssembly.instantiate(wasmBytes, go.importObject);
  void go.run(result.instance); // runs forever inside select{}
  await ready;

  wasmBooted = true;
}

// ── Per-command runner ────────────────────────────────────────────────────────

interface RunResult {
  code:   number;
  stdout: string;
  stderr: string;
}

async function runCommand(argv: string[]): Promise<RunResult> {
  let stdout = "";
  let stderr = "";

  (globalThis as any).__kapiStdout = (s: string) => { stdout += s; };
  (globalThis as any).__kapiStderr = (s: string) => { stderr += s; };

  const code: number = await (globalThis as any).kapiRun(argv);

  (globalThis as any).__kapiStdout = undefined;
  (globalThis as any).__kapiStderr = undefined;

  return { code, stdout, stderr };
}

// ── Fixture seeding ───────────────────────────────────────────────────────────

function seedFixtures(fixtureNames: string[]): void {
  const vol = (globalThis as any).__kapiVol;
  for (const name of fixtureNames) {
    const content = FIXTURES[name];
    if (content === undefined) {
      console.warn(c.yellow(`  [warn] fixture "${name}" not in library — skipping`));
      continue;
    }
    vol.writeFile(`/project/${name}`, enc.encode(content));
  }
}

function resetCwd(): void {
  const proc = (globalThis as any).__kapiMemProcess;
  if (proc?.chdir) {
    try { proc.chdir("/project"); } catch { /* ignore */ }
  }
}

// ── Skip patterns: commands unsupported by the browser-safe wasm subset ───────
//
// The wasm build omits subprocess plugins, native SQLite, and complex
// fixture directories. Such commands are skip-listed rather than failed so
// CI stays honest about what the wasm actually covers.

const WASM_UNSUPPORTED = [
  /bilingual-project/,
  /\.tmx\b/,
  /\.db\b/,
  // "fixtures/" path prefix — walkthroughs use a real fixtures dir that
  // only exists when running the native binary. Works with or without a
  // leading slash: `fixtures/foo` or `/fixtures/foo`.
  /\bfixtures\//,
  /\bsamples\//,
  /\.kapi\b/,
  /glossary\.csv/,
  /messages_en\.json/,
];

function shouldSkipForWasm(cmd: string): boolean {
  return WASM_UNSUPPORTED.some((p) => p.test(cmd));
}

// ── Main ─────────────────────────────────────────────────────────────────────

async function main() {
  const wasmDir      = join(REPO_ROOT, "web/static/wasm");
  const wasmExecPath = join(wasmDir, "wasm_exec.js");
  const wasmPath     = join(wasmDir, "kapi-cli.wasm");
  const docsDir      = join(REPO_ROOT, "web/docs");
  const walkthroughsDir = join(REPO_ROOT, "web/walkthroughs");

  if (!existsSync(wasmPath)) {
    console.error(c.red(`\nERROR: ${wasmPath} not found.`));
    console.error(c.red("Run `make web-wasm-cli` to build it first.\n"));
    process.exit(1);
  }
  if (!existsSync(wasmExecPath)) {
    console.error(c.red(`\nERROR: ${wasmExecPath} not found.\n`));
    process.exit(1);
  }

  // ── Discovery ─────────────────────────────────────────────────────────────

  const mdxSnippets: SnippetEntry[] = [];
  for (const file of findMdxFiles(docsDir)) {
    mdxSnippets.push(...extractRunnableSnippets(file));
  }
  const walkthroughSnippets = loadWalkthroughSnippets(walkthroughsDir);
  const allSnippets = [...mdxSnippets, ...walkthroughSnippets];

  if (allSnippets.length === 0) {
    console.log(c.yellow("No RunnableSnippet commands or smoke_contract entries found."));
    process.exit(0);
  }

  const autoRun  = allSnippets.filter((s) => !s.editable);
  const editables = allSnippets.filter((s) => s.editable);

  console.log(c.bold(`\nkapi snippet verifier`));
  console.log(c.dim(`  ${allSnippets.length} discovered  (${autoRun.length} auto-run, ${editables.length} editable/skipped)`));
  console.log(c.dim(`  wasm : ${wasmPath}`));
  console.log(c.dim(`  docs : ${docsDir}`));
  console.log(c.dim(`  walks: ${walkthroughsDir}`));
  console.log();

  // ── Boot ──────────────────────────────────────────────────────────────────

  process.stdout.write("Booting kapi-cli.wasm … ");
  await bootWasm(wasmExecPath, wasmPath);
  console.log(c.green("ready\n"));

  // ── Run ───────────────────────────────────────────────────────────────────

  const outcomes: Array<{ snippet: SnippetEntry; result: RunResult; pass: boolean }> = [];
  let skipCount = 0;

  for (const snippet of allSnippets) {
    const rawCmd = snippet.cmd.trim();

    // Editable snippets: display but do not assert.
    if (snippet.editable) {
      console.log(`  ${c.yellow("EDIT")} ${rawCmd}  ${c.dim("(editable — not asserted)")}`);
      console.log(`       ${c.dim(snippet.source)}\n`);
      skipCount++;
      continue;
    }

    // Commands referencing fixtures/paths not supported in the browser wasm.
    if (shouldSkipForWasm(rawCmd)) {
      console.log(`  ${c.yellow("SKIP")} ${rawCmd}  ${c.dim("(requires native binary/fixture)")}`);
      console.log(`       ${c.dim(snippet.source)}\n`);
      skipCount++;
      continue;
    }

    // Strip leading "kapi " — kapiRun receives argv without the binary name.
    const argv = rawCmd.replace(/^kapi\s+/, "").trim().split(/\s+/).filter(Boolean);

    resetCwd();
    seedFixtures(snippet.seed);

    console.log(`  ${c.bold("RUN ")} ${rawCmd}`);

    const result = await runCommand(argv);
    const pass   = result.code === 0;
    outcomes.push({ snippet, result, pass });

    if (pass) {
      console.log(`       ${c.green("✓ exit 0")}  ${c.dim(snippet.source)}`);
    } else {
      console.log(`       ${c.red(`✗ exit ${result.code}`)}  ${c.dim(snippet.source)}`);
      if (result.stderr.trim()) {
        for (const line of result.stderr.trim().split("\n")) {
          console.log(`       ${c.red("│")} ${c.red(line)}`);
        }
      }
    }

    // Preview first 5 lines of stdout.
    if (result.stdout.trim()) {
      const preview = result.stdout.trim().split("\n").slice(0, 5);
      for (const line of preview) {
        console.log(`       ${c.dim("│")} ${c.dim(line)}`);
      }
    }
    console.log();
  }

  // ── Summary ───────────────────────────────────────────────────────────────

  const passed = outcomes.filter((r) => r.pass).length;
  const failed = outcomes.filter((r) => !r.pass).length;

  console.log(c.bold("─".repeat(60)));
  console.log(
    `  ${c.green(`${passed} passed`)}   ` +
    `${failed > 0 ? c.red(`${failed} failed`) : `${failed} failed`}   ` +
    `${c.dim(`${skipCount} skipped/editable`)}`,
  );

  if (failed > 0) {
    console.log(c.red("\nFailed commands:"));
    for (const r of outcomes.filter((r) => !r.pass)) {
      console.log(`  ${c.red("✗")} ${r.snippet.cmd}  ${c.dim(`(${r.snippet.source})`)}`);
    }
    console.log();
    process.exit(1);
  } else {
    console.log(c.green("\nAll verifiable snippets passed. ✓\n"));
    process.exit(0);
  }
}

main().catch((err) => {
  console.error(c.red("\nFatal:"), err);
  process.exit(1);
});
