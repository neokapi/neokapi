import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";

// Docusaurus adapter for the kapi-lab Script Lab: client-only (Monaco + the
// WASM runtime boot in the browser) and code-split so the heavy editor chunk
// loads only on pages that embed it.

const Loading = (): React.ReactElement => (
  <div style={{ padding: "1rem", color: "var(--ifm-color-emphasis-500)", fontStyle: "italic" }}>
    Loading the script lab…
  </div>
);

const LazyScriptLab = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.ScriptLab };
});

export interface ScriptLabProps {
  defaultSampleId?: string;
  sampleIds?: string[];
}

export function ScriptLab(props: ScriptLabProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyScriptLab assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}
