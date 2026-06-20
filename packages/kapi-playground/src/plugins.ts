// Plugin manager — the single source of truth for "what is loaded in this
// browser tab", shared by the navbar status widget and every lab.
//
// It mirrors how kapi models plugins on desktop/CLI: a small registry of plugin
// descriptors plus live per-plugin state (idle → downloading → ready), with the
// wasm engine itself tracked as the core. Labs request a plugin on demand
// (ensure), the widget renders the same state, and a download triggered from
// either surface lights up both — there is exactly one store.
//
// This module stays SSR-clean and bundle-light: it has NO static import of a
// heavy bridge or of the wasm runtime. Everything that pulls onnxruntime-web /
// transformers.js / ffmpeg / the wasm boot path is loaded via dynamic import()
// the first time a plugin (or the engine) is actually ensured. So importing the
// manager (e.g. into the navbar) costs nothing until the user acts.

import { useSyncExternalStore } from "react";

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

export type PluginId =
  | "llm"
  | "okapi-bridge"
  | "pdfium"
  | "sat"
  | "vision"
  | "asr"
  | "av"
  | "bowrain";

/** Lifecycle of the shared wasm engine (drives the widget's top status dot). */
export type EnginePhase = "idle" | "booting" | "ready" | "error";

/** Lifecycle of a single plugin. `unavailable` = not runnable in a browser. */
export type PluginPhase = "unavailable" | "idle" | "downloading" | "ready" | "error";

/** Download/progress, byte-accurate when known (loaded/total) or fractional. */
export interface Progress {
  loaded?: number;
  total?: number;
  /** 0..1 when bytes are unknown but the bridge reports a fraction. */
  frac?: number;
}

export interface EngineState {
  phase: EnginePhase;
  progress?: Progress;
  error?: string;
}

export interface PluginState {
  phase: PluginPhase;
  progress?: Progress;
  error?: string;
}

export interface LabState {
  engine: EngineState;
  plugins: Record<PluginId, PluginState>;
}

export interface PluginDescriptor {
  id: PluginId;
  /** Short id-style label shown in the panel (e.g. "llm", "okapi-bridge"). */
  label: string;
  /** One-line human description (e.g. "Local LLM with Gemma 4"). */
  description: string;
  /** Approximate download size in bytes, for the panel; omit when trivial. */
  sizeBytes?: number;
  /**
   * False for plugins that cannot run in a browser at all (the cgo Java bridge,
   * the server-backed collaboration plugin). Rendered greyed/disabled.
   */
  browserSupported: boolean;
  /** True if the plugin's work runs through the wasm engine (so boot it first). */
  needsEngine: boolean;
}

// Registry. Order matches the desktop/CLI plugin list and the widget sketch.
export const PLUGIN_DESCRIPTORS: PluginDescriptor[] = [
  {
    id: "llm",
    label: "llm",
    description: "Local LLM with Gemma 4",
    // Approximate hint shown before download — the widget switches to the live
    // aggregate ("X of Y") once shards start arriving. Gemma 4 E2B is multimodal
    // (decoder + embeddings + vision/audio encoders); only the decoder ships a
    // q4f16 variant, so the rest load at fp16 and the full download is ~6 GB.
    sizeBytes: 6_000_000_000,
    browserSupported: true,
    needsEngine: true,
  },
  {
    id: "okapi-bridge",
    label: "okapi-bridge",
    description: "Okapi Java bridge",
    browserSupported: false, // cgo + JVM — native only
    needsEngine: true,
  },
  {
    id: "pdfium",
    label: "pdfium",
    description: "PDF format plugin",
    sizeBytes: 4_600_000,
    browserSupported: true,
    needsEngine: true,
  },
  {
    id: "sat",
    label: "sat",
    description: 'SaT "Segment any Text" ML model',
    sizeBytes: 428_000_000,
    browserSupported: true,
    needsEngine: false, // segmentation runs directly in JS (onnxruntime-web)
  },
  {
    id: "vision",
    label: "vision",
    description: "Document vision for Kapi — OCR",
    sizeBytes: 21_000_000,
    browserSupported: true,
    needsEngine: false,
  },
  {
    id: "asr",
    label: "asr",
    description: "Speech recognition (ASR) for kapi",
    sizeBytes: 39_000_000,
    browserSupported: true,
    needsEngine: false,
  },
  {
    id: "av",
    label: "av",
    description: "Extract audio and text from video",
    sizeBytes: 32_000_000,
    browserSupported: true,
    needsEngine: false,
  },
  {
    id: "bowrain",
    label: "bowrain",
    description: "Collaboration and governance",
    browserSupported: false, // server-backed; no browser-only mode
    needsEngine: false,
  },
];

const DESCRIPTOR_BY_ID = new Map(PLUGIN_DESCRIPTORS.map((d) => [d.id, d]));

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

function initialPlugins(): Record<PluginId, PluginState> {
  const out = {} as Record<PluginId, PluginState>;
  for (const d of PLUGIN_DESCRIPTORS) {
    out[d.id] = { phase: d.browserSupported ? "idle" : "unavailable" };
  }
  return out;
}

let state: LabState = {
  engine: { phase: "idle" },
  plugins: initialPlugins(),
};

const listeners = new Set<() => void>();

function emit() {
  // Fresh top-level object so useSyncExternalStore's identity check fires.
  state = { engine: state.engine, plugins: state.plugins };
  for (const fn of listeners) fn();
}

function setEngine(patch: Partial<EngineState>) {
  state.engine = { ...state.engine, ...patch };
  emit();
}

function setPlugin(id: PluginId, patch: Partial<PluginState>) {
  state.plugins = { ...state.plugins, [id]: { ...state.plugins[id], ...patch } };
  emit();
}

/** Subscribe to store changes; returns an unsubscribe. */
export function subscribePlugins(fn: () => void): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

/** Current snapshot (stable identity until the next change). */
export function getPluginState(): LabState {
  return state;
}

/** Count of supported plugins currently loaded, and the supported total. */
export function pluginCounts(): { ready: number; total: number } {
  let ready = 0;
  let total = 0;
  for (const d of PLUGIN_DESCRIPTORS) {
    if (!d.browserSupported) continue;
    total++;
    if (state.plugins[d.id].phase === "ready") ready++;
  }
  return { ready, total };
}

// ---------------------------------------------------------------------------
// Asset URLs (injected by the host — same wasmExecUrl/wasmUrl as the runtime)
// ---------------------------------------------------------------------------

let assets: { wasmExecUrl: string; wasmUrl: string } | null = null;

/**
 * configurePlugins injects the wasm asset URLs (from the Docusaurus config /
 * playground provider). Idempotent and required before bootEngine/ensure for
 * engine-backed plugins. Safe to call from every lab and from the widget.
 */
export function configurePlugins(a: { wasmExecUrl: string; wasmUrl: string }): void {
  assets = a;
}

// ---------------------------------------------------------------------------
// Engine boot
// ---------------------------------------------------------------------------

let enginePromise: Promise<unknown> | null = null;

/**
 * bootEngine boots (once) the shared kapi wasm engine, reflecting download
 * progress into the store so the widget shows it. Returns the runtime. Throws
 * if the asset URLs were not configured.
 */
export function bootEngine(): Promise<unknown> {
  if (enginePromise) return enginePromise;
  if (!assets) {
    return Promise.reject(new Error("plugin manager not configured (call configurePlugins)"));
  }
  const { wasmExecUrl, wasmUrl } = assets;
  setEngine({ phase: "booting", progress: { loaded: 0 }, error: undefined });
  enginePromise = (async () => {
    const { bootKapiRuntime, onBootProgress } = await import("./runtime");
    const off = onBootProgress((p) => {
      if (p.done) return;
      setEngine({ progress: { loaded: p.loaded, total: p.total ?? undefined } });
    });
    try {
      const rt = await bootKapiRuntime(wasmExecUrl, wasmUrl);
      setEngine({ phase: "ready", progress: undefined });
      return rt;
    } catch (e) {
      setEngine({ phase: "error", error: e instanceof Error ? e.message : String(e) });
      enginePromise = null; // allow retry
      throw e;
    } finally {
      off();
    }
  })();
  return enginePromise;
}

/** True once the engine boot has been started/succeeded. */
export function engineBooted(): boolean {
  return state.engine.phase === "ready";
}

// ---------------------------------------------------------------------------
// Per-plugin ensure adapters
// ---------------------------------------------------------------------------

// Each adapter loads its bridge lazily and reports progress through the store.
// Adapters are keyed by id; missing/native-only ids reject.
const adapters: Partial<Record<PluginId, (report: (p: Progress) => void) => Promise<void>>> = {
  llm: async (report) => {
    const { ensureGemma } = await import("./gemmaBridge");
    await ensureGemma({
      onProgress: (p) => {
        if (p.status === "ready") {
          report({ frac: 1 });
        } else if (typeof p.loaded === "number" && typeof p.total === "number" && p.total > 0) {
          // Prefer the aggregate byte total across all shards — a smooth,
          // ~monotonic percentage instead of the per-shard fraction.
          report({ loaded: p.loaded, total: p.total });
        } else {
          report({ frac: (p.progress ?? 0) / 100 });
        }
      },
    });
  },
  pdfium: async () => {
    if (!assets) throw new Error("pdfium needs configured asset URLs");
    const { installPdfiumBridge } = await import("./pdfiumBridge");
    installPdfiumBridge(assets.wasmUrl.replace(/[^/]*$/, "pdfium.wasm"));
    const pdf = (globalThis as Record<string, unknown>).__kapiPdfium as
      | { ready?: Promise<void> }
      | undefined;
    await pdf?.ready;
  },
  sat: async (report) => {
    const { ensureSat } = await import("./satBridge");
    await ensureSat((frac) => report({ frac }));
  },
  vision: async (report) => {
    const { ensureOCR } = await import("./visionBridge");
    report({ frac: 0 });
    await ensureOCR();
    report({ frac: 1 });
  },
  asr: async (report) => {
    const { ensureASRModel } = await import("./asrBridge");
    await ensureASRModel((frac) => report({ frac }));
  },
  av: async (report) => {
    const { ensureAV } = await import("./avBridge");
    await ensureAV((frac) => report({ frac }));
  },
};

// In-flight ensure promises so concurrent callers (a lab + the widget) share one
// download rather than racing two.
const ensuring = new Map<PluginId, Promise<void>>();

/**
 * ensurePlugin downloads + loads a plugin on demand, booting the engine first
 * when the plugin needs it. Idempotent: a plugin already ready resolves
 * immediately; a concurrent call shares the in-flight download. Progress flows
 * into the store. Rejects for browser-unsupported plugins.
 */
export function ensurePlugin(id: PluginId): Promise<void> {
  const d = DESCRIPTOR_BY_ID.get(id);
  if (!d) return Promise.reject(new Error(`unknown plugin: ${id}`));
  if (!d.browserSupported) {
    return Promise.reject(new Error(`${id} is not available in the browser (native/server only)`));
  }
  if (state.plugins[id].phase === "ready") return Promise.resolve();
  const inflight = ensuring.get(id);
  if (inflight) return inflight;

  const run = (async () => {
    setPlugin(id, { phase: "downloading", progress: { frac: 0 }, error: undefined });
    try {
      if (d.needsEngine) await bootEngine();
      const adapter = adapters[id];
      if (!adapter) throw new Error(`no adapter for ${id}`);
      await adapter((p) => setPlugin(id, { progress: p }));
      setPlugin(id, { phase: "ready", progress: undefined });
    } catch (e) {
      setPlugin(id, { phase: "error", error: e instanceof Error ? e.message : String(e) });
      throw e;
    } finally {
      ensuring.delete(id);
    }
  })();
  ensuring.set(id, run);
  return run;
}

// ---------------------------------------------------------------------------
// React hook
// ---------------------------------------------------------------------------

export interface UsePluginManager {
  state: LabState;
  descriptors: PluginDescriptor[];
  counts: { ready: number; total: number };
  ensure: (id: PluginId) => Promise<void>;
  bootEngine: () => Promise<unknown>;
  configure: (a: { wasmExecUrl: string; wasmUrl: string }) => void;
}

const SERVER_SNAPSHOT: LabState = { engine: { phase: "idle" }, plugins: initialPlugins() };

/**
 * usePluginManager subscribes a component to the shared plugin store. The state
 * updates whenever the engine or any plugin changes, from this component or any
 * other surface (a lab or the navbar widget) — one store, many views.
 */
export function usePluginManager(): UsePluginManager {
  const snap = useSyncExternalStore(subscribePlugins, getPluginState, () => SERVER_SNAPSHOT);
  return {
    state: snap,
    descriptors: PLUGIN_DESCRIPTORS,
    counts: pluginCounts(),
    ensure: ensurePlugin,
    bootEngine,
    configure: configurePlugins,
  };
}
