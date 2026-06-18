import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import useBaseUrl from "@docusaurus/useBaseUrl";
import { PdfExplorer } from "@site/src/components/Lab";
import styles from "./pdf.module.css";

// The PDF Lab: upload a PDF (or use the bundled sample) and watch the real kapi
// engine parse it in your browser. PDF text + geometry come from PDFium compiled
// to WebAssembly, bridged into the engine's wasm PDF reader — the same PDFium
// the native kapi-pdfium plugin uses, nothing mocked. The result renders in the
// shared DocumentViewer: Layout places each text block on the page (geometry),
// Structure shows the outline, Blocks lists the extracted content.

export default function PdfLabPage(): React.ReactElement {
  // Bundled samples, fetched into the file switcher on load. report.pdf and
  // invoice.pdf are structured documents (headings + tables) that show off the
  // structure inference in the Structure tab; anatomy.pdf is the minimal sample.
  const samples = [
    { url: useBaseUrl("/samples/report.pdf"), name: "report.pdf" },
    { url: useBaseUrl("/samples/invoice.pdf"), name: "invoice.pdf" },
    { url: useBaseUrl("/samples/anatomy.pdf"), name: "anatomy.pdf" },
  ];
  return (
    <Layout
      title="PDF Lab"
      description="Upload a PDF and see it parsed in your browser by the real kapi engine — text and geometry extracted via PDFium (WebAssembly), shown as structure and content in context. Nothing is mocked."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>PDF Lab</h1>
          <p className={styles.lede}>
            Drop in a PDF and the <Link to="/lab">Lab</Link> engine — compiled to WebAssembly —
            parses it right here in your browser. Text and per-block geometry are extracted by{" "}
            <strong>PDFium</strong> (also WebAssembly), the same engine the native{" "}
            <code>kapi-pdfium</code> plugin uses on the desktop. Switch to the{" "}
            <strong>Layout</strong> tab to see each block in its place on the page,{" "}
            <strong>Structure</strong> for the document outline, and <strong>Blocks</strong> for the
            extracted content. Nothing is mocked.
          </p>
          <nav className={styles.nav} aria-label="Related labs">
            <Link to="/lab">Lab</Link>
            <Link to="/lab/segmentation">Segmentation Lab</Link>
            <Link to="/framework/content-model">Content model</Link>
          </nav>
        </div>
        <PdfExplorer samples={samples} />
      </main>
    </Layout>
  );
}
