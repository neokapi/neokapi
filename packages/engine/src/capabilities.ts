// Reverse-bridge capabilities — the optional globals a host page MAY provide
// for the engine to call back into.
//
// The wasm engine probes each of these per use and degrades with an actionable
// error (or a documented fallback) when one is absent, so a page installs only
// what it needs. This module is typing + documentation: it changes no
// behavior. Reference implementations live in @neokapi/kapi-playground
// (pdfiumBridge.ts, localLlmBridge.ts), @neokapi/kapi-lab (localNer.ts), and
// the docs site (browserTranslate.ts). The Go side of each contract is
// documented in kapi/cmd/kapi-wasm-cli (pdf wasm_bridge, local_bridge,
// ner_bridge, browser_mt).

// ── PDF text + geometry (`globalThis.__kapiPdfium`) ─────────────────────────

/** One character box inside a {@link PdfRect} (bottom-left coords, PDF points). */
export interface PdfGlyph {
  text: string;
  l: number;
  t: number;
  r: number;
  b: number;
}

/** One text rectangle on a PDF page (bottom-left coords, PDF points). */
export interface PdfRect {
  text: string;
  l: number;
  t: number;
  r: number;
  b: number;
  /** Per-character boxes inside this rect (bottom-left coords, like the rect). */
  glyphs?: PdfGlyph[];
}

/** One PDF page: 1-based number, height in points, and its text rects. */
export interface PdfPage {
  number: number;
  height: number;
  rects: PdfRect[];
}

/** Result of {@link KapiPdfium.extract} — all pages of a document. */
export interface PdfExtract {
  pages: PdfPage[];
}

/**
 * The PDF bridge the engine's browser PDF reader
 * (core/formats/pdf/wasm_bridge.go) calls. Install as
 * `globalThis.__kapiPdfium`; without it, inspecting a PDF fails with a clear
 * error while every other format keeps working. Hosts typically implement it
 * with a PDFium-wasm build loaded lazily on first use.
 */
export interface KapiPdfium {
  /** Resolves once the underlying PDF engine (e.g. pdfium.wasm) is up. */
  ready: Promise<void>;
  /** Extract text + geometry from a whole PDF document. */
  extract(bytes: Uint8Array): Promise<PdfExtract>;
}

// ── On-device LLM (`globalThis.kapiLocalGenerate`) ──────────────────────────

/** One chat message in a {@link LocalGeneratePayload}. */
export interface LocalGenerateMessage {
  role: string;
  text: string;
}

/** JSON payload passed (serialized) to {@link KapiLocalGenerate}. */
export interface LocalGeneratePayload {
  messages: LocalGenerateMessage[];
  model: string;
  max_tokens: number;
  temperature: number;
  /** JSON schema for structured generation, when the tool requests it. */
  schema?: unknown;
}

/** Structured result form of {@link KapiLocalGenerate}. */
export interface LocalGenerateResult {
  text: string;
  input_tokens?: number;
  output_tokens?: number;
}

/**
 * The on-device LLM bridge behind the browser build's "local" AI provider
 * (`--provider local`). Install as `globalThis.kapiLocalGenerate`; without it,
 * the provider errors actionably and the demo provider remains the fallback.
 * `payloadJSON` is a serialized {@link LocalGeneratePayload}.
 */
export type KapiLocalGenerate = (payloadJSON: string) => Promise<string | LocalGenerateResult>;

// ── On-device NER (`globalThis.kapiLocalNER`) ───────────────────────────────

/** Request shape (serialized as `reqJSON`) for {@link KapiLocalNER}. */
export interface LocalNERRequest {
  text: string;
  /** BCP-47 locale of the text. */
  locale: string;
}

/** One detected entity in a {@link LocalNERResponse}. */
export interface LocalNEREntity {
  text: string;
  /** Bare category: person, organization, location, date, … */
  type: string;
  confidence: number;
  offset: number;
  length: number;
}

/** Response shape (returned serialized as `respJSON`) for {@link KapiLocalNER}. */
export interface LocalNERResponse {
  entities: LocalNEREntity[];
}

/**
 * The on-device NER bridge behind `entity-extract` with `engine: ner` in the
 * browser. Install as `globalThis.kapiLocalNER`; without it, detection fails
 * with an actionable error rather than silently returning nothing. Both sides
 * of the call are JSON strings ({@link LocalNERRequest} → {@link LocalNERResponse}).
 */
export type KapiLocalNER = (reqJSON: string) => Promise<string>;

// ── Browser machine translation (`globalThis.kapiBrowserTranslate`) ─────────

/** JSON payload passed (serialized) to {@link KapiBrowserTranslate}. */
export interface BrowserTranslatePayload {
  text: string;
  /** Base language tags ("fr", not "fr-FR") — what the Translator API expects. */
  sourceLang: string;
  targetLang: string;
}

/** Result of {@link KapiBrowserTranslate}. */
export interface BrowserTranslateResult {
  ok: boolean;
  translation?: string;
  error?: string;
}

/**
 * The MT bridge behind the browser build's `browser` MT provider. Install as
 * `globalThis.kapiBrowserTranslate`; the engine uses it only when the platform
 * also exposes the built-in `Translator` API (Chrome 138+ desktop), and falls
 * back to the keyless demo provider otherwise.
 */
export type KapiBrowserTranslate = (payloadJSON: string) => Promise<BrowserTranslateResult>;

// ── Aggregate ────────────────────────────────────────────────────────────────

/**
 * All reverse-bridge globals a host page may provide, as one typed object.
 * Every member is optional — install only what the page needs.
 */
export interface KapiEngineCapabilities {
  __kapiPdfium?: KapiPdfium;
  kapiLocalGenerate?: KapiLocalGenerate;
  kapiLocalNER?: KapiLocalNER;
  kapiBrowserTranslate?: KapiBrowserTranslate;
}

/** Which capabilities the current page provides (a read-only probe). */
export interface CapabilityStatus {
  pdf: boolean;
  localLLM: boolean;
  localNER: boolean;
  /** True only when both the bridge and the platform Translator API exist. */
  browserMT: boolean;
}

/**
 * Probe the host-provided reverse bridges, mirroring the checks the engine
 * itself performs per call. Purely observational — installs nothing.
 */
export function detectCapabilities(): CapabilityStatus {
  const g = globalThis as typeof globalThis & KapiEngineCapabilities;
  return {
    pdf: typeof g.__kapiPdfium === "object" && g.__kapiPdfium !== null,
    localLLM: typeof g.kapiLocalGenerate === "function",
    localNER: typeof g.kapiLocalNER === "function",
    browserMT:
      typeof g.kapiBrowserTranslate === "function" &&
      Boolean((globalThis as Record<string, unknown>).Translator),
  };
}
