import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";

// Docusaurus adapter for the @neokapi/kapi-lab KbfConformance runner (the KBF
// Tests page). Client-only and code-split like the other lab adapters; it boots
// the kapi WASM in the browser and executes the KBF spec conformance suite
// against both the Go engine and the TypeScript mirror.

const Loading = (): React.ReactElement => (
  <div
    style={{
      padding: "1rem",
      color: "var(--ifm-color-emphasis-500)",
      fontStyle: "italic",
    }}
  >
    Loading the KBF conformance runner…
  </div>
);

const LazyConformance = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.KbfConformance };
});

export function KbfConformance(): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyConformance assets={assets} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}
