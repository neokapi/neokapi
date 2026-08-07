#!/usr/bin/env node
// Headless smoke test for the browser command surface: boots kapi-cli.wasm in
// Node and proves that every verb the lab can reach either runs or explains
// itself — never `unknown command`.
//
// This is the runtime half of the guard. The compile-time half is the Go test
// cli.TestBrowserCommandSurface, which pins cli.BrowserCommandSet against
// cli.KapiCommandSet so a verb cannot go missing from the browser build in the
// first place. This script checks the other failure mode: a verb that is
// registered but broken, and a lab component calling a verb by a spelling the
// CLI no longer has — which is exactly how /lab/convert came to answer every
// conversion with `unknown command "convert" for "kapi"` after #1180 renamed
// the toolbox proxies to their standalone binary names.
//
//   Run: node --experimental-strip-types scripts/verify-snippets/command-surface-smoke.ts
import { readFileSync } from "node:fs";
import { resolve as pathResolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { runInThisContext } from "node:vm";
import { createMemFS } from "./memfs.ts";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = pathResolve(__dirname, "../..");
const wasmDir = join(REPO_ROOT, "web/static/wasm");

const dec = new TextDecoder();
const enc = new TextEncoder();

let captured = "";
const mem = createMemFS({
  onStdout: (c: Uint8Array) => {
    captured += dec.decode(c);
  },
  onStderr: (c: Uint8Array) => {
    captured += dec.decode(c);
  },
});
(globalThis as any).fs = mem.fs;
(globalThis as any).process = Object.assign({}, process, mem.process, { env: process.env });

runInThisContext(readFileSync(join(wasmDir, "wasm_exec.js"), "utf8"));
const Go = (globalThis as any).Go;
const go = new Go();
// Mirror the browser exactly: @neokapi/engine boots with CLICOLOR_FORCE=1 so
// the playground terminal shows kapi's real styling. Capturing output therefore
// has to strip ANSI the way useLabRuntime.runCapture does — a smoke that turned
// colour off would not catch a lab component parsing coloured JSON.
go.env = { CLICOLOR_FORCE: "1", HOME: "/home", XDG_CACHE_HOME: "/cache" };
const ready = new Promise<void>((res) => ((globalThis as any).__kapiCliReady = res));
const { instance } = await WebAssembly.instantiate(
  readFileSync(join(wasmDir, "kapi-cli.wasm")),
  go.importObject,
);
void go.run(instance);
await ready;

// eslint-disable-next-line no-control-regex
const ANSI_ESCAPE = /\x1b\[[0-9;]*m/g;

async function run(...argv: string[]): Promise<{ code: number; out: string }> {
  captured = "";
  const code: number = await (globalThis as any).kapiRun(argv);
  return { code, out: captured.replace(ANSI_ESCAPE, "") };
}

let failures = 0;
function ok(label: string, cond: boolean, detail = ""): void {
  if (!cond) failures++;
  console.log(`${cond ? "  ok" : "FAIL"}: ${label}${detail ? `  — ${detail}` : ""}`);
}

// ── 1. No reachable verb answers "unknown command" ─────────────────────────
//
// The visible surface comes from the binary itself (cobra's completion
// endpoint), so this sweep widens automatically as commands are added. Hidden
// verbs are not completable, so the ones a user or a lab component types by
// name are listed explicitly; cli.TestBrowserCommandSurface is what guarantees
// the set is complete.
const HIDDEN_VERBS = ["kgrep", "ksed", "kcat", "kconv", "kdiff", "hook", "engine"];

console.log("command-surface-smoke: every reachable verb resolves");
const completion = await run("__complete", "");
ok("`__complete` enumerates the browser surface", completion.code === 0, `exit ${completion.code}`);
const completed = completion.out
  .split("\n")
  .map((l) => l.split("\t")[0].trim())
  .filter((l) => l && !l.startsWith(":") && !l.startsWith("Completion ended"));
ok("completion returned a plausible surface", completed.length >= 20, `${completed.length} verbs`);

const verbs = [...new Set([...completed, ...HIDDEN_VERBS])].sort();
for (const verb of verbs) {
  const r = await run(verb, "--help");
  ok(
    `\`kapi ${verb} --help\` resolves`,
    r.code === 0 && !/unknown command/.test(r.out),
    r.code === 0 ? "" : r.out.trim().split("\n")[0],
  );
}

// ── 2. Verbs the browser cannot run explain themselves ─────────────────────
//
// Every entry here is a cli.browserGaps key. The message must name the verb and
// the facility the browser denies — "unknown command" would tell a lab user the
// verb does not exist, which is false and unactionable.
console.log("command-surface-smoke: browser-unavailable verbs are honest");
const GAP_VERBS = ["plugin", "models", "credentials", "telemetry", "update", "mcp", "engine"];
for (const verb of GAP_VERBS) {
  // Bare, and with the subcommand a user would reach for — neither may fall
  // through to a second "unknown command" from an unregistered subcommand.
  for (const argv of [[verb], [verb, "list"]]) {
    const r = await run(...argv);
    const honest =
      r.code !== 0 &&
      r.out.includes(`kapi ${verb} is not available in the browser`) &&
      !/unknown command/.test(r.out);
    ok(`\`kapi ${argv.join(" ")}\` explains the browser limitation`, honest, r.out.trim());
  }
}

// ── 3. The verbs the labs actually call, with their real argv ──────────────
//
// Each block replays a lab component's own invocation. A rename that breaks the
// component fails here rather than in a reader's browser.
console.log("command-surface-smoke: lab components' real invocations");
mem.vol.mkdirp("/project");

// ConversionExplorer (packages/kapi-lab/src/ConversionExplorer.tsx): queries the
// generative writers, then converts through the toolbox `kconv` proxy.
const MARKDOWN = `# Release notes

A paragraph with **bold** and a [link](https://example.com).

| Format | Kind |
| ------ | ---- |
| md     | text |
`;
mem.vol.writeFile("/project/article.md", enc.encode(MARKDOWN));

const formats = await run("formats", "list", "--json");
ok("ConversionExplorer: `formats list --json` exits 0", formats.code === 0, formats.out.trim());
let generative: string[] = [];
try {
  const parsed = JSON.parse(formats.out) as {
    formats?: { name: string; has_writer?: boolean; generative?: boolean; interchange?: boolean }[];
  };
  generative = (parsed.formats ?? [])
    .filter((f) => f.has_writer && f.generative && !f.interchange)
    .map((f) => f.name);
} catch (e) {
  ok("ConversionExplorer: formats JSON parses", false, String(e));
}
ok(
  "ConversionExplorer: the engine reports generative targets",
  generative.length > 0,
  `${generative.length} targets`,
);

// Convert to each of ConversionExplorer's own GENERATIVE_TARGETS — the pills it
// opens on and falls back to. Every one must serialize a prose document, since
// that is what the Conversion Lab's samples are. (The engine reports a wider
// generative set, some of it keyed-catalog or media-only; the lab surfaces those
// too, and a pill whose writer rejects a given input reports the error inline.)
const LAB_TARGETS = ["doclang", "markdown", "html", "asciidoc", "json", "yaml", "plaintext"];
for (const fmt of LAB_TARGETS) {
  ok(
    `ConversionExplorer: the engine still reports \`${fmt}\` as generative`,
    generative.includes(fmt),
    generative.join(" "),
  );
}
for (const fmt of LAB_TARGETS) {
  const outPath = `/project/converted-${fmt}`;
  const r = await run("kconv", "/project/article.md", "--to", fmt, "-o", outPath);
  let written: Uint8Array | null = null;
  try {
    written = mem.vol.readFile(outPath);
  } catch {
    written = null;
  }
  ok(
    `ConversionExplorer: \`kconv --to ${fmt}\` writes output`,
    r.code === 0 && written != null && written.length > 0,
    r.code === 0 ? "" : r.out.trim().split("\n").slice(0, 2).join(" | "),
  );
}

// The regression that started this: the old spelling must stay gone, so nobody
// reintroduces a bare `convert` alias and quietly re-splits the surface.
const retired = await run("convert", "/project/article.md", "--to", "markdown");
ok(
  "the retired bare `convert` spelling is not a command",
  retired.code !== 0 && /unknown command/.test(retired.out),
  retired.out.trim(),
);

// WorkspaceExplorer (packages/kapi-lab/src/WorkspaceExplorer.tsx).
const ws = await run("extract", "/project/article.md", "-o", "/project/w.kpz", "--target-lang", "qps");
ok("WorkspaceExplorer: `extract` to a workspace exits 0", ws.code === 0, ws.out.trim());
const info = await run("info", "/project/w.kpz", "--json");
ok("WorkspaceExplorer: `info --json` exits 0", info.code === 0, info.out.trim());

// TryNeokapi modal (web/src/components/TryNeokapi/ModalBody.tsx).
const pseudo = await run(
  "pseudo-translate",
  "/project/article.md",
  "-o",
  "/project/out-article.md",
  "--target-lang",
  "fr",
);
ok("TryNeokapi: `pseudo-translate` exits 0", pseudo.code === 0, pseudo.out.trim());

// ── 4. Verbs newly reachable in the browser actually work ──────────────────
console.log("command-surface-smoke: newly wired verbs run for real");
const inspect = await run("inspect", "/project/article.md");
ok(
  "`kapi inspect` emits anchored blocks",
  inspect.code === 0 && inspect.out.includes("content_hash"),
  inspect.out.trim().slice(0, 160),
);
const check = await run("check", "/project/article.md", "--no-fail");
ok("`kapi check` runs the default checkset", check.code === 0, check.out.trim().slice(0, 160));
const brand = await run("voice", "guide", "--pack", "technical-docs");
ok("`kapi voice guide --pack` works offline", brand.code === 0, brand.out.trim().slice(0, 160));

// Installing a profile writes to the SQLite brand store, which the browser has
// no driver for. The failure must say so rather than blame a missing Go import
// ("unknown driver \"sqlite\" (forgotten import?)").
const brandStore = await run("voice", "pack", "technical-docs");
ok(
  "`kapi voice pack` reports the missing SQLite driver honestly",
  brandStore.code !== 0 && /not available in the browser build/.test(brandStore.out),
  brandStore.out.trim().slice(0, 240),
);

console.log(failures === 0 ? "\ncommand-surface-smoke: all checks passed" : `\ncommand-surface-smoke: ${failures} failure(s)`);
process.exit(failures === 0 ? 0 : 1);
