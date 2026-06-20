import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import { KlfExplorer } from "@site/src/components/Lab/KlfExplorer";
import styles from "./index.module.css";

// The KLF Lab — a free-play destination for the Kapi Localization Format. Edit
// a .klf document and watch the real engine (core/klf, compiled to WebAssembly)
// canonicalize the bytes, render each block to its preview, validate the run
// structure, and resolve a companion .klfl annotation overlay. Everything runs
// the same code the kapi CLI runs — nothing is mocked.

export default function KlfLabPage(): React.ReactElement {
  return (
    <Layout
      title="KLF Lab"
      description="Kapi's localization format keeps a document's text, its structure, and its translations together in one file. Edit one here and watch neokapi check and render it live."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>KLF Lab</h1>
          <p className={styles.lede}>
            The <Link to="/reference/klf/spec">Kapi Localization Format</Link> keeps a
            document&rsquo;s text, its structure, and its translations together in one file. Edit
            one below and watch neokapi work on it live — rendering each piece as it&rsquo;ll
            appear, checking that the formatting is well-formed, and flagging anything that
            doesn&rsquo;t add up — so you can see how your content holds together as you change it.
          </p>
        </div>

        <section className={styles.section}>
          <KlfExplorer defaultSampleId="full" />
        </section>

        <p className={styles.footnote}>
          Want to see the two reference implementations agree? The{" "}
          <Link to="/klf-tests">KLF conformance suite</Link> runs the spec tests against both the Go
          engine and the TypeScript mirror in your browser. For the full schema, see the{" "}
          <Link to="/reference/klf/spec">specification</Link>.
        </p>
      </main>
    </Layout>
  );
}
