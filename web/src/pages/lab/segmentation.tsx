import React from "react";
import Layout from "@theme/Layout";
import { SegmentationLab } from "@site/src/components/Lab";
import styles from "./segmentation.module.css";

// One consolidated Segmentation lab: every engine neokapi exposes — rule-based
// (SRX), Unicode (UAX-29 via ICU4X, and the browser's Intl.Segmenter), the
// native Hybrid, and the learned SaT / LLM segmenters — compared on a source you
// provide, each showing its sentences inline. It demonstrates segmentation as a
// stand-off overlay on the content model, not just an AI feature.

export default function SegmentationLabPage(): React.ReactElement {
  return (
    <Layout
      title="Segmentation Lab"
      description="Before text can be translated well it has to be split into sentences correctly — knowing that “Dr.” or “$3.50” isn't a sentence break. Compare how neokapi's segmentation methods handle the tricky cases on your own text, in your browser."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Segmentation Lab</h1>
          <p className={styles.lede}>
            Before anything gets translated, text has to be broken into sentences — and getting that
            right (that &ldquo;Dr.&rdquo; or &ldquo;$3.50&rdquo; isn&rsquo;t the end of one) is what
            makes translations and translation-memory matches reliable. neokapi offers several ways
            to do it, from fast rule-based splitting to a learned model and a local LLM. Bring your
            own text and watch how each handles abbreviations, decimals, quotes, and languages
            written without spaces — the very same engines you can pick in the CLI and the flow
            editor.
          </p>
        </div>
        <SegmentationLab />
      </main>
    </Layout>
  );
}
