import { useMemo } from "react";
import useBaseUrl from "@docusaurus/useBaseUrl";
import type { KapiPlaygroundConfig } from "@neokapi/kapi-playground";

// Docusaurus adapter: resolve the wasm asset URLs against the site base URL.
// The core kit never resolves these itself (it stays framework-agnostic); the
// host injects them via <KapiPlaygroundProvider config={...}>.
//
// The returned object MUST be referentially stable: consumers (useLabRuntime)
// key their boot effect on it, so a fresh object every render would re-run that
// effect forever (setState-in-effect → "Maximum update depth"). The URLs are
// stable strings, so memoize on them.
export function useKapiPlaygroundConfig(): KapiPlaygroundConfig {
  const wasmExecUrl = useBaseUrl("/wasm/wasm_exec.js");
  const wasmUrl = useBaseUrl("/wasm/kapi-cli.wasm");
  return useMemo(() => ({ wasmExecUrl, wasmUrl }), [wasmExecUrl, wasmUrl]);
}
