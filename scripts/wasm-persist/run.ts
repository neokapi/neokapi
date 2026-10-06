// wasm-persist: prove the browser engine keeps its workspace. Boots the real
// kapi-cli.wasm through @neokapi/engine in headless Chromium, served without
// COOP/COEP headers (GitHub Pages cannot set them), and checks that:
//
//   - the engine runs in its Worker and keeps the workspace in the origin
//     private file system (storage.kind "opfs");
//   - files and a recorded edit survive a reload and a browser restart;
//   - a second tab finds the workspace held and runs in memory, and says so;
//   - an export of the workspace imports into a fresh browser profile, files
//     and the project's context both.
//
// Usage: node --experimental-strip-types scripts/wasm-persist/run.ts [wasmDir]
// (make wasm-persist-smoke builds the engine first).
import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { extname, join, resolve } from "node:path";
import { build } from "esbuild";
import { chromium } from "playwright";
import type { BrowserContext, Page } from "playwright";

const wasmDir = resolve(process.argv[2] ?? "web/static/wasm");
const scratch = mkdtempSync(join(tmpdir(), "kapi-wasm-persist-"));
const out = join(scratch, "site");

// The page and the Worker, bundled. The facade starts its Worker from
// new URL("./worker.ts", import.meta.url), so the Worker's bundle is served
// under that name.
await build({
  entryPoints: { page: "scripts/wasm-persist/page.ts" },
  bundle: true,
  format: "esm",
  platform: "browser",
  outdir: out,
  logLevel: "warning",
});
await build({
  entryPoints: ["packages/engine/src/worker.ts"],
  bundle: true,
  format: "esm",
  platform: "browser",
  outfile: join(out, "worker.ts"),
  logLevel: "warning",
});
writeFileSync(
  join(out, "index.html"),
  '<!doctype html><meta charset="utf-8"><title>kapi</title><script type="module" src="/page.js"></script>',
);

const types: Record<string, string> = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".ts": "text/javascript",
  ".wasm": "application/wasm",
  ".gz": "application/gzip",
};
// A plain static server: no COOP, no COEP.
const server = createServer((req, res) => {
  const url = new URL(req.url ?? "/", "http://localhost");
  const file = url.pathname.startsWith("/wasm/")
    ? join(wasmDir, url.pathname.slice("/wasm/".length))
    : join(out, url.pathname === "/" ? "index.html" : url.pathname);
  try {
    const body = readFileSync(file);
    res.writeHead(200, { "content-type": types[extname(file)] ?? "application/octet-stream" });
    res.end(body);
  } catch {
    res.writeHead(404);
    res.end();
  }
});
await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
const origin = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;

let failures = 0;
const ok = (label: string, cond: boolean, detail = "") => {
  console.log(`${cond ? "ok  " : "FAIL"} ${label}${detail ? `  ${detail}` : ""}`);
  if (!cond) failures++;
};

const RECIPE =
  "version: v1\nname: site\ndefaults:\n  source_language: en\n  target_languages: [fr]\n" +
  'collections:\n  - name: docs\n    content:\n      - path: "docs/*.json"\n';

async function open(ctx: BrowserContext): Promise<Page> {
  const page = await ctx.newPage();
  page.on("pageerror", (e) => console.log(`  page error: ${e.message}`));
  await page.goto(origin);
  await page.waitForFunction(() => typeof window.boot === "function");
  return page;
}

async function boot(page: Page) {
  return page.evaluate(async () => {
    const rt = await window.boot();
    return { storage: rt.storage, cwd: rt.cwd() };
  });
}

/** What the page's workspace holds: the edited file and the project's log. */
async function holdings(page: Page) {
  return page.evaluate(async () => {
    const rt = window.rt!;
    const dec = new TextDecoder();
    const file = rt.vol.exists("/site/docs/app.json")
      ? dec.decode(rt.vol.readFile("/site/docs/app.json"))
      : null;
    let text: string | null = null;
    try {
      const read = await rt.read({ doc: "docs/app.json" }, { project: "/site" });
      text = read.blocks.map((b) => b.text).join("|");
    } catch (e) {
      text = `refused: ${(e as Error).message}`;
    }
    const exported = await rt.exportWorkspace();
    return {
      file,
      text,
      operations: exported.projects.find((p) => p.root === "/site")?.operations ?? 0,
    };
  });
}

const profile = join(scratch, "profile");
try {
  // ── A first visit: write a project, edit it, record a term ────────────────
  let ctx = await chromium.launchPersistentContext(profile, { headless: true });
  let page = await open(ctx);
  const first = await boot(page);
  ok("the engine keeps its workspace in the origin private file system", first.storage.kind === "opfs", JSON.stringify(first.storage));
  const applied = await page.evaluate(async (recipe) => {
    const rt = window.rt!;
    const enc = new TextEncoder();
    rt.vol.mkdirp("/site/docs");
    rt.vol.writeFile("/site/kapi.yaml", enc.encode(recipe));
    rt.vol.writeFile("/site/docs/app.json", enc.encode('{"greeting":"Hello"}'));
    const read = await rt.read({ doc: "docs/app.json" }, { project: "/site" });
    const block = read.blocks[0];
    const result = await rt.apply(
      { ops: [{ op: "set_content", at: block.ref, if_match: block.rev, text: "Hello again" }] },
      { project: "/site" },
    );
    const term = await rt.apply(
      { ops: [{ op: "term", action: "upsert", term: "handbook", locale: "en" }] } as never,
      { project: "/site" },
    );
    return { edit: result.status, term: term.status };
  }, RECIPE);
  ok("an edit and a term apply", applied.edit === "applied" && applied.term === "applied", JSON.stringify(applied));
  const before = await holdings(page);
  ok("the edit landed in the file", before.file?.includes("Hello again") ?? false, before.file ?? "");
  ok("the project's log holds the operations", before.operations >= 2, `operations=${before.operations}`);

  // ── A second tab while the first holds the workspace ──────────────────────
  const second = await open(ctx);
  const other = await boot(second);
  ok(
    "a second tab runs in memory and says another tab holds the workspace",
    other.storage.kind === "memory" && other.storage.reason === "another-tab",
    JSON.stringify(other.storage),
  );
  await second.close();

  // ── A reload ──────────────────────────────────────────────────────────────
  await page.reload();
  await page.waitForFunction(() => typeof window.boot === "function");
  const reloaded = await boot(page);
  ok("after a reload the workspace is kept", reloaded.storage.kind === "opfs", JSON.stringify(reloaded.storage));
  const afterReload = await holdings(page);
  ok("after a reload the edited file is there", afterReload.file === before.file, afterReload.file ?? "missing");
  ok("after a reload a read shows the edit", afterReload.text === "Hello again", afterReload.text ?? "");
  ok("after a reload the log holds every operation", afterReload.operations === before.operations, `operations=${afterReload.operations}`);

  // ── A browser restart ─────────────────────────────────────────────────────
  await ctx.close();
  ctx = await chromium.launchPersistentContext(profile, { headless: true });
  page = await open(ctx);
  const restarted = await boot(page);
  ok("after a restart the workspace is kept", restarted.storage.kind === "opfs", JSON.stringify(restarted.storage));
  const afterRestart = await holdings(page);
  ok("after a restart the edited file is there", afterRestart.file === before.file, afterRestart.file ?? "missing");
  ok("after a restart the log holds every operation", afterRestart.operations === before.operations, `operations=${afterRestart.operations}`);
  const term = await page.evaluate(async () => {
    let out = "";
    window.rt!.setSinks((s) => (out += s), (s) => (out += s));
    window.rt!.chdir("/site");
    const code = await window.rt!.run(["terms", "lookup", "handbook"]);
    return { code, out };
  });
  ok("after a restart the term is in the project's terms", term.code === 0 && term.out.includes("handbook"), term.out.slice(0, 200));

  // ── Export, then import into a fresh profile ──────────────────────────────
  const exported = await page.evaluate(async () => {
    const e = await window.rt!.exportWorkspace();
    return { bytes: Array.from(e.data), files: e.files, projects: e.projects, skipped: e.skipped };
  });
  ok("the export carries the files and the project", exported.files >= 2 && exported.projects.length >= 1, JSON.stringify({ files: exported.files, projects: exported.projects, skipped: exported.skipped }));
  await ctx.close();

  ctx = await chromium.launchPersistentContext(join(scratch, "fresh"), { headless: true });
  page = await open(ctx);
  await boot(page);
  const empty = await page.evaluate(() => window.rt!.vol.exists("/site"));
  ok("a fresh profile starts empty", !empty);
  const imported = await page.evaluate(async (bytes) => window.rt!.importWorkspace(new Uint8Array(bytes)), exported.bytes);
  ok(
    "the import writes the files and merges the project's context",
    imported.files === exported.files && imported.projects.some((p) => p.root === "/site" && p.merged >= 2),
    JSON.stringify(imported),
  );
  const afterImport = await holdings(page);
  ok("after the import a read shows the edit", afterImport.text === "Hello again", afterImport.text ?? "");
  ok("after the import the log holds every operation", afterImport.operations === before.operations, `operations=${afterImport.operations}`);
  await ctx.close();
} finally {
  server.close();
  rmSync(scratch, { recursive: true, force: true });
}

if (failures) {
  console.error(`wasm-persist: ${failures} check(s) failed`);
  process.exit(1);
}
console.log("wasm-persist: the workspace is kept across reloads, restarts and an export");
