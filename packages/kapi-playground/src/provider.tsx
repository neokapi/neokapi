import React, { createContext, useContext } from "react";

// Framework-agnostic config: the host (Docusaurus adapter, a plain React page,
// Storybook) injects the resolved asset URLs. The core never resolves a base
// URL or imports @docusaurus/useBaseUrl itself.
export interface KapiPlaygroundConfig {
  /** Resolved URL of the Go `wasm_exec.js` glue script. */
  wasmExecUrl: string;
  /** Resolved URL of the kapi-cli wasm (the runtime also probes `${url}.gz`). */
  wasmUrl: string;
}

const ConfigContext = createContext<KapiPlaygroundConfig | null>(null);

export function KapiPlaygroundProvider({
  config,
  children,
}: {
  config: KapiPlaygroundConfig;
  children: React.ReactNode;
}): React.ReactElement {
  return <ConfigContext.Provider value={config}>{children}</ConfigContext.Provider>;
}

/** Read the injected config. Throws if used outside a provider. */
export function useKapiConfig(): KapiPlaygroundConfig {
  const cfg = useContext(ConfigContext);
  if (!cfg) {
    throw new Error("useKapiConfig must be used within a <KapiPlaygroundProvider>.");
  }
  return cfg;
}
