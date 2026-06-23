import React from "react";
import Layout from "@theme/Layout";
import { FlowBuilderRunner } from "@site/src/components/Lab/FlowBuilderRunner";
import styles from "./index.module.css";

// The Lab is one workspace, many lessons: the flow editor IS the lab, and the
// page is app-like — the workspace fills the viewport (no double scrolling)
// and every lesson is a scenario whose guided walkthrough drives the same
// workspace: focusing the node it talks about, opening its panel, running the
// flow. Everything runs the real kapi engine in the browser via WebAssembly;
// nothing is mocked. The same explorers are embedded inline next to the
// concepts they teach in the Framework docs; the segmentation engines have
// their own lab at /lab/segmentation.

export default function LabPage(): React.ReactElement {
  return (
    <Layout
      title="Content Model Workspace"
      description="See how neokapi reads a document, breaks it into translatable pieces, and runs a localization flow over it — step by step, on real files, right in your browser. No install."
      noFooter
      wrapperClassName="lab-app-wrapper"
    >
      <main className={styles.appPage}>
        <header className={styles.appHeader}>
          <h1 className={styles.appTitle}>Content Model Workspace</h1>
          <p className={styles.appLede}>
            Pick a lesson and follow along as neokapi reads a file, breaks it into translatable
            pieces, and runs a flow over it — live, in your browser. It&rsquo;s the real engine, so
            what you build here is what you&rsquo;d get on your own files.
          </p>
        </header>
        <div className={styles.appWorkspace}>
          <FlowBuilderRunner defaultScenarioId="annotations" withRecordedTraces fill />
        </div>
      </main>
    </Layout>
  );
}
