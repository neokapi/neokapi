import { useCallback, useEffect, useRef, useState } from "react";
// Import from the light /runtime subpath (not the package index) so we don't
// pull xterm / the modal into explorer bundles — explorers never show a terminal.
import { bootKapiRuntime } from "@neokapi/kapi-playground/runtime";
import type { InspectResult, KapiRuntime, TraceRunResult } from "@neokapi/kapi-playground/runtime";
import type { ContentTree, FlowTrace } from "./types";

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
}

export interface LabRuntime {
  status: LabStatus;
  error: string | null;
  ready: boolean;
  /** Seed a file into the in-memory filesystem under /project. Returns its path. */
  writeFile: (filename: string, content: string) => string;
  inspect: (filename: string, content: string) => Promise<InspectOutcome>;
  /** Run a command with tracing; argv uses absolute /project paths. */
  trace: (argv: string[]) => Promise<TraceOutcome>;
  run: (argv: string[]) => Promise<number>;
  /** Read a file from the in-memory filesystem (decoded UTF-8), or null. */
  readFile: (path: string) => string | null;
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

  useEffect(() => {
    if (!assets) return;
    let cancelled = false;
    setStatus("booting");
    bootKapiRuntime(assets.wasmExecUrl, assets.wasmUrl)
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
  }, [assets]);

  const writeFile = useCallback((filename: string, content: string): string => {
    const rt = runtimeRef.current;
    const path = `${PROJECT_DIR}/${filename}`;
    if (rt) rt.vol.writeFile(path, enc.encode(content));
    return path;
  }, []);

  const inspect = useCallback(
    async (filename: string, content: string): Promise<InspectOutcome> => {
      const rt = runtimeRef.current;
      if (!rt) return { ok: false, error: "runtime not ready" };
      return serialized(async () => {
        const path = `${PROJECT_DIR}/${filename}`;
        rt.vol.writeFile(path, enc.encode(content));
        const res: InspectResult = await rt.inspect(path);
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
      const res: TraceRunResult = await rt.runWithTrace(argv);
      if (!res.trace) return { ok: false, error: `run exited ${res.code} with no trace` };
      return { ok: true, trace: res.trace as FlowTrace };
    });
  }, []);

  const run = useCallback(async (argv: string[]): Promise<number> => {
    const rt = runtimeRef.current;
    if (!rt) return 1;
    return serialized(() => rt.run(argv));
  }, []);

  const readFile = useCallback((path: string): string | null => {
    const rt = runtimeRef.current;
    if (!rt) return null;
    try {
      return new TextDecoder().decode(rt.vol.readFile(path));
    } catch {
      return null;
    }
  }, []);

  return {
    status,
    error,
    ready: status === "ready",
    writeFile,
    inspect,
    trace,
    run,
    readFile,
  };
}
