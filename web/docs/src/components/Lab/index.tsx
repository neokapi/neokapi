import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { useKapiPlaygroundConfig } from "../KapiPlayground/config";

// Docusaurus adapters for the @neokapi/kapi-lab explorers. Each is client-only
// (the WASM runtime boots in the browser) and code-split (React.lazy) so the
// heavy lab chunk loads only on pages that embed an explorer. Asset URLs are
// resolved against the site base URL via the shared playground config.

const Loading = (): React.ReactElement => (
  <div style={{ padding: "1rem", color: "var(--ifm-color-emphasis-500)", fontStyle: "italic" }}>
    Loading the interactive lab…
  </div>
);

const LazyAnatomy = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.AnatomyExplorer };
});

const LazyPipeline = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.PipelineExplorer };
});

const LazyBatch = React.lazy(async () => {
  const mod = await import("@neokapi/kapi-lab");
  return { default: mod.BatchExplorer };
});

export interface AnatomyExplorerProps {
  defaultSampleId?: string;
  sampleIds?: string[];
}

export function AnatomyExplorer(props: AnatomyExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        // useBaseUrl (inside useKapiPlaygroundConfig) must run in a component.
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyAnatomy assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

export interface PipelineExplorerProps {
  defaultSampleId?: string;
  defaultPipelineId?: string;
  sampleIds?: string[];
}

export function PipelineExplorer(props: PipelineExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyPipeline assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}

export interface BatchExplorerProps {
  sampleIds?: string[];
  defaultPattern?: string;
}

export function BatchExplorer(props: BatchExplorerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          return (
            <Suspense fallback={<Loading />}>
              <LazyBatch assets={assets} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}
