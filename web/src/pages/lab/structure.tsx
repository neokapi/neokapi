import React from "react";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import { PdfExplorer } from "@site/src/components/Lab";
import { LabLaunch } from "@site/src/components/Lab/LabLaunch";
import { LabPageShell } from "@site/src/components/Lab/LabPageShell";

// Structure & Layout lab: how the engine recovers a document's *shape* — the
// reading order, the outline, and per-block page geometry. PDF is the richest
// browser-runnable case: PDFium (WebAssembly) extracts text + geometry, bridged
// into the engine's wasm PDF reader (the same PDFium the native kapi-pdfium
// plugin uses), and the shared DocumentViewer renders Layout / Structure /
// Blocks. Nothing is mocked. Press Run to load the engine and the pdfium plugin.

export default function StructureLabPage(): React.ReactElement {
  // Bundled samples with real structure (headings + tables) so the Structure and
  // Layout tabs have something to recover; anatomy.pdf is the minimal sample.
  const samples = [
    { url: useBaseUrl("/samples/report.pdf"), name: "report.pdf" },
    { url: useBaseUrl("/samples/invoice.pdf"), name: "invoice.pdf" },
    { url: useBaseUrl("/samples/anatomy.pdf"), name: "anatomy.pdf" },
  ];
  return (
    <Layout
      title="Structure and layout"
      description="Examine the reading order, outline and page geometry recovered from a PDF."
    >
      <LabPageShell
        title="Structure and layout"
        lede={
          <>
            Compare a PDF with its extracted reading order, outline and block positions. Open
            <strong> Layout</strong> to inspect page geometry, <strong>Structure</strong> for the
            inferred outline, and <strong>Blocks</strong> for the content. Check the recovered order
            against the original page, especially around tables and columns.
          </>
        }
      >
        <LabLaunch description="Open the workspace, then run the sample to load the engine and PDF reader. The first run downloads these assets. This browser experiment uses the Go engine with a browser PDF bridge.">
          <PdfExplorer samples={samples} />
        </LabLaunch>
      </LabPageShell>
    </Layout>
  );
}
