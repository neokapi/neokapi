// Runs a Go js/wasm binary in Node with the browser engine's database driver
// underneath: it loads @sqlite.org/sqlite-wasm, installs the SQLite bridge
// (packages/engine/src/sqlite.ts) and then starts Go in the same thread, the
// way packages/engine boots the engine on a page. It is Go's
// wasm_exec_node.js plus that one step, and `make test-wasm-stores` hands it to
// `go test -exec` through go_js_wasm_exec beside it.
//
// The file system is Node's own, so a test's temporary directories are real;
// its databases are not, since they live in SQLite's memory as they do in the
// browser.
//
// Usage: node --experimental-strip-types run.ts <binary.wasm> [args...]
import { readFileSync } from "node:fs";
import * as fs from "node:fs";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import * as path from "node:path";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { runInThisContext } from "node:vm";
import { installSQLiteBridge, loadSQLite } from "../../packages/engine/src/sqlite.ts";

if (process.argv.length < 3) {
  console.error("usage: run.ts <binary.wasm> [args...]");
  process.exit(1);
}

installSQLiteBridge(await loadSQLite());

const g = globalThis as Record<string, unknown>;
g.require = createRequire(import.meta.url);
g.fs = fs;
g.path = path;

const goroot = process.env.GOROOT || execFileSync("go", ["env", "GOROOT"], { encoding: "utf8" }).trim();
runInThisContext(readFileSync(join(goroot, "lib/wasm/wasm_exec.js"), "utf8"));

// Go's js/wasm runtime refuses an argv plus environment above 12 KiB, and a
// developer's shell easily carries more. The binary gets what a test reads.
const keep = /^(HOME|USER|LANG|LC_\w+|TZ|PATH|TMPDIR|CI|GITHUB_ACTIONS|GO\w*|KAPI_\w+|XDG_\w+|NEOKAPI_\w+)$/;
const env: Record<string, string> = { TMPDIR: tmpdir() };
for (const [k, v] of Object.entries(process.env)) {
  if (v !== undefined && keep.test(k)) env[k] = v;
}

interface GoRuntime {
  argv: string[];
  env: Record<string, string>;
  exit: (code: number) => void;
  exited: boolean;
  importObject: WebAssembly.Imports;
  _pendingEvent: unknown;
  _resume(): void;
  run(instance: WebAssembly.Instance): Promise<void>;
}
const Go = g.Go as new () => GoRuntime;
const go = new Go();
go.argv = process.argv.slice(2);
go.env = env;
go.exit = process.exit;

const { instance } = await WebAssembly.instantiate(readFileSync(process.argv[2]), go.importObject);
process.on("exit", (code) => {
  // Node exits when no event is pending; a Go program still running then is
  // deadlocked, so make it print its goroutines.
  if (code === 0 && !go.exited) {
    go._pendingEvent = { id: 0 };
    go._resume();
  }
});
await go.run(instance);
