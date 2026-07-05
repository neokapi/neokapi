// The engine ABI — the versioned, feature-detectable contract between the kapi
// WASM binary (kapi/cmd/kapi-wasm-cli) and this package.
//
// The Go side registers every entry point from a single table (`engineExports`
// in main.go) and additionally installs `kapiEngineABI()`, which returns the
// descriptor typed here. Within one `abi` value the contract is strictly
// additive: new functions may appear in `functions`, but existing names,
// signatures, and payload shapes never change or disappear. A wasm build
// without `kapiEngineABI` predates the descriptor and should be treated as
// abi 0 (probe the individual globals instead).
//
// The raw wire shapes below are exactly what the Go entry points return
// (converted JS objects or JSON strings) — the `KapiRuntime` facade in
// runtime.ts is the ergonomic layer on top.

/** Descriptor returned by the engine's `kapiEngineABI()` global. */
export interface EngineABI {
  /** ABI major version. Bumped only on a breaking change to an entry point. */
  abi: number;
  /** The kapi build version (ldflags-stamped; "dev" for source builds). */
  version: string;
  /** Names of the globals registered by this build, for feature detection. */
  functions: string[];
}

/** The newest engine ABI version this package understands. */
export const SUPPORTED_ENGINE_ABI = 1;

/** Wire shape of `kapiPreview(path)` (a converted JS object). */
export interface RawPreviewResponse {
  ok: boolean;
  error?: string;
  format?: string;
  blocks?: { id: string; text: string }[];
  total?: number;
  bytes?: number;
}

/**
 * Wire shape of `labInspect(path)` / `labInspectAnnotated(path, optsJSON?)`.
 * `json` carries the serialized ContentTree; the facade parses it.
 */
export interface RawInspectResponse {
  ok: boolean;
  error?: string;
  format?: string;
  json?: string;
  bytes?: number;
}

/** Wire shape of `labSegment(text, engine, locale)` (a converted JS object). */
export interface RawSegmentResponse {
  ok: boolean;
  error?: string;
  engine?: string;
  segments?: { text: string }[];
}

/**
 * Read the engine's ABI descriptor, or null when this wasm build predates
 * `kapiEngineABI` (or no engine has booted on this page yet).
 */
export function engineABI(): EngineABI | null {
  const fn = globalThis.kapiEngineABI;
  if (typeof fn !== "function") return null;
  const raw = fn();
  return {
    abi: raw.abi,
    version: raw.version,
    functions: Array.from(raw.functions ?? []),
  };
}

/**
 * Whether the booted engine registered a global entry point of this name.
 * Uses the ABI descriptor when available and falls back to probing the global
 * directly on pre-ABI builds.
 */
export function hasEngineFunction(name: string): boolean {
  const abi = engineABI();
  if (abi) return abi.functions.includes(name);
  return typeof (globalThis as Record<string, unknown>)[name] === "function";
}
