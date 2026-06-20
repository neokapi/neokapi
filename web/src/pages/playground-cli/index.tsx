import React from "react";
import Layout from "@theme/Layout";
import KapiPlaygroundExplorer from "@site/src/components/KapiPlayground/KapiPlaygroundExplorer";

export default function CliPlaygroundPage(): React.ReactElement {
  return (
    <Layout
      title="CLI Playground"
      description="Run the kapi command-line tool in your browser. It operates on an in-memory project folder — pick a sample, run commands, download results, no server."
    >
      <main className="container margin-vert--lg">
        <h1>CLI Playground</h1>
        <p>
          The <code>kapi</code> command-line tool, running entirely in your browser. It works
          against an in-memory project folder. Pick a sample to play with — a single{" "}
          <strong>loose file</strong> for one-off commands, or a ready-made{" "}
          <strong>.kapi sample project</strong> to run the offline funnel (add, extract, run,
          merge). Or upload your own files and run any command. Nothing leaves your machine.
        </p>
        <KapiPlaygroundExplorer />
      </main>
    </Layout>
  );
}
