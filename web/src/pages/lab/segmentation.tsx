import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
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
      description="Compare every neokapi segmentation engine — SRX rules, the UAX-29 / Intl.Segmenter Unicode baselines, the native Hybrid, and the learned SaT and LLM segmenters — on your own text, in the browser."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Segmentation Lab</h1>
          <p className={styles.lede}>
            neokapi treats segmentation as a stand-off overlay on the{" "}
            <Link to="/framework/architecture">content model</Link>: the same source, segmented into
            sentences by whichever engine you choose. Compare the pure-Go <strong>SRX</strong>{" "}
            rules, the <strong>UAX-29</strong> Unicode baseline (ICU4X) and the browser&apos;s
            built-in <strong>Intl.Segmenter</strong>, the native <strong>Hybrid</strong> (ICU4X base
            refined by SRX exceptions), and the learned <strong>SaT</strong> and{" "}
            <strong>Gemma</strong> segmenters — the same engine names the CLI and flow editor use.
            Bring your own text and watch how each treats abbreviations, decimals, quotes, and
            scripts without spaces.
          </p>
        </div>
        <SegmentationLab />
      </main>
    </Layout>
  );
}
