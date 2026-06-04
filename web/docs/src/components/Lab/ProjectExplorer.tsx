import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";

// Docusaurus adapter for the @neokapi/kapi-lab ProjectExplorer — the live
// .kapi project lab (recipe + run a declared flow, contrasted with the
// single-file .klz workspace). Client-only and code-split.

const Loading = (): React.ReactElement => (
  <div
    style={{
      padding: "1rem",
      color: "var(--ifm-color-emphasis-500)",
      fontStyle: "italic",
    }}
  >
    Loading the interactive .kapi project lab…
  </div>
);

const LazyProject = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.ProjectExplorer };
});

export interface ProjectExplorerProps {
  defaultSampleId?: string;
}

export function ProjectExplorer(props: ProjectExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyProject assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}
