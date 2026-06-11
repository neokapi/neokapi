import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import { SegmentationPreview } from "@site/src/components/Lab";
import styles from "./segmentation.module.css";

// The segmentation engines, side by side — split out of the main Lab page so
// the flow workspace there can be app-like (full height) while this
// comparison keeps its own focused page (linked from the Labs menu).

export default function SegmentationLabPage(): React.ReactElement {
  return (
    <Layout
      title="Segmentation Lab"
      description="Compare neokapi's segmentation engines side by side — pure-Go SRX rules, the raw UAX-29 Unicode baseline, and the Hybrid that segments natively — on your own text, in the browser."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>Segmentation Lab</h1>
          <p className={styles.lede}>
            The <Link to="/lab">Lab&apos;s segmentation lesson</Link> shows sentence segmentation as
            a stand-off overlay; this lab compares the engines that produce it. Switch between the
            pure-Go <strong>SRX</strong> rules, the raw <strong>UAX-29</strong> Unicode baseline
            (ICU4X, a companion WebAssembly module), and the <strong>Hybrid</strong> — ICU4X breaks
            refined by SRX exceptions, how neokapi segments natively. The <strong>SaT</strong> ML
            segmenter is a native plugin (kapi-sat), shown here but disabled in the browser. Watch
            how each treats abbreviations, decimals, and quotes.
          </p>
        </div>
        <SegmentationPreview defaultSampleId="page-html" />
      </main>
    </Layout>
  );
}
