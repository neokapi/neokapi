import React, { Suspense } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";
import useBaseUrl from "@docusaurus/useBaseUrl";
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
  /** Scenario preselected in the workspace picker. */
  defaultScenarioId?: string;
  /** Restrict the scenario picker (default: all). */
  scenarioIds?: string[];
  /** Offer the built-in recorded native traces for replay (default: off). */
  withRecordedTraces?: boolean;
  /** App-like mode: the workspace fills the host's height (see kapi-lab). */
  fill?: boolean;
}

// Recorded `kapi run --trace` outputs from native runs — the workspace replays
// them to show what live wasm runs can't: parallel workers and the Java
// bridge's gRPC boundary.
const RECORDED_TRACES = [
  {
    name: "Pseudo-translate JSON",
    description: "Basic native pipeline with 6 Parts",
    path: "/data/traces/pseudo-translate-json.json",
  },
  {
    name: "Multi-tool pipeline",
    description: "Multiple tools, concurrency, buffering",
    path: "/data/traces/multi-tool-pipeline.json",
  },
  {
    name: "Bridge HTML (gRPC boundary)",
    description: "Java/Okapi bridge with gRPC boundary",
    path: "/data/traces/bridge-html-pseudo.json",
  },
  {
    name: "AI Translate (parallel workers)",
    description: "Parallel block processing with 3 concurrent workers",
    path: "/data/traces/ai-translate-parallel.json",
  },
  {
    name: "Translate + QA (parallel)",
    description: "Two parallel stages: AI translate then QA check",
    path: "/data/traces/translate-qa-parallel.json",
  },
];

export function FlowBuilderRunner({
  withRecordedTraces,
  ...props
}: FlowBuilderRunnerProps): React.ReactElement {
  return (
    <BrowserOnly fallback={<Loading />}>
      {() => {
        // useBaseUrl (inside useKapiPlaygroundConfig) must run in a component.
        function Inner(): React.ReactElement {
          const assets = useKapiPlaygroundConfig();
          const tracesBase = useBaseUrl("/data/traces/");
          const recordedTraces = withRecordedTraces
            ? RECORDED_TRACES.map((t) => ({
                name: t.name,
                description: t.description,
                url: tracesBase + t.path.split("/").pop(),
              }))
            : undefined;
          return (
            <Suspense fallback={<Loading />}>
              <LazyFlowBuilderRunner assets={assets} recordedTraces={recordedTraces} {...props} />
            </Suspense>
          );
        }
        return <Inner />;
      }}
    </BrowserOnly>
  );
}
