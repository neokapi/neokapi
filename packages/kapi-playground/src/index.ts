// @neokapi/kapi-playground — a framework-agnostic kit for running the real
// kapi CLI (compiled to WebAssembly) in the browser.
//
// This package does NOT import @docusaurus/*, @theme/*, or BrowserOnly. Asset
// URLs (wasmUrl / wasmExecUrl) are injected via <KapiPlaygroundProvider>, so
// the kit works under Docusaurus, a plain React page (the landing hero), or
// Storybook. The host is responsible for:
//   1. wrapping the app in <KapiPlaygroundProvider config={{ wasmUrl, wasmExecUrl }}>
//   2. mounting exactly one <KapiModal /> near the root
//   3. rendering <RunnableSnippet> / <KapiEmbed> where desired, and/or calling
//      openKapi(...) imperatively.
//
// Only <RunnableSnippet> (inline trigger) and the store are SSR/light-weight.
// <KapiModal> / <KapiEmbed> pull in xterm + the wasm boot path and should be
// rendered client-only (the host wraps them) — they are the heavy payload.

import "./styles.css";

// Inline trigger + imperative API (light, SSR-clean — no xterm/wasm).
export { default as RunnableSnippet } from "./RunnableSnippet";
export type { RunnableSnippetProps } from "./RunnableSnippet";
export { openKapi, serializeSession, deserializeSession } from "./store";
export type { OpenKapiOptions, KapiFile, BinaryKapiFile, SessionState } from "./store";

// Provider for injected asset URLs.
export { KapiPlaygroundProvider, useKapiConfig } from "./provider";
export type { KapiPlaygroundConfig } from "./provider";

// Heavy, client-only components.
export { default as KapiModal } from "./KapiModal";
export { default as KapiEmbed } from "./KapiEmbed";
export type { KapiEmbedProps, KapiEmbedHandle, KapiRunRequest } from "./KapiEmbed";

// Runtime + fixtures (for advanced hosts / tooling).
export { bootKapiRuntime, isBooted } from "./runtime";
export type {
  KapiRuntime,
  PreviewResult,
  PreviewBlock,
  InspectResult,
  AnnotateOptions,
  TraceRunResult,
} from "./runtime";
export { fixtureNames, getFixture } from "./fixtures";
export type { Fixture } from "./fixtures";

// Curated sample library — the single source of truth shared by the docs CLI
// playground picker and the kapi-lab explorers (SSR-clean; no xterm/wasm).
export {
  LOOSE_SAMPLES,
  PROJECT_SAMPLES,
  projectSampleById,
  WORKSPACE_SAMPLES,
  workspaceSampleById,
  HERO_SAMPLES,
  heroSampleById,
  TRY_SAMPLES,
  trySampleById,
  DOCX_B64,
  XLSX_B64,
  TRY_XLSX_B64,
  TRY_PPTX_B64,
  TRY_MD_B64,
  JSON_SAMPLE,
  tmxOf,
} from "./samples";
export type { LooseSample, ProjectSample, WorkspaceSample, HeroSample, TrySample } from "./samples";

// Gemma bridge — installs the host hook so `kapi --provider gemma` runs the real
// Gemma 4 model in-browser via transformers.js/WebGPU (opt-in, lazy download).
export {
  installGemmaBridge,
  uninstallGemmaBridge,
  isGemmaModelLoaded,
  ensureGemma,
  generateGemmaText,
  runGemmaImageOCR,
} from "./gemmaBridge";
export type { InstallGemmaOptions, GemmaProgress, GemmaResult } from "./gemmaBridge";

// Plugin manager — the shared "what is loaded in this tab" store read by the
// navbar status widget and every lab (SSR-clean; heavy bridges are lazy). Prefer
// the light "@neokapi/kapi-playground/plugins" subpath in bundle-sensitive hosts.
export {
  PLUGIN_DESCRIPTORS,
  configurePlugins,
  bootEngine,
  engineBooted,
  ensurePlugin,
  subscribePlugins,
  getPluginState,
  pluginCounts,
  usePluginManager,
} from "./plugins";
export type {
  PluginId,
  PluginPhase,
  EnginePhase,
  Progress,
  EngineState,
  PluginState,
  LabState,
  PluginDescriptor,
  UsePluginManager,
} from "./plugins";

// SaT segmentation — the real wtpsplit "Segment any Text" model on
// onnxruntime-web (opt-in, lazy ~428 MB download), mirroring the native kapi-sat.
export { segmentSat, ensureSat, satLoaded } from "./satBridge";
export type { SatSegmentResult } from "./satBridge";
