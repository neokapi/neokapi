import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import { SegmentationPreview } from "@site/src/components/Lab";
import { FlowBuilderRunner } from "@site/src/components/Lab/FlowBuilderRunner";
import styles from "./index.module.css";

// The Lab is one workspace, many lessons: the flow editor IS the lab. Every
// lesson is a scenario with a guided walkthrough that drives the same
// workspace — focusing the node it talks about, opening its panel, running the
// flow — because in neokapi the designed flow is exactly what runs. The
// content model lives behind the Source pill (the reader's tree), the
// round-trip guarantee behind the Sink pill (output diffed against input),
// tools in the step config panels (the script step is a real code editor),
// the project model in the recipe view the canvas serializes to, and the
// concurrency story in the run transport — live wasm runs plus recorded
// native traces (parallel workers, the Java bridge). Everything runs the real
// kapi engine in the browser via WebAssembly; nothing is mocked. The same
// explorers are embedded inline next to the concepts they teach in the
// Framework docs.

interface SectionProps {
  step: string;
  title: string;
  intro: React.ReactNode;
  children: React.ReactNode;
}

function Section({
  step,
  title,
  intro,
  children,
}: SectionProps): React.ReactElement {
  return (
    <section className={styles.section}>
      <span className={styles.step}>{step}</span>
      <h2>{title}</h2>
      <p className={styles.sectionIntro}>{intro}</p>
      {children}
    </section>
  );
}

export default function LabPage(): React.ReactElement {
  return (
    <Layout
      title="Lab"
      description="Learn the neokapi architecture by running it — one flow workspace whose guided lessons teach the content model, pipeline, tools, formats, projects and concurrency on the real engine, in your browser."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>neokapi Lab</h1>
          <p className={styles.lede}>
            Learn the architecture by running it. The Lab is one workspace —
            the same flow editor that ships in Kapi — whose guided lessons
            teach the engine on a sample, or on files you drop in. Nothing is
            mocked; this is the same engine the{" "}
            <Link to="/playground-cli">CLI</Link> runs, compiled to
            WebAssembly. The same explorers appear inline next to the concepts
            they teach throughout the{" "}
            <Link to="/framework/architecture">Framework</Link> docs.
          </p>
        </div>

        <Section
          step="01 · The workspace"
          title="One flow, every lesson"
          intro={
            <>
              A flow is an ordered composition of tools, and it runs as a
              concurrent pipeline: a reader, the tool chain, a writer,
              connected by channels of Parts. The graph you design here is
              exactly what runs. Pick a lesson and its{" "}
              <strong>walkthrough</strong> drives the workspace step by step —
              the rest of the engine is one click deep, behind the{" "}
              <strong>Inspect</strong> pills, the node panels, and the recipe
              view.
            </>
          }
        >
          <FlowBuilderRunner defaultScenarioId="annotations" withRecordedTraces />
        </Section>

        <Section
          step="02 · Segmentation engines"
          title="Compare the segmenters, side by side"
          intro={
            <>
              The segmentation lesson above shows the overlay; this interlude
              compares the engines that produce it. Switch between the pure-Go{" "}
              <strong>SRX</strong> rules, the raw <strong>UAX-29</strong>{" "}
              Unicode baseline (ICU4X, a companion WebAssembly module), and the{" "}
              <strong>Hybrid</strong> — ICU4X breaks refined by SRX exceptions,
              how neokapi segments natively. The <strong>SaT</strong> ML
              segmenter is a native plugin (kapi-sat), shown here but disabled
              in the browser. Watch how each treats abbreviations, decimals,
              and quotes.
            </>
          }
        >
          <SegmentationPreview defaultSampleId="page-html" />
        </Section>
      </main>
    </Layout>
  );
}
