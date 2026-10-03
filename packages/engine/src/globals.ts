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
  /** KBF spec operations (JSON string in → JSON string out). Synchronous. */
  // eslint-disable-next-line no-var
  var kbf: ((reqJSON: string) => string) | undefined;
  /**
   * Start a directory over: forget the projects at or below it in the
   * workspace and delete its databases. Resolves to null, or to an error
   * message. The host clears the directory's files itself.
   */
  // eslint-disable-next-line no-var
  var kapiReset: ((dir: string) => Promise<string | null>) | undefined;
  /**
   * Read a page of a document's blocks through the change service.
   * `requestJSON` is a serialized ReadRequest, `optionsJSON` the call's
   * options ({project}). Resolves to a serialized ReadPage, or to a
   * kapi.change-result/v1 refusal; rejects only on a failure the contract has
   * no code for.
   */
  // eslint-disable-next-line no-var
  var kapiRead: ((requestJSON: string, optionsJSON?: string) => Promise<string>) | undefined;
  /**
   * Apply a serialized kapi.change/v1 change set through the change service.
   * `optionsJSON` is the call's options ({project, actor}). Resolves to a
   * serialized kapi.change-result/v1, a refusal included.
   */
  // eslint-disable-next-line no-var
  var kapiApply: ((changeSetJSON: string, optionsJSON?: string) => Promise<string>) | undefined;
  /**
   * Describe what a format supports. `requestJSON` is a serialized
   * DescribeRequest. Resolves to a serialized FormatDescription, or to a
   * kapi.change-result/v1 refusal.
   */
  // eslint-disable-next-line no-var
  var kapiDescribe: ((requestJSON: string, optionsJSON?: string) => Promise<string>) | undefined;
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
