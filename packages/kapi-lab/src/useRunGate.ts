import { useCallback, useState } from "react";
import { ensurePlugin } from "@neokapi/kapi-playground/plugins";
import type { PluginId } from "@neokapi/kapi-playground/plugins";
import type { BootProgress } from "@neokapi/kapi-playground/runtime";
import type { LabRuntime, LabStatus } from "./useLabRuntime";

// useRunGate — the "nothing runs until the user says Run" primitive.
//
// Labs boot the wasm engine lazily (useLabRuntime with autoBoot:false), so on
// page load nothing is fetched or executed. A lab renders <RunGate> until the
// gate is `ready`; the Run action boots the engine (through the shared plugin
// manager, so the navbar widget reflects it) and downloads any required plugins.
// Once `ready`, the lab's normal body renders and its compute runs.

export interface RunGate {
  /** True once the user has pressed Run (boot requested). */
  armed: boolean;
  /** armed AND the engine has finished booting — render the lab body. */
  ready: boolean;
  /** Underlying engine status, for the idle/booting/error UI. */
  status: LabStatus;
  /** Engine boot download progress (≈13 MB), or null. */
  bootProgress: BootProgress | null;
  error: string | null;
  /** Plugins this gate ensures on Run (for the idle-state label). */
  requires: PluginId[];
  /** Boot the engine + ensure required plugins. Idempotent. */
  run: () => void;
}

export interface UseRunGateOptions {
  /** Plugins to download on Run (in addition to booting the engine). */
  requires?: PluginId[];
  /**
   * Start already-armed (skip the Run button). For stories/tests/embeds that
   * intentionally want the lab live on mount — pair with autoBoot:true.
   */
  autoArm?: boolean;
}

export function useRunGate(runtime: LabRuntime, opts: UseRunGateOptions = {}): RunGate {
  const requires = opts.requires ?? [];
  const [armed, setArmed] = useState(opts.autoArm ?? false);
  const requiresKey = requires.join(",");

  const run = useCallback(() => {
    setArmed(true);
    runtime.boot();
    for (const id of requiresKey ? (requiresKey.split(",") as PluginId[]) : []) {
      void ensurePlugin(id).catch(() => undefined);
    }
  }, [runtime, requiresKey]);

  return {
    armed,
    ready: armed && runtime.ready,
    status: runtime.status,
    bootProgress: runtime.bootProgress,
    error: runtime.error,
    requires,
    run,
  };
}
