import React from "react";
import Layout from "@theme/Layout";
import BrowserOnly from "@docusaurus/BrowserOnly";

export default function CliPlaygroundPage(): React.ReactElement {
  return (
    <Layout
      title="CLI Playground"
      description="Run the kapi command-line tool in your browser. The CLI is compiled to WebAssembly and operates on an in-memory project folder — upload files, run commands, download results, no server."
    >
      <main className="container margin-vert--lg">
        <h1>CLI Playground</h1>
        <p>
          The <code>kapi</code> command-line tool, compiled to WebAssembly and
          running entirely in your browser. It works against an in-memory
          project folder: upload files into it, run commands in the terminal,
          and download the results. Nothing leaves your machine.
        </p>
        <BrowserOnly fallback={<p>Loading the in-browser terminal…</p>}>
          {() => {
            const Playground = require("./_Playground").default;
            return <Playground />;
          }}
        </BrowserOnly>
      </main>
    </Layout>
  );
}
