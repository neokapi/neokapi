// KapiRuntime — the singleton that owns the one booted kapi WASM instance.
//
// The wasm module installs a single global function set (see globals.ts /
// abi.ts) and our memfs is a module-global volume, so there is exactly one
// live session at a time. That is fine for the embedding model: only one host
// surface (modal, lab, terminal) is ever driving the runtime. `setSinks`
// points the live stdout/stderr at whichever surface is active.
//
// Boot is lazy: nothing is fetched until `bootKapiRuntime()` is first called.
// Subsequent calls reuse the warm instance.

import type { ContentTree } from "@neokapi/contract-types";
import type { RawInspectResponse, RawPreviewResponse } from "./abi.ts";
import { createMemFS } from "./memfs.ts";
import type { MemFS, MemVolume } from "./memfs.ts";
import "./globals.ts";

export interface PreviewBlock {
  id: string;
  text: string;
}

export interface PreviewResult {
  ok: boolean;
  error?: string;
  format?: string;
  blocks?: PreviewBlock[];
  total?: number;
  bytes?: number;
}

/** Which read-only annotators `inspectAnnotated` runs (all default to true). */
export interface AnnotateOptions {
  term?: boolean;
  brand?: boolean;
  qa?: boolean;
  /** Run segmentation and surface sentence boundaries in the preview. */
  segment?: boolean;
  /** Segmentation engine when `segment` is set ("" = srx; "uax29" = ICU4X). */
  segmentEngine?: string;
  /** Content language (BCP-47, "" = "en"); locale-sensitive engines tailor to it. */
  segmentLocale?: string;
}

export interface InspectResult {
  ok: boolean;
  error?: string;
  format?: string;
  /**
   * Parsed ContentTree (the hierarchical content-model view), typed with the
   * generated projection shape from @neokapi/contract-types (AD-034 —
   * generated from the Go core/editor structs, so it cannot drift).
   * @neokapi/kapi-lab layers its own loose refinement on top.
   */
  tree?: ContentTree;
  bytes?: number;
}

export interface TraceRunResult {
  /** Process-style exit code from the underlying kapiRun. */
  code: number;
  /**
   * Parsed FlowTrace JSON, or null when the run produced no trace file (null is
   * already covered by unknown). Typed as unknown; @neokapi/kapi-lab casts it to
   * its FlowTrace type.
   */
  trace: unknown;
}

/**
 * A KBF spec operation routed to the canonical Go engine (core/kbf) via the
 * `kbf` wasm endpoint. The shape is op-specific; see the KBF docs Lab/Tests
 * pages for the per-op payloads (roundtrip, validateBlock, validateTarget,
 * resolveAnchor, validateAnnotation, renderHtml).
 */
export interface KbfRequest {
  op:
    | "roundtrip"
    | "validateBlock"
    | "validateTarget"
    | "resolveAnchor"
    | "validateAnnotation"
    | "renderHtml";
  [key: string]: unknown;
}

/** Generic KBF endpoint response; always carries `ok`. */
export interface KbfResponse {
  ok: boolean;
  error?: string;
  [key: string]: unknown;
}

/** One sentence produced by the segmentation engine. */
export interface SegmentPiece {
  text: string;
}

/** Result of the `labSegment` wasm endpoint. */
export interface SegmentResult {
  ok: boolean;
  error?: string;
  /** The engine that actually ran (e.g. "srx" when "" was requested). */
  engine?: string;
  segments?: SegmentPiece[];
}

export interface KapiRuntime {
  vol: MemVolume;
  run(argv: string[]): Promise<number>;
  preview(path: string): Promise<PreviewResult>;
  /** Inspect a file's content model, returning the parsed ContentTree. */
  inspect(path: string): Promise<InspectResult>;
  /**
   * Inspect a file like {@link inspect}, but run the engine's read-only
   * annotators (terminology, brand vocabulary, rule-based QA) first so the
   * parsed blocks carry stand-off overlays. `opts` toggles individual annotators
   * (term/brand/qa); all default to true. Wraps the `labInspectAnnotated` global.
   */
  inspectAnnotated(path: string, opts?: AnnotateOptions): Promise<InspectResult>;
  /**
   * Run a KBF spec operation against the canonical Go engine. Synchronous: the
   * wasm endpoint does pure in-memory work over the JSON payload (no fs), so it
   * returns the parsed response directly rather than a Promise.
   */
  kbf(req: KbfRequest): KbfResponse;
  /**
   * Segment raw text with a named engine ("" = default srx) and locale.
   * Synchronous: pure in-memory work (the "uax29"/ICU4X path makes one
   * re-entrant JS call), so it returns the result directly.
   */
  segment(text: string, engine: string, locale: string): SegmentResult;
  /** List the segmentation engines registered in this wasm build. */
  segmentEngines(): string[];
  /**
   * Run a command with flow tracing enabled and return the parsed FlowTrace.
   * Appends `--trace <tmp>` to argv, runs it, and reads the trace back from the
   * in-memory filesystem. The caller supplies the command plus its input and
   * output args, e.g. ["pseudo-translate", "/p/in.json", "-o", "/p/out.json"]
   * or ["run", "translate-qa", "-i", "/p/in.json", "-o", "/p/out.json"].
   */
  runWithTrace(argv: string[]): Promise<TraceRunResult>;
  cwd(): string;
  chdir(dir: string): void;
  /** Point the live stdout/stderr sinks at a destination (the active terminal). */
  setSinks(out: (s: string) => void, err: (s: string) => void): void;
}

// Monotonic counter for unique in-memfs trace paths, so a re-run never reads a
// stale trace from a prior call.
let traceSeq = 0;

// The one active session's output sinks. Only one terminal is ever live, so a
// single pair of refs suffices — no per-embed isolation needed (see #658).
let outSink: (s: string) => void = () => {};
let errSink: (s: string) => void = () => {};

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const existing = document.querySelector(`script[data-kapi-wasm-exec]`);
    if (existing) return resolve();
    const s = document.createElement("script");
    s.src = src;
    s.dataset.kapiWasmExec = "1";
    s.onload = () => resolve();
    s.onerror = () => reject(new Error(`failed to load ${src}`));
    document.head.appendChild(s);
  });
}

// ---------------------------------------------------------------------------
// Boot progress
// ---------------------------------------------------------------------------

/** Download progress for the engine boot, for hosts that render a bar. */
export interface BootProgress {
  /** Bytes received so far. */
  loaded: number;
  /** Total bytes (from Content-Length), or null when the server omits it. */
  total: number | null;
  /** True once the engine is up (terminal event). */
  done?: boolean;
}

const bootProgressListeners = new Set<(p: BootProgress) => void>();
let lastBootProgress: BootProgress | null = null;

function emitBootProgress(p: BootProgress) {
  lastBootProgress = p;
  for (const fn of bootProgressListeners) fn(p);
}

/**
 * Subscribe to engine-boot download progress. The last event is replayed on
 * subscribe (so a late subscriber catches up); returns an unsubscribe.
 */
export function onBootProgress(fn: (p: BootProgress) => void): () => void {
  bootProgressListeners.add(fn);
  if (lastBootProgress) fn(lastBootProgress);
  return () => bootProgressListeners.delete(fn);
}

/** Wrap a response body with a byte-counting stage that reports progress. */
function countingStream(resp: Response): ReadableStream<Uint8Array<ArrayBuffer>> {
  const total = Number(resp.headers.get("content-length")) || null;
  let loaded = 0;
  emitBootProgress({ loaded: 0, total });
  const counter = new TransformStream<Uint8Array<ArrayBuffer>, Uint8Array<ArrayBuffer>>({
    transform(chunk, controller) {
      loaded += chunk.byteLength;
      emitBootProgress({ loaded, total });
      controller.enqueue(chunk);
    },
  });
  return resp.body!.pipeThrough(counter);
}

// Fetch the wasm bytes. Prefer the precompressed `.wasm.gz` (the binary is
// ~64 MB raw, ~13 MB gzipped) and inflate it in the browser via
// DecompressionStream — this is portable and does not depend on the host
// setting Content-Encoding (GitHub Pages / Docusaurus static serving do not).
// Falls back to the raw `.wasm` if the compressed asset or the API is missing.
// Both paths report download progress through onBootProgress.
async function fetchWasmBytes(wasmUrl: string): Promise<ArrayBuffer | Response> {
  if (typeof DecompressionStream !== "undefined") {
    try {
      const gzResp = await fetch(`${wasmUrl}.gz`);
      if (gzResp.ok && gzResp.body) {
        const stream = countingStream(gzResp).pipeThrough(new DecompressionStream("gzip"));
        return await new Response(stream).arrayBuffer();
      }
    } catch {
      /* fall through to the raw asset */
    }
  }
  // Buffer the raw asset through the same counter so progress still reports;
  // instantiate() accepts the ArrayBuffer.
  const resp = await fetch(wasmUrl);
  if (resp.ok && resp.body) {
    return await new Response(countingStream(resp)).arrayBuffer();
  }
  return resp;
}

async function instantiate(
  source: ArrayBuffer | Response,
  importObject: WebAssembly.Imports,
): Promise<WebAssembly.Instance> {
  if (source instanceof Response) {
    try {
      const r = await WebAssembly.instantiateStreaming(source.clone(), importObject);
      return r.instance;
    } catch {
      const buf = await source.arrayBuffer();
      const r = await WebAssembly.instantiate(buf, importObject);
      return r.instance;
    }
  }
  const r = await WebAssembly.instantiate(source, importObject);
  return r.instance;
}

// The Go class wasm_exec.js defines, and the fs/process host shims that Go's
// js/wasm runtime reads from globalThis (see syscall/fs_js.go). We install our
// memfs-backed shims before wasm_exec.js runs. Deliberately NOT declared
// ambiently: global `fs`/`process` declarations would collide with
// @types/node in consumer programs.
interface GoInstance {
  importObject: WebAssembly.Imports;
  env: Record<string, string>;
  run(instance: WebAssembly.Instance): Promise<void>;
}
interface WasmExecHost {
  Go?: new () => GoInstance;
  fs?: unknown;
  process?: { env?: Record<string, string> };
}

/** Throw a clear error when a required engine global is missing. */
function requireFn<T>(fn: T | undefined, name: string): T {
  if (typeof fn !== "function") {
    throw new Error(`engine global ${name} is not registered (wasm not booted?)`);
  }
  return fn;
}

/**
 * Wire the {@link KapiRuntime} facade over the engine globals and a booted
 * instance's memfs. Split from boot so the facade is unit-testable against a
 * mocked global surface; hosts should call {@link bootKapiRuntime} instead.
 */
export function makeRuntime(mem: MemFS): KapiRuntime {
  const dec = new TextDecoder();
  const run = requireFn(globalThis.kapiRun, "kapiRun");
  const preview = requireFn(globalThis.kapiPreview, "kapiPreview");
  const inspect = requireFn(globalThis.labInspect, "labInspect");

  const parseInspect = (res: RawInspectResponse): InspectResult => {
    if (!res || !res.ok) return { ok: false, error: res?.error ?? "inspect failed" };
    try {
      return {
        ok: true,
        format: res.format,
        tree: res.json ? (JSON.parse(res.json) as ContentTree) : undefined,
        bytes: res.bytes,
      };
    } catch (e) {
      return { ok: false, error: `parse content tree: ${(e as Error).message}` };
    }
  };

  return {
    vol: mem.vol,
    run: (argv: string[]) => run(argv),
    preview: (path: string): Promise<PreviewResult> =>
      preview(path).then((res: RawPreviewResponse) => res),
    inspect: async (path: string): Promise<InspectResult> => parseInspect(await inspect(path)),
    inspectAnnotated: async (path: string, opts?: AnnotateOptions): Promise<InspectResult> => {
      const fn = globalThis.labInspectAnnotated;
      if (typeof fn !== "function") {
        return { ok: false, error: "labInspectAnnotated unavailable in this wasm build" };
      }
      // The wasm endpoint accepts an optional JSON options string; omit it to
      // let the engine default all annotators on.
      const res = await (opts ? fn(path, JSON.stringify(opts)) : fn(path));
      return parseInspect(res);
    },
    kbf: (req: KbfRequest): KbfResponse => {
      const fn = globalThis.kbf;
      if (typeof fn !== "function") {
        return { ok: false, error: "kbf endpoint unavailable in this wasm build" };
      }
      try {
        return JSON.parse(fn(JSON.stringify(req))) as KbfResponse;
      } catch (e) {
        return { ok: false, error: `kbf request failed: ${(e as Error).message}` };
      }
    },
    segment: (text: string, engine: string, locale: string): SegmentResult => {
      const fn = globalThis.labSegment;
      if (typeof fn !== "function") {
        return { ok: false, error: "segment endpoint unavailable in this wasm build" };
      }
      try {
        // labSegment returns a converted JS object directly (not a JSON
        // string): { ok, engine, segments: [{text}] } or { ok:false, error }.
        const res = fn(text, engine, locale);
        if (!res || !res.ok) return { ok: false, error: res?.error ?? "segment failed" };
        const segs = (res.segments ?? []).map((s) => ({ text: s.text }));
        return { ok: true, engine: res.engine, segments: segs };
      } catch (e) {
        return { ok: false, error: `segment request failed: ${(e as Error).message}` };
      }
    },
    segmentEngines: (): string[] => {
      const fn = globalThis.labSegmentEngines;
      if (typeof fn !== "function") return [];
      try {
        return Array.from(fn() ?? []);
      } catch {
        return [];
      }
    },
    runWithTrace: async (argv: string[]): Promise<TraceRunResult> => {
      const tracePath = `/.lab/trace-${++traceSeq}.json`;
      const code = await run([...argv, "--trace", tracePath]);
      try {
        // kapiRun resolves only after the command (incl. the synchronous
        // trace write) completes, so the file is present to read here.
        return { code, trace: JSON.parse(dec.decode(mem.vol.readFile(tracePath))) };
      } catch {
        return { code, trace: null };
      }
    },
    cwd: () => mem.vol.cwd(),
    chdir: (dir: string) => mem.process.chdir(dir),
    setSinks: (out, err) => {
      outSink = out;
      errSink = err;
    },
  };
}

let booting: Promise<KapiRuntime> | null = null;

/**
 * Boot the kapi CLI wasm once and return the shared runtime. Idempotent: the
 * first call starts the boot; later calls await the same promise.
 *
 * `wasmExecUrl` is Go's wasm_exec.js (shipped next to the engine asset);
 * `wasmUrl` is the engine binary — a precompressed sibling `<wasmUrl>.gz` is
 * preferred when the platform can inflate it (see fetchWasmBytes).
 */
export function bootKapiRuntime(wasmExecUrl: string, wasmUrl: string): Promise<KapiRuntime> {
  if (booting) return booting;
  booting = (async () => {
    const dec = new TextDecoder();
    const mem = createMemFS({
      onStdout: (c) => outSink(dec.decode(c)),
      onStderr: (c) => errSink(dec.decode(c)),
    });

    const g = globalThis as WasmExecHost;
    // Install our fs/process BEFORE wasm_exec.js runs, so it doesn't install
    // its own enosys defaults. Preserve any existing process.env.
    g.fs = mem.fs;
    const existingProc = g.process ?? {};
    g.process = Object.assign({}, existingProc, mem.process, { env: existingProc.env || {} });

    await loadScript(wasmExecUrl);
    const Go = g.Go;
    if (!Go) throw new Error("wasm_exec.js did not define Go");

    const go = new Go();
    // stdout isn't a TTY in the browser, so force color for JSON output
    // (--json / --jq) — the terminal renders ANSI just fine.
    go.env = { CLICOLOR_FORCE: "1" };
    const ready = new Promise<void>((res) => {
      globalThis.__kapiCliReady = res;
    });

    const source = await fetchWasmBytes(wasmUrl);
    const instance = await instantiate(source, go.importObject);
    void go.run(instance); // blocks forever (select{}); not awaited
    await ready;

    emitBootProgress({
      loaded: lastBootProgress?.loaded ?? 0,
      total: lastBootProgress?.total ?? null,
      done: true,
    });

    return makeRuntime(mem);
  })();
  return booting;
}

/** True once boot has been started (used to skip a loading flash on re-open). */
export function isBooted(): boolean {
  return booting !== null;
}
