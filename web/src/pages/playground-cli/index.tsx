import React from "react";
import Layout from "@theme/Layout";
import KapiPlaygroundExplorer from "@site/src/components/KapiPlayground/KapiPlaygroundExplorer";

export default function CliPlaygroundPage(): React.ReactElement {
  return (
    <Layout
      title="CLI Playground"
      description="Try the kapi command-line tool hands-on — on a sample project or your own files — without installing anything. Everything runs in your browser; nothing leaves your machine."
    >
      <main className="container margin-vert--lg">
        <h1>CLI Playground</h1>
        <p>
          Try the <code>kapi</code> command-line tool hands-on, without installing anything — it
          runs entirely in your browser. Start from a sample, or upload your own files and run any
          command to see what kapi does with them. Nothing leaves your machine.
        </p>
        <KapiPlaygroundExplorer />
      </main>
    </Layout>
  );
}
