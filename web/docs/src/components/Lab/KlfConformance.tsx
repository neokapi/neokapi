import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";

// Docusaurus adapter for the @neokapi/kapi-lab KlfConformance runner (the KLF
// Tests page). Client-only and code-split like the other lab adapters; it boots
// the kapi WASM in the browser and executes the KLF spec conformance suite
// against both the Go engine and the TypeScript mirror.

const Loading = (): React.ReactElement => (
  <div
    style={{
      padding: "1rem",
      color: "var(--ifm-color-emphasis-500)",
      fontStyle: "italic",
    }}
  >
    Loading the KLF conformance runner…
  </div>
);

const LazyConformance = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.KlfConformance };
});

export function KlfConformance(): React.ReactElement {
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
