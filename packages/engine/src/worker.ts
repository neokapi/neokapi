// The engine's dedicated Worker: Go, SQLite and the engine's file system run
// here, off the page's thread, where SQLite's opfs-sahpool VFS can keep the
// workspace in the origin private file system (storage.ts). The page talks to
// it through the facade (runtime.ts) over the messages in protocol.ts.
//
// Every call the page sends runs against the engine's globals here. After a
// call, the Worker sends the page the file system changes it made, then the
// call's value, and keeps the changes in the pool.

import type { EngineABI } from "./abi.ts";
import { createMemFS } from "./memfs.ts";
import type { MemFS } from "./memfs.ts";
import { fetchWasmBytes, instantiate } from "./fetch.ts";
import type { WasmExecHost } from "./fetch.ts";
import { installSQLiteBridge, loadSQLite } from "./sqlite.ts";
import {
  ENGINE_PREFIX,
  VolumeStore,
  applyChanges,
  changesOf,
  keepCapacity,
  openPool,
} from "./storage.ts";
import type { StorageInfo, VolOp } from "./storage.ts";
import { PAGE_FUNCTIONS } from "./protocol.ts";
import { invokeEngine, localCalls } from "./local.ts";
import type { FromWorker, ToWorker, WorkerBoot } from "./protocol.ts";

type Scope = {
  postMessage(msg: FromWorker, transfer?: Transferable[]): void;
  onmessage: ((ev: MessageEvent<ToWorker>) => void) | null;
  importScripts?: (...urls: string[]) => void;
};
const scope = globalThis as unknown as Scope;
const g = globalThis as unknown as Record<string, unknown> & WasmExecHost;

const post = (msg: FromWorker, transfer: Transferable[] = []) => scope.postMessage(msg, transfer);

let mem: MemFS | null = null;
let local: ReturnType<typeof localCalls> | null = null;
let store: VolumeStore | null = null;
let pool: Awaited<ReturnType<typeof openPool>> | null = null;
let quiet = false;
// Set once another tab takes the workspace over: the pool is no longer ours.
let lost = false;

/**
 * Another tab took the workspace over. Keep the changes not yet kept, stop
 * using the pool and tell the page, which stops this Worker: only then are
 * the pool's files free for the tab that took it.
 */
function onLost(): void {
  if (lost) return;
  flush();
  lost = true;
  store = null;
  post({ t: "lost" });
}
// Changes the page has not seen, and changes the pool has not kept.
const forPage = new Set<string>();
const forPool = new Set<string>();
let flushTimer: ReturnType<typeof setTimeout> | null = null;

/** Send the page the changes it has not seen, and keep every change. */
function flush(): void {
  if (flushTimer) {
    clearTimeout(flushTimer);
    flushTimer = null;
  }
  if (!mem) return;
  if (forPage.size) {
    const ops = changesOf(mem, forPage);
    forPage.clear();
    post({ t: "vol", ops });
  }
  if (forPool.size) {
    const ops = changesOf(mem, forPool);
    forPool.clear();
    try {
      store?.save(ops);
    } catch (e) {
      post({ t: "err", text: `kapi: could not keep files: ${(e as Error).message}\n` });
    }
  }
}

function changed(p: string): void {
  if (quiet) return;
  forPage.add(p);
  forPool.add(p);
  // A change outside a call (a goroutine finishing late) still reaches the
  // page and the pool.
  flushTimer ??= setTimeout(flush, 100);
}

// ── Asking the page ─────────────────────────────────────────────────────────

let nextAsk = 1;
const asks = new Map<number, { resolve: (v: unknown) => void; reject: (e: Error) => void }>();

function askPage(name: string, args: unknown[]): Promise<unknown> {
  const id = nextAsk++;
  return new Promise((resolve, reject) => {
    asks.set(id, { resolve, reject });
    post({ t: "bridge", id, name, args });
  });
}

/** Intl.Segmenter is in the Worker too, so the engine calls it here. */
function intlSentenceBreaks(text: string, locale: string): number[] {
  const seg = new Intl.Segmenter(locale || "en", { granularity: "sentence" });
  const u16: number[] = [];
  for (const part of seg.segment(text)) {
    if (part.index > 0 && part.index < text.length) u16.push(part.index);
  }
  // UTF-16 offsets to code-point offsets: Go anchors spans by rune.
  const out: number[] = [];
  let i = 0;
  let cp = 0;
  for (const b of u16) {
    while (i < b) {
      i += (text.codePointAt(i) as number) > 0xffff ? 2 : 1;
      cp++;
    }
    out.push(cp);
  }
  return out;
}

/** Mirror the page's capabilities: a stand-in for each one the page has. */
function syncBridges(present: string[]): void {
  const has = new Set(present);
  for (const name of PAGE_FUNCTIONS) {
    if (has.has(name)) g[name] ??= (...args: unknown[]) => askPage(name, args);
    else delete g[name];
  }
  if (has.has("__kapiPdfium")) {
    g.__kapiPdfium ??= {
      get ready() {
        return askPage("__kapiPdfium.ready", []);
      },
      extract: (bytes: Uint8Array) => askPage("__kapiPdfium.extract", [bytes]),
    };
  } else delete g.__kapiPdfium;
  if (has.has("kapiIntlSentenceBreaks") && typeof Intl.Segmenter === "function") {
    g.kapiIntlSentenceBreaks ??= intlSentenceBreaks;
  } else delete g.kapiIntlSentenceBreaks;
  // The browser MT provider checks for the platform API beside its bridge.
  if (has.has("Translator")) g.Translator ??= {};
  else delete g.Translator;
}

// ── Calls ───────────────────────────────────────────────────────────────────

// An export's bytes move to the page rather than being copied.
function transferables(v: unknown): Transferable[] {
  if (v && typeof v === "object" && (v as { data?: unknown }).data instanceof Uint8Array) {
    return [(v as { data: Uint8Array }).data.buffer];
  }
  return [];
}

let calls = Promise.resolve();

async function call(id: number, fn: string, args: unknown[], bridges: string[]): Promise<void> {
  syncBridges(bridges);
  let reply: FromWorker;
  try {
    if (lost) throw new Error("another tab took over the workspace");
    const value = await invokeEngine(local!, fn, args);
    reply = { t: "result", id, ok: true, value, cwd: mem!.process.cwd() };
  } catch (e) {
    reply = {
      t: "result",
      id,
      ok: false,
      error: e instanceof Error ? e.message : String(e),
      cwd: mem!.process.cwd(),
    };
  }
  flush();
  post(reply, reply.ok ? transferables(reply.value) : []);
  if (!lost && store && pool && "pool" in pool) {
    // Spare slots for the next call's new databases; between calls, since
    // growing the pool waits on the file system.
    calls = calls
      .then(() => keepCapacity((pool as { pool: Parameters<typeof keepCapacity>[0] }).pool))
      .catch(() => {});
  }
}

// ── Boot ────────────────────────────────────────────────────────────────────

async function loadWasmExec(url: string): Promise<void> {
  try {
    // wasm_exec.js defines globalThis.Go in strict code, so it runs as a
    // module as it does as a script.
    await import(/* webpackIgnore: true */ /* @vite-ignore */ url);
  } catch (e) {
    // A classic Worker can still run it as a script.
    if (typeof scope.importScripts !== "function") throw e;
    scope.importScripts(url);
  }
}

async function boot(b: WorkerBoot): Promise<void> {
  const sqliteReady = loadSQLite({ wasmUrl: b.sqliteWasmUrl });
  sqliteReady.catch(() => {});
  const wasmReady = fetchWasmBytes(b.wasmUrl, (progress) => post({ t: "progress", progress }));
  wasmReady.catch(() => {});

  const sqlite3 = await sqliteReady;
  let info: StorageInfo = { kind: "memory", reason: "disabled" };
  if (b.persist !== null) {
    pool = await openPool(sqlite3, b.persist, { whenHeld: b.whenHeld, onLost });
    info = "pool" in pool ? pool.info : pool;
  }

  const dec = new TextDecoder();
  mem = createMemFS({
    onStdout: (c) => post({ t: "out", text: dec.decode(c) }),
    onStderr: (c) => post({ t: "err", text: dec.decode(c) }),
    onChange: changed,
  });
  local = localCalls(mem);
  if (pool && "pool" in pool) {
    try {
      store = new VolumeStore(pool.pool);
      quiet = true;
      store.restore(mem);
    } finally {
      quiet = false;
    }
  }
  installSQLiteBridge(sqlite3, {
    pool: pool && "pool" in pool ? pool.pool : undefined,
    hidden: (name) => name.startsWith(ENGINE_PREFIX),
  });

  g.fs = mem.fs;
  g.process = Object.assign({}, g.process ?? {}, mem.process, { env: g.process?.env ?? {} });
  await loadWasmExec(b.wasmExecUrl);
  const Go = g.Go;
  if (!Go) throw new Error("wasm_exec.js did not define Go");
  const go = new Go();
  go.env = { CLICOLOR_FORCE: "1" };
  const ready = new Promise<void>((res) => {
    g.__kapiCliReady = res;
  });
  const instance = await instantiate(await wasmReady, go.importObject);
  await Promise.race([
    ready,
    go.run(instance).then(() => {
      throw new Error("kapi engine exited before becoming ready");
    }),
  ]);

  // The page starts from the whole file system; later it gets the changes.
  forPage.clear();
  const ops: VolOp[] = changesOf(mem, ["/"]);
  flush();
  const abiFn = g.kapiEngineABI as (() => EngineABI) | undefined;
  const raw = typeof abiFn === "function" ? abiFn() : null;
  const abi = raw
    ? { abi: raw.abi, version: raw.version, functions: Array.from(raw.functions ?? []) }
    : null;
  post({ t: "ready", storage: info, ops, cwd: mem.process.cwd(), abi });
}

scope.onmessage = (ev) => {
  const msg = ev.data;
  switch (msg.t) {
    case "boot":
      boot(msg.boot).catch((e: unknown) =>
        post({ t: "boot-error", error: e instanceof Error ? e.message : String(e) }),
      );
      break;
    case "call":
      void call(msg.id, msg.fn, msg.args, msg.bridges);
      break;
    case "vol":
      // The page already holds these: keep them, and do not send them back.
      if (!mem) break;
      quiet = true;
      try {
        applyChanges(mem, msg.ops);
      } finally {
        quiet = false;
      }
      try {
        store?.save(msg.ops);
      } catch (e) {
        post({ t: "err", text: `kapi: could not keep files: ${(e as Error).message}\n` });
      }
      break;
    case "chdir":
      try {
        mem?.process.chdir(msg.dir);
      } catch {
        /* the page checked it against its mirror */
      }
      break;
    case "bridge-result": {
      const ask = asks.get(msg.id);
      asks.delete(msg.id);
      if (!ask) break;
      if (msg.ok) ask.resolve(msg.value);
      else ask.reject(new Error(msg.error ?? "the page's bridge failed"));
      break;
    }
  }
};
