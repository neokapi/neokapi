import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
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
      description="Convert a document between formats in your browser — Markdown, HTML, DocLang, XLIFF, PO, JSON and more — and see both the rendered page and the format source, side by side. The real kapi engine, compiled to WebAssembly; nothing is mocked."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Conversion Lab</h1>
          <p className={styles.lede}>
            Pick a document and a target format: the <Link to="/reference">native reader</Link>{" "}
            parses it into the content model (roles, runs, tables, geometry), and a{" "}
            <strong>generative writer</strong> re-serializes that model — so structure and inline
            styling survive the hop. Toggle between the <strong>Rendered</strong> page and the{" "}
            <strong>Source</strong> of the chosen format. Skeleton-driven formats (docx, odt, idml,
            epub) inject translations back into an original file and so are not offered as targets.
            Everything runs the real <code>kapi convert</code> compiled to WebAssembly — nothing is
            mocked.
          </p>
        </div>
        <ConversionExplorer defaultSampleId="article-md" defaultTarget="doclang" />
      </main>
    </Layout>
  );
}
