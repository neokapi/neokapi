import React from "react";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import { PdfExplorer } from "@site/src/components/Lab";
import styles from "./pdf.module.css";

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
      title="Structure & Layout Lab"
      description="A PDF is just ink on a page until something recovers its reading order, its outline, and the place of every block. Drop one in and watch neokapi reconstruct its structure — privately, in your browser."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Structure &amp; Layout</h1>
          <p className={styles.lede}>
            A document is more than its words — it has a reading order, an outline, and a place for
            every block on the page. Drop in a PDF and neokapi recovers all three, right here in
            your browser. Switch to <strong>Layout</strong> to see each block in its place on the
            page, <strong>Structure</strong> for the outline, and <strong>Blocks</strong> for the
            extracted content — so you can see exactly what it found and where. Nothing is fetched
            until you press Run.
          </p>
        </div>
        <PdfExplorer samples={samples} />
      </main>
    </Layout>
  );
}
