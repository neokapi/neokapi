import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
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
      title="Lab"
      description="Learn the neokapi architecture by running it — one flow workspace whose guided lessons teach the content model, pipeline, tools, formats, projects and concurrency on the real engine, in your browser."
      noFooter
      wrapperClassName="lab-app-wrapper"
    >
      <main className={styles.appPage}>
        <header className={styles.appHeader}>
          <h1 className={styles.appTitle}>neokapi Lab</h1>
          <p className={styles.appLede}>
            Pick a lesson and its walkthrough drives the workspace — the real engine, compiled to
            WebAssembly, runs your flow in the browser. Nothing is mocked.
          </p>
          <nav className={styles.appNav} aria-label="Related labs">
            <Link to="/lab/segmentation">Segmentation</Link>
            <Link to="/lab/convert">File Conversion</Link>
            <Link to="/lab/structure">Structure &amp; Layout</Link>
            <Link to="/lab/vision">Kapi Vision</Link>
            <Link to="/lab/media">Audio &amp; Video</Link>
            <Link to="/playground-cli">CLI Playground</Link>
            <Link to="/klf-lab">Kapi L10N Format</Link>
            <Link to="/framework/architecture">Framework docs</Link>
          </nav>
        </header>
        <div className={styles.appWorkspace}>
          <FlowBuilderRunner defaultScenarioId="annotations" withRecordedTraces fill />
        </div>
      </main>
    </Layout>
  );
}
