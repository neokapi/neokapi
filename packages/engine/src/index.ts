// @neokapi/engine — the kapi content engine (compiled to WebAssembly) as a
// typed, dependency-light npm package.
//
// This package owns the boot path (in-memory filesystem, wasm_exec loader,
// instantiation, ready handshake) and the `KapiRuntime` facade over the
// engine's global function set. The wasm binary itself is NOT bundled — it is
// a ~13 MB (gzipped) asset the host serves or points at (see the README for
// CDN vs self-hosted loading patterns).
//
// Deliberately free of UI and heavy ML dependencies (no xterm, monaco,
// pdfium, onnxruntime): those live in host kits such as
// @neokapi/kapi-playground, which builds on this package.

// Boot + runtime facade.
export { bootKapiRuntime, isBooted, makeRuntime, onBootProgress } from "./runtime.ts";
export type {
  AnnotateOptions,
  BootProgress,
  InspectResult,
  KapiRuntime,
  KlfRequest,
  KlfResponse,
  PreviewBlock,
  PreviewResult,
  SegmentPiece,
  SegmentResult,
  TraceRunResult,
} from "./runtime.ts";

// In-memory filesystem (the Node-fs subset Go's js/wasm runtime calls).
export { createMemFS } from "./memfs.ts";
export type { MemFS, MemVolume } from "./memfs.ts";

// Versioned ABI: feature detection + the raw wire shapes.
export { engineABI, hasEngineFunction, SUPPORTED_ENGINE_ABI } from "./abi.ts";
export type {
  EngineABI,
  RawInspectResponse,
  RawPreviewResponse,
  RawSegmentResponse,
} from "./abi.ts";

// Reverse-bridge capabilities the host page may provide.
export { detectCapabilities } from "./capabilities.ts";
export type {
  BrowserTranslatePayload,
  BrowserTranslateResult,
  CapabilityStatus,
  KapiBrowserTranslate,
  KapiEngineCapabilities,
  KapiLocalGenerate,
  KapiLocalNER,
  KapiPdfium,
  LocalGenerateMessage,
  LocalGeneratePayload,
  LocalGenerateResult,
  LocalNEREntity,
  LocalNERRequest,
  LocalNERResponse,
  PdfExtract,
  PdfGlyph,
  PdfPage,
  PdfRect,
} from "./capabilities.ts";

// Ambient typings for the engine globals (side-effect import so any consumer
// of the package gets typed `globalThis.kapiRun` & co.).
import "./globals.ts";
