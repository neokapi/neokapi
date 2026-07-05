// The boot/runtime facade lives in @neokapi/engine (packages/engine) — the
// publishable, dependency-light wrapper around the kapi WASM engine. This
// module keeps the playground's historical public surface
// (`@neokapi/kapi-playground/runtime`) stable and layers the one
// playground-owned concern onto boot: installing the PDFium reverse bridge
// (pdfiumBridge.ts pulls @embedpdf/pdfium, which must not become an engine
// dependency).

export * from "@neokapi/engine/runtime";

import { bootKapiRuntime as bootEngineRuntime } from "@neokapi/engine/runtime";
import type { KapiRuntime } from "@neokapi/engine/runtime";
import { installPdfiumBridge } from "./pdfiumBridge";

let booting: Promise<KapiRuntime> | null = null;

/**
 * Boot the kapi CLI wasm once and return the shared runtime (see
 * @neokapi/engine). On top of the engine boot, installs the browser
 * PDFium-wasm bridge so the engine's wasm PDF reader can extract text +
 * geometry. Lazy: pdfium.wasm (next to the kapi wasm) is only fetched the
 * first time a PDF is inspected. A failure here must never break boot —
 * non-PDF use is unaffected.
 */
export function bootKapiRuntime(wasmExecUrl: string, wasmUrl: string): Promise<KapiRuntime> {
  booting ??= bootEngineRuntime(wasmExecUrl, wasmUrl).then((rt) => {
    try {
      installPdfiumBridge(wasmUrl.replace(/[^/]*$/, "pdfium.wasm"));
    } catch {
      /* bridge is best-effort; PDF inspection will surface a clear error */
    }
    return rt;
  });
  return booting;
}
