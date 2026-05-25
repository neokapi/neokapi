import useBaseUrl from "@docusaurus/useBaseUrl";
import type { KapiPlaygroundConfig } from "@neokapi/kapi-playground";

// Docusaurus adapter: resolve the wasm asset URLs against the site base URL.
// The core kit never resolves these itself (it stays framework-agnostic); the
// host injects them via <KapiPlaygroundProvider config={...}>.
export function useKapiPlaygroundConfig(): KapiPlaygroundConfig {
  const wasmExecUrl = useBaseUrl("/wasm/wasm_exec.js");
  const wasmUrl = useBaseUrl("/wasm/kapi-cli.wasm");
  return { wasmExecUrl, wasmUrl };
}
