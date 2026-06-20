import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
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
      description="See how the kapi engine recovers a document's structure and page geometry in your browser — PDF text and per-block layout extracted via PDFium (WebAssembly), shown as Layout, Structure, and Blocks. Nothing is mocked."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Structure &amp; Layout</h1>
          <p className={styles.lede}>
            A document is more than its words — it has a reading order, an outline, and a place for
            every block on the page. Drop in a PDF and the <Link to="/lab">Core Framework</Link>{" "}
            engine, compiled to WebAssembly, recovers all three right here in your browser. Text and
            per-block geometry come from <strong>PDFium</strong> (also WebAssembly), the same engine
            the native <code>kapi-pdfium</code> plugin uses on the desktop. Switch to{" "}
            <strong>Layout</strong> to see each block in its place on the page,{" "}
            <strong>Structure</strong> for the outline, and <strong>Blocks</strong> for the
            extracted content. Press Run to load the engine and the <code>pdfium</code> plugin —
            nothing is fetched until you do.
          </p>
          <nav className={styles.nav} aria-label="Related labs">
            <Link to="/lab">Core Framework</Link>
            <Link to="/lab/vision">Kapi Vision</Link>
            <Link to="/framework/content-model">Content model</Link>
          </nav>
        </div>
        <PdfExplorer samples={samples} />
      </main>
    </Layout>
  );
}
