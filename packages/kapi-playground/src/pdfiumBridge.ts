// PDFium-wasm bridge — the browser counterpart to the native kapi-pdfium plugin.
//
// The kapi wasm engine's browser PDF reader (core/formats/pdf/wasm_bridge.go)
// can't run PDFium's cgo build, but PDFium *compiled to wasm* runs directly in
// the browser. This module loads that wasm (via @embedpdf/pdfium, which ships a
// growable-heap build and exposes the low-level FPDFText_* APIs) and installs
// `globalThis.__kapiPdfium`, the contract the Go reader calls:
//
//   globalThis.__kapiPdfium = {
//     ready: Promise<void>,                    // resolves once pdfium.wasm is up
//     extract(bytes): Promise<{ pages: { number, height,
//       rects: { text, l, t, r, b }[] }[] }>,  // bottom-left coords (PDF points)
//   }
//
// Loading is lazy: the 4.6 MB pdfium.wasm is fetched only the first time a PDF is
// actually inspected, so non-PDF lab use pays nothing. The .wasm is self-hosted
// (passed as `wasmBinary`, sidestepping streaming-compile / Content-Type
// requirements) and the @embedpdf build is single-threaded, so no COOP/COEP
// cross-origin-isolation headers are needed.

// Minimal structural typing of the @embedpdf/pdfium module surface we use. The
// FPDF* C functions sit at the top level; Emscripten runtime helpers live under
// `.pdfium`.
interface PdfiumModule {
  pdfium: {
    wasmExports: { malloc(n: number): number; free(p: number): void };
    HEAPU8: Uint8Array;
    getValue(ptr: number, type: "double"): number;
    UTF16ToString(ptr: number): string;
  };
  PDFiumExt_Init(): void;
  FPDF_LoadMemDocument(buf: number, size: number, password: string): number;
  FPDF_GetPageCount(doc: number): number;
  FPDF_GetPageSizeByIndex(doc: number, index: number, wOut: number, hOut: number): number;
  FPDF_LoadPage(doc: number, index: number): number;
  FPDF_ClosePage(page: number): void;
  FPDF_CloseDocument(doc: number): void;
  FPDFText_LoadPage(page: number): number;
  FPDFText_ClosePage(textPage: number): void;
  FPDFText_CountRects(textPage: number, start: number, count: number): number;
  FPDFText_GetRect(
    textPage: number,
    index: number,
    l: number,
    t: number,
    r: number,
    b: number,
  ): void;
  FPDFText_GetBoundedText(
    textPage: number,
    l: number,
    t: number,
    r: number,
    b: number,
    out: number,
    outLen: number,
  ): number;
}

export interface PdfRect {
  text: string;
  l: number;
  t: number;
  r: number;
  b: number;
}
export interface PdfPage {
  number: number;
  height: number;
  rects: PdfRect[];
}
export interface PdfExtract {
  pages: PdfPage[];
}

export interface KapiPdfium {
  ready: Promise<void>;
  extract(bytes: Uint8Array): Promise<PdfExtract>;
}

async function loadModule(wasmUrl: string): Promise<PdfiumModule> {
  // Dynamic import so @embedpdf/pdfium (and its wasm glue) is only pulled into
  // the bundle/chunk that actually parses a PDF.
  const mod = (await import("@embedpdf/pdfium")) as unknown as {
    init(opts: { wasmBinary: ArrayBuffer }): Promise<PdfiumModule>;
  };
  const resp = await fetch(wasmUrl);
  if (!resp.ok) throw new Error(`pdfium: failed to fetch ${wasmUrl} (${resp.status})`);
  const wasmBinary = await resp.arrayBuffer();
  const m = await mod.init({ wasmBinary });
  m.PDFiumExt_Init(); // required before any document op
  return m;
}

function extractDocument(m: PdfiumModule, bytes: Uint8Array): PdfExtract {
  const rt = m.pdfium;
  // Arrow wrappers (not destructured refs) so the unbound-method lint is happy;
  // these are plain wasm exports with no `this`.
  const malloc = (n: number) => rt.wasmExports.malloc(n);
  const free = (p: number) => rt.wasmExports.free(p);

  const buf = malloc(bytes.length);
  rt.HEAPU8.set(bytes, buf);
  const doc = m.FPDF_LoadMemDocument(buf, bytes.length, "");
  if (!doc) {
    free(buf);
    throw new Error("pdfium: failed to load PDF (corrupt or password-protected)");
  }

  // Reused scratch out-params: 4 doubles for a rect, 2 for page size.
  const dL = malloc(8);
  const dT = malloc(8);
  const dR = malloc(8);
  const dB = malloc(8);
  const dW = malloc(8);
  const dH = malloc(8);
  const pages: PdfPage[] = [];

  try {
    const pageCount = m.FPDF_GetPageCount(doc);
    for (let i = 0; i < pageCount; i++) {
      m.FPDF_GetPageSizeByIndex(doc, i, dW, dH);
      const height = rt.getValue(dH, "double");

      const page = m.FPDF_LoadPage(doc, i);
      const textPage = m.FPDFText_LoadPage(page);
      const rects: PdfRect[] = [];

      const count = m.FPDFText_CountRects(textPage, 0, -1);
      for (let r = 0; r < count; r++) {
        m.FPDFText_GetRect(textPage, r, dL, dT, dR, dB);
        const l = rt.getValue(dL, "double");
        const t = rt.getValue(dT, "double"); // top = larger Y (bottom-left origin)
        const rr = rt.getValue(dR, "double");
        const b = rt.getValue(dB, "double");

        // FPDFText_GetBoundedText returns the char count; first call (out=0)
        // sizes the buffer, then we copy UTF-16LE and decode.
        const need = m.FPDFText_GetBoundedText(textPage, l, t, rr, b, 0, 0);
        let text = "";
        if (need > 0) {
          const tbuf = malloc((need + 1) * 2);
          m.FPDFText_GetBoundedText(textPage, l, t, rr, b, tbuf, need);
          text = rt.UTF16ToString(tbuf).slice(0, need);
          free(tbuf);
        }
        rects.push({ text, l, t, r: rr, b });
      }

      m.FPDFText_ClosePage(textPage);
      m.FPDF_ClosePage(page);
      pages.push({ number: i + 1, height, rects });
    }
  } finally {
    for (const p of [dL, dT, dR, dB, dW, dH]) free(p);
    m.FPDF_CloseDocument(doc);
    free(buf);
  }
  return { pages };
}

/**
 * Install `globalThis.__kapiPdfium` so the kapi wasm engine's browser PDF reader
 * can extract text + geometry. Idempotent; the pdfium.wasm at `wasmUrl` is
 * fetched lazily on first use. `wasmUrl` should point at a self-hosted copy of
 * @embedpdf/pdfium's dist/pdfium.wasm (see the Makefile web-wasm-cli target).
 */
export function installPdfiumBridge(wasmUrl: string): void {
  const g = globalThis as unknown as { __kapiPdfium?: KapiPdfium };
  if (g.__kapiPdfium) return;

  let modPromise: Promise<PdfiumModule> | null = null;
  const ensure = () => (modPromise ??= loadModule(wasmUrl));

  g.__kapiPdfium = {
    // A getter keeps loading lazy: awaiting `ready` (the Go reader does) starts
    // the fetch; never touched → never fetched.
    get ready() {
      return ensure().then(() => undefined);
    },
    extract: (bytes: Uint8Array) => ensure().then((m) => extractDocument(m, bytes)),
  } as KapiPdfium;
}
