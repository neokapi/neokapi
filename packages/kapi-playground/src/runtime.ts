// KapiRuntime — the singleton that owns the one booted kapi WASM instance.
//
// The wasm module installs a single global `kapiRun`/`kapiPreview` and our
// memfs is a module-global volume, so there is exactly one live session at a
// time. That is fine for the modal model: only one KapiModal is ever open and
// driving the runtime. This module generalizes the old module-global
// `setSinks` into a runtime object that the active terminal points at.
//
// Boot is lazy: nothing is fetched until `boot()` is first called (i.e. when
// the modal first opens). Subsequent opens reuse the warm instance.

import { createMemFS } from "./memfs";
import type { MemVolume } from "./memfs";

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
}

export interface InspectResult {
  ok: boolean;
  error?: string;
  format?: string;
  /**
   * Parsed ContentTree (the hierarchical content-model view). Typed as unknown
   * here so the kit stays lab-agnostic; @neokapi/kapi-lab casts it to its
   * ContentTree type.
   */
  tree?: unknown;
  bytes?: number;
}

export interface TraceRunResult {
  /** Process-style exit code from the underlying kapiRun. */
  code: number;
  /**
   * Parsed FlowTrace JSON, or null when the run produced no trace file.
   * Typed as unknown; @neokapi/kapi-lab casts it to its FlowTrace type.
   */
  trace: unknown | null;
}

/**
 * A KLF spec operation routed to the canonical Go engine (core/klf) via the
 * `klf` wasm endpoint. The shape is op-specific; see the KLF docs Lab/Tests
 * pages for the per-op payloads (roundtrip, validateBlock, validateTarget,
 * resolveAnchor, renderHtml).
 */
export interface KlfRequest {
  op: "roundtrip" | "validateBlock" | "validateTarget" | "resolveAnchor" | "renderHtml";
  [key: string]: unknown;
}

/** Generic KLF endpoint response; always carries `ok`. */
export interface KlfResponse {
  ok: boolean;
  error?: string;
  [key: string]: unknown;
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
   * Run a KLF spec operation against the canonical Go engine. Synchronous: the
   * wasm endpoint does pure in-memory work over the JSON payload (no fs), so it
   * returns the parsed response directly rather than a Promise.
   */
  klf(req: KlfRequest): KlfResponse;
  /**
   * Run a command with flow tracing enabled and return the parsed FlowTrace.
   * Appends `--trace <tmp>` to argv, runs it, and reads the trace back from the
   * in-memory filesystem. The caller supplies the command plus its input and
   * output args, e.g. ["pseudo-translate", "/p/in.json", "-o", "/p/out.json"]
   * or ["run", "ai-translate-qa", "-i", "/p/in.json", "-o", "/p/out.json"].
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

// Fetch the wasm bytes. Prefer the precompressed `.wasm.gz` (the binary is
// ~64 MB raw, ~13 MB gzipped) and inflate it in the browser via
// DecompressionStream — this is portable and does not depend on the host
// setting Content-Encoding (GitHub Pages / Docusaurus static serving do not).
// Falls back to the raw `.wasm` if the compressed asset or the API is missing.
async function fetchWasmBytes(wasmUrl: string): Promise<ArrayBuffer | Response> {
  const canInflate = typeof (globalThis as any).DecompressionStream !== "undefined";
  if (canInflate) {
    try {
      const gzResp = await fetch(`${wasmUrl}.gz`);
      if (gzResp.ok && gzResp.body) {
        const stream = gzResp.body.pipeThrough(new (globalThis as any).DecompressionStream("gzip"));
        return await new Response(stream).arrayBuffer();
      }
    } catch {
      /* fall through to the raw asset */
    }
  }
  // Return the Response so the caller can use instantiateStreaming when possible.
  return fetch(wasmUrl);
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

let booting: Promise<KapiRuntime> | null = null;

/**
 * Boot the kapi CLI wasm once and return the shared runtime. Idempotent: the
 * first call starts the boot; later calls await the same promise.
 */
export function bootKapiRuntime(wasmExecUrl: string, wasmUrl: string): Promise<KapiRuntime> {
  if (booting) return booting;
  booting = (async () => {
    const dec = new TextDecoder();
    const mem = createMemFS({
      onStdout: (c) => outSink(dec.decode(c)),
      onStderr: (c) => errSink(dec.decode(c)),
    });

    const g = globalThis as any;
    // Install our fs/process BEFORE wasm_exec.js runs, so it doesn't install
    // its own enosys defaults. Preserve any existing process.env.
    g.fs = mem.fs;
    const existingProc = g.process || {};
    g.process = Object.assign({}, existingProc, mem.process, { env: existingProc.env || {} });

    await loadScript(wasmExecUrl);
    const Go = g.Go;
    if (!Go) throw new Error("wasm_exec.js did not define Go");

    const go = new Go();
    // stdout isn't a TTY in the browser, so force color for JSON output
    // (--json / --jq) — the terminal renders ANSI just fine.
    go.env = { CLICOLOR_FORCE: "1" };
    const ready = new Promise<void>((res) => {
      g.__kapiCliReady = res;
    });

    const source = await fetchWasmBytes(wasmUrl);
    const instance = await instantiate(source, go.importObject);
    void go.run(instance); // blocks forever (select{}); not awaited
    await ready;

    return {
      vol: mem.vol,
      run: (argv: string[]) => g.kapiRun(argv) as Promise<number>,
      preview: (path: string) => g.kapiPreview(path) as Promise<PreviewResult>,
      inspect: async (path: string): Promise<InspectResult> => {
        const res = (await g.labInspect(path)) as {
          ok: boolean;
          error?: string;
          format?: string;
          json?: string;
          bytes?: number;
        };
        if (!res || !res.ok) return { ok: false, error: res?.error ?? "inspect failed" };
        try {
          return {
            ok: true,
            format: res.format,
            tree: res.json ? JSON.parse(res.json) : undefined,
            bytes: res.bytes,
          };
        } catch (e) {
          return { ok: false, error: `parse content tree: ${(e as Error).message}` };
        }
      },
      inspectAnnotated: async (path: string, opts?: AnnotateOptions): Promise<InspectResult> => {
        const fn = g.labInspectAnnotated;
        if (typeof fn !== "function") {
          return { ok: false, error: "labInspectAnnotated unavailable in this wasm build" };
        }
        // The wasm endpoint accepts an optional JSON options string; omit it to
        // let the engine default all annotators on.
        const res = (await (opts ? fn(path, JSON.stringify(opts)) : fn(path))) as {
          ok: boolean;
          error?: string;
          format?: string;
          json?: string;
          bytes?: number;
        };
        if (!res || !res.ok) return { ok: false, error: res?.error ?? "inspect failed" };
        try {
          return {
            ok: true,
            format: res.format,
            tree: res.json ? JSON.parse(res.json) : undefined,
            bytes: res.bytes,
          };
        } catch (e) {
          return { ok: false, error: `parse content tree: ${(e as Error).message}` };
        }
      },
      klf: (req: KlfRequest): KlfResponse => {
        const fn = g.klf;
        if (typeof fn !== "function") {
          return { ok: false, error: "klf endpoint unavailable in this wasm build" };
        }
        try {
          return JSON.parse(fn(JSON.stringify(req)) as string) as KlfResponse;
        } catch (e) {
          return { ok: false, error: `klf request failed: ${(e as Error).message}` };
        }
      },
      runWithTrace: async (argv: string[]): Promise<TraceRunResult> => {
        const tracePath = `/.lab/trace-${++traceSeq}.json`;
        const code = (await g.kapiRun([...argv, "--trace", tracePath])) as number;
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
  })();
  return booting;
}

/** True once boot has been started (used to skip a loading flash on re-open). */
export function isBooted(): boolean {
  return booting !== null;
}
