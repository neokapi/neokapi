import React from "react";
import Layout from "@theme/Layout";
import BrowserOnly from "@docusaurus/BrowserOnly";

export default function PlaygroundPage(): React.ReactElement {
  return (
    <Layout
      title="Playground"
      description="Run neokapi formats and tools directly in your browser, compiled to WebAssembly. Upload a document and see it pseudo-translated client-side."
    >
      <main className="container margin-vert--lg">
        <h1>Playground</h1>
        <p>
          This page runs the neokapi engine entirely in your browser, compiled
          to WebAssembly &mdash; no server, no upload. Drop in a document (a Word{" "}
          <code>.docx</code>, or any supported text format) and see it
          pseudo-translated on the spot, then download the localized file.
        </p>
        <BrowserOnly fallback={<p>Loading the in-browser engine&hellip;</p>}>
          {() => {
            const PseudoPlayground = require("./_PseudoPlayground").default;
            return <PseudoPlayground />;
          }}
        </BrowserOnly>
      </main>
    </Layout>
  );
}
