import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";

// Docusaurus adapter for the @neokapi/kapi-lab FlowBuilderRunner explorer.
// Like the other Lab adapters it is client-only (the WASM runtime boots in the
// browser) and code-split (React.lazy) so the heavy lab + flow-editor chunk
// loads only on the page that embeds it. Asset URLs resolve against the site
// base URL via the shared playground config. Kept in its own file so this
// later-added explorer does not have to edit Lab/index.tsx.

const Loading = (): React.ReactElement => (
  <div style={{ padding: "1rem", color: "var(--ifm-color-emphasis-500)", fontStyle: "italic" }}>
    Loading the interactive lab…
  </div>
);

const LazyFlowBuilderRunner = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.FlowBuilderRunner };
});

export interface FlowBuilderRunnerProps {
  defaultSampleId?: string;
  sampleIds?: string[];
}

export function FlowBuilderRunner(props: FlowBuilderRunnerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        // useBaseUrl (inside useKapiPlaygroundConfig) must run in a component.
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyFlowBuilderRunner assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}
