import React from "react";
import Layout from "@theme/Layout";
import { ConversionExplorer } from "@site/src/components/Lab";
import styles from "./convert.module.css";

// The Conversion Lab: read a document in one format, re-express it in another —
// the real kapi `convert` (kconv) running in the browser via WebAssembly. The
// target list is restricted to GENERATIVE writers (those that reconstruct a
// whole document from the content model); skeleton-driven formats (docx/odt/
// idml/epub) inject into an original file and so cannot be conversion targets.

export default function ConversionLabPage(): React.ReactElement {
  return (
    <Layout
      title="Conversion Lab"
      description="Turn a document into another format — Markdown, HTML, XLIFF, JSON and more — without losing its structure or inline formatting. See the before and after, side by side, right in your browser."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Conversion Lab</h1>
          <p className={styles.lede}>
            Pick a document and a target format. neokapi reads it, understands its structure —
            headings, lists, tables, inline styling — and rewrites it in the new format with all of
            that kept intact. The original and the result sit side by side, and you can flip the
            result between the <strong>Rendered</strong> page and its raw <strong>Source</strong>,
            so you can confirm nothing was lost in the move. It all runs in your browser.
          </p>
        </div>
        <ConversionExplorer defaultSampleId="article-md" defaultTarget="doclang" />
      </main>
    </Layout>
  );
}
