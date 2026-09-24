import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import KapiPlaygroundExplorer from "@site/src/components/KapiPlayground/KapiPlaygroundExplorer";
import { LabLaunch } from "@site/src/components/Lab/LabLaunch";
import { LabPageShell } from "@site/src/components/Lab/LabPageShell";

export default function CliPlaygroundPage(): React.ReactElement {
  return (
    <Layout
      title="CLI playground"
      description="Explore supported kapi commands in an in-browser terminal with sample files and projects."
    >
      <LabPageShell
        title="CLI playground"
        lede={
          <>
            Open a sample and run its suggested command. The terminal uses the browser build of kapi
            with an in-memory filesystem. Follow the <Link to="/labs">labs learning path</Link> for
            guided exercises.
          </>
        }
      >
        <p>
          Supported file and project operations run locally in your browser. The full workspace
          context graph requires native kapi. Ordinary AI provider selections use deterministic
          demonstration responses; explicitly selected browser providers depend on browser support
          and model availability.
        </p>
        <LabLaunch
          label="Open terminal"
          description="Opening the terminal downloads and starts the browser engine. Select a sample, then press Enter to run the suggested command. Download any files you want to keep before reloading the page."
        >
          <KapiPlaygroundExplorer />
        </LabLaunch>
      </LabPageShell>
    </Layout>
  );
}
