// The messages between a page and the engine's Worker.
//
// The engine's globals (the ABI, abi.ts) live in the Worker. The page's
// facade (runtime.ts) sends each call as a message, and the Worker answers
// with the call's value after the file system changes the call made, so a
// page that reads its mirror of the file system after a call sees them. The
// host capabilities a page provides (capabilities.ts) stay on the page: the
// Worker installs stand-ins that ask the page.

import type { EngineABI } from "./abi.ts";
import type { BootProgress } from "./fetch.ts";
import type { StorageInfo, VolOp, WhenHeld } from "./storage.ts";

/** What the page asks the Worker to boot with. */
export interface WorkerBoot {
  wasmExecUrl: string;
  wasmUrl: string;
  sqliteWasmUrl: string;
  /** The storage key, or null for a workspace in memory. */
  persist: string | null;
  /** What to do when another tab holds the workspace. */
  whenHeld?: WhenHeld;
}

export type ToWorker =
  | { t: "boot"; boot: WorkerBoot }
  | { t: "call"; id: number; fn: string; args: unknown[]; bridges: string[] }
  | { t: "vol"; ops: VolOp[] }
  | { t: "chdir"; dir: string }
  | { t: "bridge-result"; id: number; ok: boolean; value?: unknown; error?: string };

export type FromWorker =
  | { t: "progress"; progress: BootProgress }
  | { t: "ready"; storage: StorageInfo; ops: VolOp[]; cwd: string; abi: EngineABI | null }
  | { t: "boot-error"; error: string }
  | { t: "out" | "err"; text: string }
  | { t: "vol"; ops: VolOp[] }
  | { t: "result"; id: number; ok: boolean; value?: unknown; error?: string; cwd: string }
  | { t: "bridge"; id: number; name: string; args: unknown[] }
  // Another tab took the workspace over: the Worker kept its last changes and
  // no longer uses the pool, and the page stops it.
  | { t: "lost" };

/**
 * The page globals the engine may call back (capabilities.ts), which the
 * Worker answers by asking the page. `Translator` is the platform API whose
 * presence the browser MT provider checks beside its bridge.
 */
export const PAGE_FUNCTIONS = [
  "kapiLocalGenerate",
  "kapiLocalNER",
  "kapiBrowserTranslate",
  "kapiICU4XSentenceBreaks",
] as const;

/** Every page global whose presence the Worker mirrors. */
export const PAGE_BRIDGES = [
  ...PAGE_FUNCTIONS,
  "__kapiPdfium",
  "kapiIntlSentenceBreaks",
  "Translator",
] as const;

/** The page bridges present now. */
export function presentBridges(): string[] {
  const g = globalThis as Record<string, unknown>;
  return PAGE_BRIDGES.filter((name) => g[name] != null);
}
