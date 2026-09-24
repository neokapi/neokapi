import React from "react";
import Layout from "@theme/Layout";
import { ConversionExplorer } from "@site/src/components/Lab";
import { LabLaunch } from "@site/src/components/Lab/LabLaunch";
import { LabPageShell } from "@site/src/components/Lab/LabPageShell";

// The Conversion Lab: read a document in one format, re-express it in another —
// the real kapi `kconv` running in the browser via WebAssembly. The
// target list is restricted to GENERATIVE writers (those that reconstruct a
// whole document from the content model); skeleton-driven formats (docx/odt/
// idml/epub) inject into an original file and so cannot be conversion targets.
// Nothing boots until the reader presses Run (the explorer's shared gate).

export default function ConversionLabPage(): React.ReactElement {
  return (
    <Layout
      title="Conversion lab"
      description="Compare document formats and inspect which structures each output can represent."
    >
      <LabPageShell
        title="Conversion lab"
        lede={
          <>
            Read a sample into the content model and compare its representations in different output
            formats. Inspect headings, lists and inline content in both the rendered output and its
            source. Each destination format has different expressive limits: identify what survives,
            what changes, and what requires a format-aware round trip through the original file.
          </>
        }
      >
        <LabLaunch>
          <ConversionExplorer defaultSampleId="article-md" defaultTarget="doclang" />
        </LabLaunch>
      </LabPageShell>
    </Layout>
  );
}
