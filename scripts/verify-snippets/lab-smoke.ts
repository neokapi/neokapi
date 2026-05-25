#!/usr/bin/env node
// Headless smoke test for the lab WASM surface: boots kapi-cli.wasm in Node,
// seeds a sample file, then exercises (1) labInspect → ContentTree JSON and
// (2) the --trace path → FlowTrace JSON read back from memfs. Mirrors the boot
// logic in harness.ts. Run: node --experimental-strip-types scripts/verify-snippets/lab-smoke.ts
import { readFileSync } from "node:fs";
import { resolve as pathResolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { runInThisContext } from "node:vm";
import { createMemFS } from "./memfs.ts";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = pathResolve(__dirname, "../..");
const wasmDir = join(REPO_ROOT, "web/docs/static/wasm");

const dec = new TextDecoder();
const enc = new TextEncoder();

const mem = createMemFS({
  onStdout: (c: Uint8Array) => process.stderr.write(dec.decode(c)),
  onStderr: (c: Uint8Array) => process.stderr.write(dec.decode(c)),
});
(globalThis as any).fs = mem.fs;
(globalThis as any).process = Object.assign({}, process, mem.process, { env: process.env });

// wasm_exec.js is a classic script that sets globalThis.Go; run it in the
// current global context (the same trick harness.ts uses, without new Function).
runInThisContext(readFileSync(join(wasmDir, "wasm_exec.js"), "utf8"));
const Go = (globalThis as any).Go;
const go = new Go();
go.env = { CLICOLOR_FORCE: "0" };
const ready = new Promise<void>((res) => ((globalThis as any).__kapiCliReady = res));
const { instance } = await WebAssembly.instantiate(readFileSync(join(wasmDir, "kapi-cli.wasm")), go.importObject);
void go.run(instance);
await ready;

const SAMPLE = JSON.stringify(
  { greeting: "Hello, {name}!", cart: { empty: "Your cart is empty" }, farewell: "See you tomorrow" },
  null, 2,
);
mem.vol.writeFile("/project/sample.json", enc.encode(SAMPLE));

let failures = 0;
const ok = (label: string, cond: boolean, detail = "") => {
  console.log(`${cond ? "✓" : "✗"} ${label}${detail ? "  " + detail : ""}`);
  if (!cond) failures++;
};

// ── 1. labInspect → ContentTree ──────────────────────────────────────────────
const inspectRes: any = await (globalThis as any).labInspect("/project/sample.json");
ok("labInspect returns ok", inspectRes?.ok === true, JSON.stringify(inspectRes?.error ?? ""));
const tree = JSON.parse(inspectRes.json);
ok("labInspect format is native json (not okapi bridge)", tree.format === "json", `format=${tree.format}`);
ok("labInspect found blocks", tree.stats.blocks >= 3, `stats=${JSON.stringify(tree.stats)}`);
ok("labInspect root has a layer", tree.root[0]?.kind === "layer", `kind=${tree.root[0]?.kind}`);
const allBlocks: any[] = [];
const walk = (n: any) => { if (n.kind === "block") allBlocks.push(n); (n.children ?? []).forEach(walk); };
tree.root.forEach(walk);
const greeting = allBlocks.find((b) => /Hello/.test(JSON.stringify(b.source)));
ok("greeting block has run sequence", Array.isArray(greeting?.source) && greeting.source.length >= 1);
console.log("   greeting block id:", greeting?.id, "| runs:", JSON.stringify(greeting?.source));

// ── 2. --trace → FlowTrace via memfs ─────────────────────────────────────────
const code: number = await (globalThis as any).kapiRun([
  "pseudo-translate", "/project/sample.json", "-o", "/project/out.json", "--trace", "/project/trace.json",
]);
ok("pseudo-translate --trace exits 0", code === 0, `code=${code}`);
const trace = JSON.parse(dec.decode(mem.vol.readFile("/project/trace.json")));
ok("trace has reader/tool/writer nodes", trace.nodes?.length === 3, `nodes=${JSON.stringify(trace.nodes?.map((n: any) => `${n.id}:${n.name}`))}`);
ok("trace native json reader (not bridge)", trace.nodes?.[0]?.name === "json", `reader=${trace.nodes?.[0]?.name}`);
ok("trace has events", trace.events?.length > 0, `events=${trace.events?.length}`);
ok("trace has part snapshots", Object.keys(trace.parts ?? {}).length > 0, `parts=${Object.keys(trace.parts ?? {}).length}`);
const withAfter = Object.entries(trace.parts ?? {}).find(([, v]: any) => v.afterNode && Object.keys(v.afterNode).length);
ok("a part has before/after snapshots", !!withAfter);
if (withAfter) {
  const [pid, set]: any = withAfter;
  console.log(`   part ${pid}: source="${set.initial.sourceText}" → after tool target="${Object.values(set.afterNode)[0]?.targetText ?? "(none)"}"`);
}

// ── 3. recipe-based custom flow → trace (the ToolLab / FlowBuilder path) ─────
const RECIPE = `version: v1
name: Lab
defaults:
  source_language: en
flows:
  lab:
    steps:
      - tool: pseudo-translate
`;
mem.vol.writeFile("/project/lab.kapi", enc.encode(RECIPE));
const rcode: number = await (globalThis as any).kapiRun([
  "run", "lab", "-p", "/project/lab.kapi", "-i", "/project/sample.json",
  "-o", "/project/out-recipe.json", "--target-lang", "qps", "--trace", "/project/rtrace.json",
]);
ok("recipe flow run exits 0", rcode === 0, `code=${rcode}`);
const rtrace = JSON.parse(dec.decode(mem.vol.readFile("/project/rtrace.json")));
ok("recipe trace tool node is named (not tool-N)", rtrace.nodes?.some((n: any) => n.name === "pseudo-translate"), `names=${JSON.stringify(rtrace.nodes?.map((n: any) => n.name))}`);
ok("recipe trace has part snapshots", Object.keys(rtrace.parts ?? {}).length > 0, `parts=${Object.keys(rtrace.parts ?? {}).length}`);

// ── 4. AI flow runs offline via the demo provider (FlowBuilder / ai pipelines) ─
// A credential-requiring tool (ai-translate) inside a recipe flow must be
// coerced to the deterministic demo provider — otherwise it hits the real
// network (api.anthropic.com), which is unreachable in the browser/Node.
const AI_RECIPE = `version: v1
name: Lab
defaults:
  source_language: en
flows:
  lab:
    steps:
      - tool: ai-translate
`;
mem.vol.writeFile("/project/ai.kapi", enc.encode(AI_RECIPE));
const aicode: number = await (globalThis as any).kapiRun([
  "run", "lab", "-p", "/project/ai.kapi", "-i", "/project/sample.json",
  "-o", "/project/out-ai.json", "--target-lang", "fr", "--trace", "/project/aitrace.json",
]);
ok("ai-translate flow runs offline (demo provider) exits 0", aicode === 0, `code=${aicode}`);
const aitrace = JSON.parse(dec.decode(mem.vol.readFile("/project/aitrace.json")));
ok("ai flow trace has part snapshots", Object.keys(aitrace.parts ?? {}).length > 0, `parts=${Object.keys(aitrace.parts ?? {}).length}`);

// ── 5. the script tool runs user JS in WASM (goja) — the Script Lab path ──────
// JS uses single quotes so it sits cleanly inside a double-quoted YAML scalar.
const SCRIPT_RECIPE = `version: v1
name: Lab
defaults:
  source_language: en
flows:
  lab:
    steps:
      - tool: script
        config:
          code: "if (part.type === 'block') { part.block.source[0].content.text = part.block.source[0].content.text.toUpperCase(); } emit(part);"
`;
mem.vol.writeFile("/project/script.kapi", enc.encode(SCRIPT_RECIPE));
const scode: number = await (globalThis as any).kapiRun([
  "run", "lab", "-p", "/project/script.kapi", "-i", "/project/sample.json",
  "-o", "/project/out-script.json", "--target-lang", "qps", "--trace", "/project/scripttrace.json",
]);
ok("script tool (goja) runs user JS in WASM, exits 0", scode === 0, `code=${scode}`);
const scriptOut = dec.decode(mem.vol.readFile("/project/out-script.json"));
ok("script transformed block text (uppercased)", /HELLO|YOUR CART IS EMPTY|SEE YOU TOMORROW/.test(scriptOut), scriptOut.slice(0, 80));

// ── 6. HTML round-trip is byte-exact (in-memory skeleton works in WASM) ───────
// Without a writable temp FS the skeleton store must fall back to memory; if it
// doesn't, the HTML writer re-serializes and normalizes doctype case / spacing.
const HTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Welcome</title>
  </head>
  <body><p>Thanks for trying <strong>kapi</strong>.</p></body>
</html>
`;
mem.vol.writeFile("/project/page.html", enc.encode(HTML));
await (globalThis as any).kapiRun(["pseudo-translate", "/project/page.html", "-o", "/project/out.html", "--target-lang", "qps"]);
const htmlOut = dec.decode(mem.vol.readFile("/project/out.html"));
ok("html round-trip keeps lowercase doctype", htmlOut.includes("<!doctype html>"), htmlOut.slice(0, 20));
ok("html round-trip keeps self-closing spacing", /<meta charset="utf-8" \/>/.test(htmlOut));
ok("html round-trip rewrites lang + keeps inline tag", htmlOut.includes('lang="qps"') && htmlOut.includes("<strong>"));

// ── 7. script tool function form (process(part)) runs in WASM ────────────────
const FN_RECIPE = `version: v1
name: Lab
defaults:
  source_language: en
flows:
  lab:
    steps:
      - tool: script
        config:
          code: "function process(part) { if (part.type === 'block') { part.block.source[0].content.text = part.block.source[0].content.text.toUpperCase(); } return part; }"
`;
mem.vol.writeFile("/project/fn.kapi", enc.encode(FN_RECIPE));
const fncode: number = await (globalThis as any).kapiRun([
  "run", "lab", "-p", "/project/fn.kapi", "-i", "/project/sample.json",
  "-o", "/project/out-fn.json", "--target-lang", "qps",
]);
ok("script function form process(part) exits 0", fncode === 0, `code=${fncode}`);
const fnOut = dec.decode(mem.vol.readFile("/project/out-fn.json"));
ok("script function form transformed text (uppercased)", /HELLO|YOUR CART IS EMPTY/.test(fnOut), fnOut.slice(0, 60));

console.log(failures === 0 ? "\nALL LAB SMOKE CHECKS PASSED" : `\n${failures} CHECK(S) FAILED`);
process.exit(failures === 0 ? 0 : 1);
