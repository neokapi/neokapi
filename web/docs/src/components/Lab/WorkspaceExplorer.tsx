import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";

// Docusaurus adapter for the @neokapi/kapi-lab WorkspaceExplorer — the live
// .klz workspace lab (extract → transform → pack → merge, inspecting the
// recipe / overlays / dirty state). Client-only (the WASM engine boots in the
// browser) and code-split so the lab chunk loads only where embedded.

const Loading = (): React.ReactElement => (
  <div
    style={{
      padding: "1rem",
      color: "var(--ifm-color-emphasis-500)",
      fontStyle: "italic",
    }}
  >
    Loading the interactive .klz workspace lab…
  </div>
);

const LazyWorkspace = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.WorkspaceExplorer };
});

export interface WorkspaceExplorerProps {
  defaultSampleId?: string;
}

export function WorkspaceExplorer(props: WorkspaceExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyWorkspace assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}
