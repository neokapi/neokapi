import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";

// Docusaurus adapter for the @neokapi/kapi-lab ToolLab explorer. Like the other
// Lab adapters it is client-only (the WASM runtime boots in the browser) and
// code-split (React.lazy) so the heavy lab chunk loads only on pages that embed
// it. Asset URLs are resolved against the site base URL via the shared
// playground config. Kept in its own module (rather than Lab/index.tsx) so the
// integrator can wire the kapi-lab barrel export without touching index.tsx.

const Loading = (): React.ReactElement => (
  <div style={{ padding: "1rem", color: "var(--ifm-color-emphasis-500)", fontStyle: "italic" }}>
    Loading the interactive lab…
  </div>
);

const LazyToolLab = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.ToolLab };
});

export interface ToolLabProps {
  defaultSampleId?: string;
  sampleIds?: string[];
}

export function ToolLab(props: ToolLabProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        // useBaseUrl (inside useKapiPlaygroundConfig) must run in a component.
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyToolLab assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}
