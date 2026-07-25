import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";

// Docusaurus adapter for the @neokapi/kapi-lab KbfExplorer (the KBF Lab). Like
// the other lab adapters it is client-only (the WASM runtime boots in the
// browser) and code-split (React.lazy) so the heavy lab chunk loads only on
// pages that embed it. Asset URLs are resolved against the site base URL via
// the shared playground config.

const Loading = (): React.ReactElement => (
  <div
    style={{
      padding: "1rem",
      color: "var(--ifm-color-emphasis-500)",
      fontStyle: "italic",
    }}
  >
    Loading the interactive KBF lab…
  </div>
);

const LazyKbf = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.KbfExplorer };
});

export interface KbfExplorerProps {
  defaultSampleId?: string;
  hideAnnotations?: boolean;
}

export function KbfExplorer(props: KbfExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        // useBaseUrl (inside useKapiPlaygroundConfig) must run in a component.
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyKbf assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}
