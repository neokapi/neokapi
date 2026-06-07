import { useCallback, useEffect, useMemo, useRef, useState } from "react";
// Import from the light /runtime subpath (not the package index) so we don't
// pull xterm / the modal into explorer bundles — explorers never show a terminal.
import { bootKapiRuntime } from "@neokapi/kapi-playground/runtime";
import type {
  AnnotateOptions,
  InspectResult,
  KapiRuntime,
  KlfRequest,
  KlfResponse,
  SegmentResult,
  TraceRunResult,
} from "@neokapi/kapi-playground/runtime";
import type { ContentTree, FlowTrace } from "@neokapi/ui-primitives/preview";

export type LabStatus = "idle" | "booting" | "ready" | "error";

export interface LabRuntimeAssets {
  wasmExecUrl: string;
  wasmUrl: string;
}

export interface InspectOutcome {
  ok: boolean;
  error?: string;
  format?: string;
  tree?: ContentTree;
}

export interface TraceOutcome {
  ok: boolean;
  error?: string;
  trace?: FlowTrace;
  /** Captured stdout + stderr from the run (e.g. script log() output). */
  output?: string;
}

const noop = () => {};

export interface LabRuntime {
  status: LabStatus;
  error: string | null;
  ready: boolean;
  /** Create a directory (and parents) under /project. */
  mkdir: (path: string) => void;
  /** Seed a file into the in-memory filesystem under /project. Accepts text or
   *  raw bytes (use bytes for binary formats like .docx). Returns its path. */
  writeFile: (filename: string, data: string | Uint8Array) => string;
  inspect: (filename: string, data: string | Uint8Array) => Promise<InspectOutcome>;
  /**
   * Inspect like {@link inspect}, but with the engine's read-only annotators
   * (terminology, brand vocabulary, rule-based QA) run first so the blocks carry
   * stand-off overlays. `opts` toggles individual annotators (default all on).
   */
  inspectAnnotated: (
    filename: string,
    data: string | Uint8Array,
    opts?: AnnotateOptions,
  ) => Promise<InspectOutcome>;
  /** Run a command with tracing; argv uses absolute /project paths. */
  trace: (argv: string[]) => Promise<TraceOutcome>;
  run: (argv: string[]) => Promise<number>;
  /** Run a command capturing stdout+stderr (e.g. `info --json`). */
  runCapture: (argv: string[]) => Promise<{ code: number; output: string }>;
  /** Read a file from the in-memory filesystem (decoded UTF-8), or null. */
  readFile: (path: string) => string | null;
  /** Read raw bytes from the in-memory filesystem, or null (for binary
   *  outputs like .docx — used to confirm a valid OOXML zip was produced). */
  readBytes: (path: string) => Uint8Array | null;
  /** Run a KLF spec operation against the canonical Go engine (synchronous). */
  klf: (req: KlfRequest) => KlfResponse;
  /** Segment raw text with a named engine + locale (synchronous). */
  segment: (text: string, engine: string, locale: string) => SegmentResult;
  /** Segmentation engine names registered in this wasm build. */
  segmentEngines: () => string[];
}

const PROJECT_DIR = "/project";
const enc = new TextEncoder();

// One shared kapi WASM instance backs every explorer (bootKapiRuntime is an
// idempotent module singleton). Commands mutate global os.Stdout + a fresh
// cobra root, so concurrent invocations could interleave; serialize them on a
// single module-level promise chain so explorers on the same page take turns.
let runChain: Promise<unknown> = Promise.resolve();
function serialized<T>(fn: () => Promise<T>): Promise<T> {
  const next = runChain.then(fn, fn);
  runChain = next.then(
    () => undefined,
    () => undefined,
  );
  return next;
}

// useLabRuntime boots (or reuses) the kapi WASM and exposes the structured lab
// calls. Booting is lazy and client-only — call it from a <BrowserOnly> subtree.
export function useLabRuntime(assets: LabRuntimeAssets | null): LabRuntime {
  const [status, setStatus] = useState<LabStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const runtimeRef = useRef<KapiRuntime | null>(null);

  // Key the boot on the URL *strings*, not the `assets` object identity — a
  // caller that returns a fresh config object every render would otherwise re-run
  // this effect forever (setStatus → re-render → new object → re-run), which
  // pegs the tab ("Maximum update depth"). The strings are stable.
  const wasmExecUrl = assets?.wasmExecUrl;
  const wasmUrl = assets?.wasmUrl;
  useEffect(() => {
    if (!wasmExecUrl || !wasmUrl) return;
    let cancelled = false;
    setStatus("booting");
    bootKapiRuntime(wasmExecUrl, wasmUrl)
      .then((rt) => {
        if (cancelled) return;
        runtimeRef.current = rt;
        setStatus("ready");
      })
      .catch((e) => {
        if (cancelled) return;
        setError(e instanceof Error ? e.message : String(e));
        setStatus("error");
      });
    return () => {
      cancelled = true;
    };
  }, [wasmExecUrl, wasmUrl]);

  const mkdir = useCallback((path: string): void => {
    const rt = runtimeRef.current;
    if (rt) rt.vol.mkdirp(`${PROJECT_DIR}/${path}`);
  }, []);

  const writeFile = useCallback((filename: string, data: string | Uint8Array): string => {
    const rt = runtimeRef.current;
    const path = `${PROJECT_DIR}/${filename}`;
    if (rt) rt.vol.writeFile(path, typeof data === "string" ? enc.encode(data) : data);
    return path;
  }, []);

  const inspect = useCallback(
    async (filename: string, data: string | Uint8Array): Promise<InspectOutcome> => {
      const rt = runtimeRef.current;
      if (!rt) return { ok: false, error: "runtime not ready" };
      return serialized(async () => {
        const path = `${PROJECT_DIR}/${filename}`;
        rt.vol.writeFile(path, typeof data === "string" ? enc.encode(data) : data);
        const res: InspectResult = await rt.inspect(path);
        if (!res.ok) return { ok: false, error: res.error };
        return { ok: true, format: res.format, tree: res.tree as ContentTree };
      });
    },
    [],
  );

  const inspectAnnotated = useCallback(
    async (
      filename: string,
      data: string | Uint8Array,
      opts?: AnnotateOptions,
    ): Promise<InspectOutcome> => {
      const rt = runtimeRef.current;
      if (!rt) return { ok: false, error: "runtime not ready" };
      return serialized(async () => {
        const path = `${PROJECT_DIR}/${filename}`;
        rt.vol.writeFile(path, typeof data === "string" ? enc.encode(data) : data);
        const res: InspectResult = await rt.inspectAnnotated(path, opts);
        if (!res.ok) return { ok: false, error: res.error };
        return { ok: true, format: res.format, tree: res.tree as ContentTree };
      });
    },
    [],
  );

  const trace = useCallback(async (argv: string[]): Promise<TraceOutcome> => {
    const rt = runtimeRef.current;
    if (!rt) return { ok: false, error: "runtime not ready" };
    return serialized(async () => {
      // Capture stdout/stderr for the duration of the run so callers can show
      // script log() output and diagnostics. Safe because runs are serialized.
      let captured = "";
      rt.setSinks(
        (s) => (captured += s),
        (s) => (captured += s),
      );
      const res: TraceRunResult = await rt.runWithTrace(argv);
      rt.setSinks(noop, noop);
      if (!res.trace)
        return { ok: false, error: `run exited ${res.code} with no trace`, output: captured };
      return { ok: true, trace: res.trace as FlowTrace, output: captured };
    });
  }, []);

  const run = useCallback(async (argv: string[]): Promise<number> => {
    const rt = runtimeRef.current;
    if (!rt) return 1;
    return serialized(() => rt.run(argv));
  }, []);

  const runCapture = useCallback(
    async (argv: string[]): Promise<{ code: number; output: string }> => {
      const rt = runtimeRef.current;
      if (!rt) return { code: 1, output: "runtime not ready" };
      return serialized(async () => {
        let captured = "";
        rt.setSinks(
          (s) => (captured += s),
          (s) => (captured += s),
        );
        const code = await rt.run(argv);
        rt.setSinks(noop, noop);
        return { code, output: captured };
      });
    },
    [],
  );

  const readFile = useCallback((path: string): string | null => {
    const rt = runtimeRef.current;
    if (!rt) return null;
    try {
      return new TextDecoder().decode(rt.vol.readFile(path));
    } catch {
      return null;
    }
  }, []);

  const readBytes = useCallback((path: string): Uint8Array | null => {
    const rt = runtimeRef.current;
    if (!rt) return null;
    try {
      return rt.vol.readFile(path);
    } catch {
      return null;
    }
  }, []);

  const klf = useCallback((req: KlfRequest): KlfResponse => {
    const rt = runtimeRef.current;
    if (!rt) return { ok: false, error: "runtime not ready" };
    // The klf endpoint is pure CPU work over an in-memory JSON payload (no fs,
    // no shared stdout), so it needs no serialization against the run chain.
    return rt.klf(req);
  }, []);

  const segment = useCallback((text: string, engine: string, locale: string): SegmentResult => {
    const rt = runtimeRef.current;
    if (!rt) return { ok: false, error: "runtime not ready" };
    // Like klf: pure CPU (plus, for uax29, one re-entrant ICU4X JS call) over an
    // in-memory string, no fs or shared stdout — no run-chain serialization.
    return rt.segment(text, engine, locale);
  }, []);

  const segmentEngines = useCallback((): string[] => {
    const rt = runtimeRef.current;
    return rt ? rt.segmentEngines() : [];
  }, []);

  // Memoize the returned object: every method is useCallback-stable, so the
  // identity changes only when status/error change. Without this, consumers
  // that put the whole `runtime` in an effect's dep array re-run that effect
  // every render → "Maximum update depth exceeded".
  return useMemo(
    () => ({
      status,
      error,
      ready: status === "ready",
      mkdir,
      writeFile,
      inspect,
      inspectAnnotated,
      trace,
      run,
      runCapture,
      readFile,
      readBytes,
      klf,
      segment,
      segmentEngines,
    }),
    [
      status,
      error,
      mkdir,
      writeFile,
      inspect,
      inspectAnnotated,
      trace,
      run,
      runCapture,
      readFile,
      readBytes,
      klf,
      segment,
      segmentEngines,
    ],
  );
}
