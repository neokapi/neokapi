import React from "react";
import Layout from "@theme/Layout";
import { FlowBuilderRunner } from "@site/src/components/Lab/FlowBuilderRunner";
import { LabPageShell } from "@site/src/components/Lab/LabPageShell";
import { LabLaunch } from "@site/src/components/Lab/LabLaunch";

export default function LabPage(): React.ReactElement {
  return (
    <Layout title="Flow workspace" description="Build and inspect processing flows in the browser.">
      <LabPageShell
        title="Flow workspace"
        maxWidthClassName="max-w-[1500px]"
        lede="Select a scenario, inspect its tools, and run the flow on a sample."
      >
        <p>
          Live runs use the browser build of kapi. Ordinary AI provider selections use deterministic
          demonstration responses. Recorded native traces show concurrency and provider-backed
          examples from previous runs.
        </p>
        <LabLaunch label="Open flow workspace">
          <FlowBuilderRunner defaultScenarioId="annotations" withRecordedTraces />
        </LabLaunch>
      </LabPageShell>
    </Layout>
  );
}
