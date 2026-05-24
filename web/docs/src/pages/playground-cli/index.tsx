import React from "react";
import Layout from "@theme/Layout";
import KapiEmbedFullBleed from "@site/src/components/KapiPlayground/KapiEmbedFullBleed";

export default function CliPlaygroundPage(): React.ReactElement {
  return (
    <Layout
      title="CLI Playground"
      description="Run the kapi command-line tool in your browser. The CLI is compiled to WebAssembly and operates on an in-memory project folder — upload files, run commands, download results, no server."
    >
      <main className="container margin-vert--lg">
        <h1>CLI Playground</h1>
        <p>
          The <code>kapi</code> command-line tool, compiled to WebAssembly and running entirely in
          your browser. It works against an in-memory project folder: upload files into it, run
          commands in the terminal, and download the results. Nothing leaves your machine.
        </p>
        <KapiEmbedFullBleed />
      </main>
    </Layout>
  );
}
