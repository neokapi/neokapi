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
            Splitting text into sentences sounds trivial until &ldquo;Dr.&rdquo; or
            &ldquo;$3.50&rdquo; ends one by mistake. Compare neokapi&rsquo;s segmenters — rules,
            Unicode, a learned model, a local LLM — on your own text, and see where they agree.
          </p>
        </div>
        <SegmentationLab />
      </main>
    </Layout>
  );
}
