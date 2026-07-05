// Ambient typings for every global the kapi WASM engine registers
// (kapi/cmd/kapi-wasm-cli main.go, `engineExports`) plus the optional
// host-provided reverse bridges. Importing anything from @neokapi/engine pulls
// these in, so no call site needs a `globalThis as any` cast.
//
// Everything is declared `| undefined`: before an engine has booted on the
// page none of the entry points exist, and the reverse bridges are optional
// capabilities the page may never install. Feature-detect via `engineABI()` /
// `hasEngineFunction()` (abi.ts) or `detectCapabilities()` (capabilities.ts).

import type {
  EngineABI,
  RawInspectResponse,
  RawPreviewResponse,
  RawSegmentResponse,
} from "./abi.ts";
import type {
  KapiBrowserTranslate,
  KapiLocalGenerate,
  KapiLocalNER,
  KapiPdfium,
} from "./capabilities.ts";

declare global {
  // ── Engine entry points (registered by the wasm binary at boot) ───────────

  /** Run one kapi CLI invocation; resolves to the process-style exit code. */
  // eslint-disable-next-line no-var
  var kapiRun: ((argv: string[]) => Promise<number>) | undefined;
  /** Block-level text preview of a file in the engine's filesystem. */
  // eslint-disable-next-line no-var
  var kapiPreview: ((path: string) => Promise<RawPreviewResponse>) | undefined;
  /** Parse a file into its ContentTree (serialized in `.json`). */
  // eslint-disable-next-line no-var
  var labInspect: ((path: string) => Promise<RawInspectResponse>) | undefined;
  /**
   * Like labInspect, but runs the read-only annotators first. `optsJSON` is a
   * serialized AnnotateOptions; omit it to default every annotator on.
   */
  // eslint-disable-next-line no-var
  var labInspectAnnotated:
    | ((path: string, optsJSON?: string) => Promise<RawInspectResponse>)
    | undefined;
  /** Segment raw text with a named engine ("" = default) and locale. Synchronous. */
  // eslint-disable-next-line no-var
  var labSegment:
    | ((text: string, engine: string, locale: string) => RawSegmentResponse)
    | undefined;
  /** List the segmentation engines registered in this wasm build. */
  // eslint-disable-next-line no-var
  var labSegmentEngines: (() => string[]) | undefined;
  /** KLF spec operations (JSON string in → JSON string out). Synchronous. */
  // eslint-disable-next-line no-var
  var klf: ((reqJSON: string) => string) | undefined;
  /** ABI descriptor for feature detection; absent on pre-ABI builds. */
  // eslint-disable-next-line no-var
  var kapiEngineABI: (() => EngineABI) | undefined;

  // ── Boot handshake (provided by the host, invoked by the engine) ──────────

  /** Resolved by the engine once all entry points are registered. */
  // eslint-disable-next-line no-var
  var __kapiCliReady: (() => void) | undefined;

  // ── Reverse bridges (optional capabilities the host page may provide) ─────

  /** PDF text + geometry bridge (see capabilities.ts). */
  // eslint-disable-next-line no-var
  var __kapiPdfium: KapiPdfium | undefined;
  /** On-device LLM bridge behind the browser "local" AI provider. */
  // eslint-disable-next-line no-var
  var kapiLocalGenerate: KapiLocalGenerate | undefined;
  /** On-device NER bridge behind `entity-extract` engine "ner". */
  // eslint-disable-next-line no-var
  var kapiLocalNER: KapiLocalNER | undefined;
  /** MT bridge driving the platform's built-in Translator API. */
  // eslint-disable-next-line no-var
  var kapiBrowserTranslate: KapiBrowserTranslate | undefined;
}

export {};
