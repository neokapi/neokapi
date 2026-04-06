import React from "react";
import Layout from "@theme/Layout";
import FlowVisualization from "./_FlowVisualization";

export default function FlowVisualizationPage(): React.ReactElement {
  return (
    <Layout
      title="Flow Visualization"
      description="Interactive visualization of neokapi's channel-based concurrent pipeline"
    >
      <main className="container margin-vert--lg">
        <h1>Flow Visualization</h1>
        <p>
          Watch how Parts flow through neokapi&apos;s concurrent processing pipeline in real-time.
        </p>
        <FlowVisualization />
      </main>
    </Layout>
  );
}
