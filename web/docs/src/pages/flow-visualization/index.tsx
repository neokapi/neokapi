import React from "react";
import Layout from "@theme/Layout";
import { Play } from "lucide-react";
// SSR-clean event bus only (no xterm/wasm) — the shared modal (mounted in
// Root.tsx) code-splits the heavy runtime in when first opened.
import { openKapi } from "@neokapi/kapi-playground/store";
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
        <p>
          This page replays a recorded trace. To drive the real pipeline yourself, run a flow in an
          in-browser terminal &mdash; the actual <code>kapi</code> binary, compiled to WebAssembly,
          no install or server.
        </p>
        <p>
          <button
            type="button"
            className="button button--primary"
            onClick={() =>
              openKapi({
                cmd: "kapi pseudo-translate messages.json",
                seed: ["messages.json"],
                autoRun: true,
              })
            }
          >
            <Play size={16} aria-hidden="true" fill="currentColor" style={{ marginRight: 6 }} />
            Try it live
          </button>
        </p>
        <FlowVisualization />
      </main>
    </Layout>
  );
}
