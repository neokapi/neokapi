import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useKapiPlaygroundConfig } from "@site/src/components/KapiPlayground/config";
import { configurePlugins, bootEngine, usePluginManager } from "@neokapi/kapi-playground/plugins";
import type { RunGate as RunGateState } from "@neokapi/kapi-lab/useRunGate";

// Shared boot helper for the curated result-view components.
//
// All three curated components (BlockPreview, BeforeAfter, DualExample) need a
// warm KapiRuntime — the one booted kapi WASM instance — to call `preview(path)`
// or `run(argv)` and read files back out of the in-memory volume. This hook
// centralizes the lazy boot:
//
//   • it imports `bootKapiRuntime` from @neokapi/kapi-playground dynamically, so
//     the heavy wasm boot path is a separate async chunk (a docs page that
//     never renders a curated view ships zero wasm);
//   • it resolves the wasm asset URLs through the existing Docusaurus adapter
//     (useKapiPlaygroundConfig), the same source the playground modal uses;
//   • boot is idempotent kit-side (bootKapiRuntime caches the in-flight promise),
//     so multiple curated views on one page share the single warm instance.
//
// The component that calls this must be rendered client-only (under
// BrowserOnly) — the dynamic import and the wasm boot only work in the browser.

// The runtime + preview/result types are imported as types only (erased at
// build time), so importing them here does NOT pull the heavy chunk into the
// initial bundle.
import type { KapiRuntime, PreviewResult } from "@neokapi/kapi-playground";

export type { KapiRuntime, PreviewResult };

export interface CuratedRuntimeState {
  /** The warm runtime once booted; null before Run / while booting. */
  runtime: KapiRuntime | null;
  /** Boot error message, if the wasm failed to load. */
  error: string;
  /**
   * True only on the very first boot of the page (no warm instance yet). Lets a
   * component show "Starting kapi for the first time…" vs. a quick spinner when
   * another curated view already warmed the runtime.
   */
  cold: boolean;
  /** True once the user has pressed Run (boot requested). */
  armed: boolean;
  /** Request boot. Nothing is fetched until this is called. */
  arm: () => void;
  /**
   * The same state shaped for the shared <RunGate> (compact) — so curated
   * views render the one site-wide activation gate instead of a bespoke one.
   */
  gate: RunGateState;
}

/**
 * Boot (or reuse) the shared kapi runtime for curated views — only after the
 * user presses Run (arm). Nothing is fetched on page load. Boot is routed
 * through the plugin manager so the navbar status widget reflects it, and is
 * idempotent + shared across every curated view and lab on the page.
 */
export function useCuratedRuntime(): CuratedRuntimeState {
  const { wasmExecUrl, wasmUrl } = useKapiPlaygroundConfig();
  const [runtime, setRuntime] = useState<KapiRuntime | null>(null);
  const [error, setError] = useState<string>("");
  const [armed, setArmed] = useState(false);
  // Bumped on every arm() so a Retry after a failed boot re-runs the boot
  // effect (bootEngine itself clears its cached promise on failure).
  const [attempt, setAttempt] = useState(0);
  // Whether boot had already started when this hook first ran. `isBooted()`
  // lives in the heavy chunk, so we infer "cold" cheaply: the first hook on the
  // page to reach the effect that resolves is cold; later mounts reuse the
  // cached promise and resolve near-instantly.
  const startedCold = useRef<boolean>(true);

  // Configure the shared manager with the asset URLs (no boot) so a Run here, in
  // a lab, or via the navbar widget all converge on one engine.
  useEffect(() => {
    if (wasmExecUrl && wasmUrl) configurePlugins({ wasmExecUrl, wasmUrl });
  }, [wasmExecUrl, wasmUrl]);

  useEffect(() => {
    if (!armed) return;
    let cancelled = false;
    void (async () => {
      try {
        const kit = await import("@neokapi/kapi-playground");
        startedCold.current = !kit.isBooted();
        const rt = (await bootEngine()) as KapiRuntime;
        if (!cancelled) setRuntime(rt);
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [armed, attempt]);

  const arm = useCallback(() => {
    setError("");
    setArmed(true);
    setAttempt((a) => a + 1);
  }, []);

  // Shape the state for the shared <RunGate compact> so curated views show the
  // site-wide gate copy (size hint · runs locally · warm-engine note). The
  // manager's engine progress feeds the gate's download bar; RunGate reads the
  // warm-engine state from the manager itself.
  const mgr = usePluginManager();
  const progress = mgr.state.engine.progress;
  const gate = useMemo<RunGateState>(
    () => ({
      armed,
      ready: armed && runtime !== null,
      status: error ? "error" : runtime !== null ? "ready" : armed ? "booting" : "idle",
      bootProgress: progress
        ? { loaded: progress.loaded ?? 0, total: progress.total ?? null }
        : null,
      error: error || null,
      requires: [],
      run: arm,
    }),
    [armed, runtime, error, progress, arm],
  );

  return { runtime, error, cold: startedCold.current, armed, arm, gate };
}
