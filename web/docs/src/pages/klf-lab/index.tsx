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
      description="Edit a Kapi Localization Format document in the browser and watch the real engine parse, canonicalize, render, validate, and resolve annotations against it."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>KLF Lab</h1>
          <p className={styles.lede}>
            Edit a <Link to="/reference/klf/spec">.klf</Link> document below and
            watch the real <code>core/klf</code> engine — compiled to
            WebAssembly — work on it live: it canonicalizes the bytes, renders
            each block to its preview, validates the run structure, and resolves
            a companion <code>.klfl</code> annotation overlay anchor by anchor.
            Nothing is mocked; this is the same engine the{" "}
            <Link to="/playground-cli">kapi CLI</Link> runs.
          </p>
        </div>

        <section className={styles.section}>
          <KlfExplorer defaultSampleId="full" />
        </section>

        <p className={styles.footnote}>
          Want to see the two reference implementations agree? The{" "}
          <Link to="/klf-tests">KLF conformance suite</Link> runs the spec tests
          against both the Go engine and the TypeScript mirror in your browser.
          For the full schema, see the{" "}
          <Link to="/reference/klf/spec">specification</Link>.
        </p>
      </main>
    </Layout>
  );
}
