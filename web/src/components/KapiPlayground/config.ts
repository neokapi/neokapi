import { useMemo } from "react";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { readCdnConfig, cdnEnabled, cdnHref } from "@neokapi/docs-shared";
import type { KapiPlaygroundConfig } from "@neokapi/kapi-playground";

// Docusaurus adapter: resolve the wasm asset URLs against the site base URL.
// The core kit never resolves these itself (it stays framework-agnostic); the
// host injects them via <KapiPlaygroundProvider config={...}>.
//
// When a CDN origin is configured (cdnBaseUrl customField, from $DOCS_CDN_URL)
// the ~71 MB kapi-cli.wasm + pdfium.wasm are served from the CDN instead of the
// GitHub Pages artifact. The wasm is versioned by path (/wasm/<version>/…) so it
// can be cached immutably; the runtime derives pdfium.wasm and probes
// `${wasmUrl}.gz` relative to the same versioned directory. Empty origin → the
// assets stay same-origin (the default; unchanged local-dev behavior).
//
// The returned object MUST be referentially stable: consumers (useLabRuntime)
// key their boot effect on it, so a fresh object every render would re-run that
// effect forever (setState-in-effect → "Maximum update depth"). The URLs are
// stable strings, so memoize on them.
export function useKapiPlaygroundConfig(): KapiPlaygroundConfig {
  const { siteConfig } = useDocusaurusContext();
  const cdn = readCdnConfig(siteConfig);
  const onCdn = cdnEnabled(cdn);
  const execLocal = useBaseUrl("/wasm/wasm_exec.js");
  const wasmLocal = useBaseUrl("/wasm/kapi-cli.wasm");
  const wasmExecUrl = onCdn ? cdnHref(cdn, `/wasm/${cdn.version}/wasm_exec.js`) : execLocal;
  const wasmUrl = onCdn ? cdnHref(cdn, `/wasm/${cdn.version}/kapi-cli.wasm`) : wasmLocal;
  return useMemo(() => ({ wasmExecUrl, wasmUrl }), [wasmExecUrl, wasmUrl]);
}
