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
            Pick a document. neokapi reads it and understands its structure — headings, lists,
            tables, inline styling — which you can explore in the <strong>Preview</strong>,{" "}
            <strong>Blocks</strong>, <strong>Structure</strong> and <strong>Layout</strong> tabs.
            That one content model never changes; each output-format pill on the right re-serializes
            it, showing the <strong>Rendered</strong> page and its raw <strong>Source</strong> side
            by side, so you can confirm nothing was lost in the move. It all runs in your browser.
          </p>
        </div>
        <ConversionExplorer defaultSampleId="article-md" defaultTarget="doclang" />
      </main>
    </Layout>
  );
}
