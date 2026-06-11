// klz-wasm-smoke: prove the .klz workspace + .kapi project workflows run in
// the *browser* WASM engine (no SQLite, no real filesystem) — including
// binary Office formats, not just text (AD-025 §5 / #787). Boots the real
// kapi-cli.wasm in Node against the in-memory filesystem and drives the
// commands exactly as the docs lab would.
//
// Usage: node --experimental-strip-types scripts/klz-wasm-smoke.ts \
//          [web/static/wasm/kapi-cli.wasm] [.../wasm_exec.js]
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { createMemFS } from "./verify-snippets/memfs.ts";

const wasmPath = process.argv[2] ?? "web/static/wasm/kapi-cli.wasm";
const wasmExecPath =
  process.argv[3] ??
  join(process.env.GOROOT ?? "", "lib/wasm/wasm_exec.js");

const dec = new TextDecoder();
const enc = new TextEncoder();
let captured = "";

const mem = createMemFS({
  onStdout: (c: Uint8Array) => (captured += dec.decode(c)),
  onStderr: (c: Uint8Array) => (captured += dec.decode(c)),
});
(globalThis as any).fs = mem.fs;
(globalThis as any).process = Object.assign({}, (globalThis as any).process, mem.process, { env: {} });
(globalThis as any).__kapiVol = mem.vol;
(globalThis as any).__kapiMemProcess = mem.process;

new Function(readFileSync(wasmExecPath, "utf8"))();
const go = new (globalThis as any).Go();
go.env = { HOME: "/home", XDG_CACHE_HOME: "/cache" };
const ready = new Promise<void>((r) => ((globalThis as any).__kapiCliReady = r));
const { instance } = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject);
void go.run(instance);
await ready;

async function run(...argv: string[]): Promise<{ code: number; out: string }> {
  captured = "";
  const code: number = await (globalThis as any).kapiRun(argv);
  return { code, out: captured.trim() };
}
function ok(label: string, r: { code: number; out: string }): void {
  if (r.code !== 0) {
    console.error(`FAIL: ${label} (exit ${r.code})\n${r.out}`);
    process.exit(1);
  }
  console.log(`  ok: ${label}`);
}

mem.vol.mkdirp("/p");

// 1) .klz workspace lifecycle on a text format.
console.log("klz-wasm-smoke: .klz workspace (JSON)");
mem.vol.writeFile("/p/app.json", enc.encode('{"a":"Hello world"}'));
ok("extract", await run("extract", "/p/app.json", "-o", "/p/w.klz", "--target-lang", "qps"));
ok("transform", await run("pseudo-translate", "/p/w.klz"));
const dirty = await run("info", "/p/w.klz");
if (!/dirty/.test(dirty.out)) { console.error("FAIL: info should report dirty\n" + dirty.out); process.exit(1); }
ok("info(dirty)", dirty);
ok("pack", await run("pack", "/p/w.klz"));
const clean = await run("info", "/p/w.klz");
if (!/clean/.test(clean.out)) { console.error("FAIL: info should report clean after pack\n" + clean.out); process.exit(1); }
ok("info(clean)", clean);
ok("merge", await run("merge", "/p/w.klz", "-o", "/p/out/"));
const jsonOut = dec.decode(mem.vol.readFile("/p/out/app.json"));
if (!/[-￿]/.test(jsonOut)) { console.error("FAIL: merged JSON not pseudo-translated: " + jsonOut); process.exit(1); }

// 2) Binary Office format (.docx) through the same workflow.
console.log("klz-wasm-smoke: .klz workspace (Office .docx)");
const docx = new Uint8Array(readFileSync("core/formats/openxml/testdata/test_859.docx"));
mem.vol.writeFile("/p/sample.docx", docx);
ok("extract docx", await run("extract", "/p/sample.docx", "-o", "/p/d.klz", "--target-lang", "qps"));
ok("transform docx", await run("pseudo-translate", "/p/d.klz"));
ok("merge docx", await run("merge", "/p/d.klz", "-o", "/p/dout/"));
const docxOut = mem.vol.readFile("/p/dout/sample.docx");
if (!(docxOut[0] === 0x50 && docxOut[1] === 0x4b)) { console.error("FAIL: merged .docx is not a valid zip"); process.exit(1); }
console.log(`  ok: merged .docx is a valid OOXML zip (${docxOut.length} bytes)`);

// 3) .kapi project run against memfs + the in-memory cache.
console.log("klz-wasm-smoke: .kapi project run");
mem.vol.mkdirp("/proj");
mem.vol.mkdirp("/proj/.kapi");
mem.vol.writeFile("/proj/app.json", enc.encode('{"g":"Hello"}'));
mem.vol.writeFile(
  "/proj/demo.kapi",
  enc.encode("version: \"v1\"\nname: d\ndefaults:\n  source_locale: en\n  target_locales: [qps]\nflows:\n  pseudo:\n    steps:\n      - tool: pseudo-translate\n"),
);
ok("project run", await run("run", "pseudo", "-p", "/proj/demo.kapi", "-i", "/proj/app.json", "-o", "/proj/out.json", "--target-lang", "qps"));
const projOut = dec.decode(mem.vol.readFile("/proj/out.json"));
if (!/[-￿]/.test(projOut)) { console.error("FAIL: project run output not translated: " + projOut); process.exit(1); }

console.log("klz-wasm-smoke: OK (.klz + .kapi run in wasm; JSON + Office; dirty/pack)");
process.exit(0);
