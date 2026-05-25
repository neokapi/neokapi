import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import { AnatomyExplorer, PipelineExplorer } from "@site/src/components/Lab";
import { ToolLab } from "@site/src/components/Lab/ToolLab";
import { RoundTripExplorer } from "@site/src/components/Lab/RoundTripExplorer";
import { FlowBuilderRunner } from "@site/src/components/Lab/FlowBuilderRunner";
import styles from "./index.module.css";

// The Lab is the free-play destination that composes every interactive
// explorer into one narrative: bring a file, see how neokapi reads it, watch it
// flow through the pipeline, transform it, round-trip it, and compose your own
// flow. Every explorer runs the real kapi engine in the browser via WebAssembly
// — the same code the CLI runs, on your own files. The same explorers are
// embedded inline next to the concepts they teach in the Framework docs.

interface SectionProps {
  step: string;
  title: string;
  intro: React.ReactNode;
  children: React.ReactNode;
}

function Section({ step, title, intro, children }: SectionProps): React.ReactElement {
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
      description="Learn the neokapi architecture by running it — interactive, WebAssembly-powered explorers for the content model, pipeline, tools, formats and flows."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>neokapi Lab</h1>
          <p className={styles.lede}>
            Learn the architecture by running it. Each explorer below runs the real
            kapi engine in your browser (compiled to WebAssembly) on a sample — or
            on a file you drop in. Nothing is mocked; this is the same engine the{" "}
            <Link to="/playground-cli">CLI</Link> runs. The same explorers appear
            inline next to the concepts they teach throughout the{" "}
            <Link to="/framework/architecture">Framework</Link> docs.
          </p>
        </div>

        <Section
          step="01 · Content model"
          title="Anatomy — how a reader sees your file"
          intro={
            <>
              A format reader turns raw bytes into the content model: nested Layers
              and Groups containing Blocks, whose text is a sequence of Runs. Watch
              an HTML <code>&lt;strong&gt;</code> become a paired inline code while a
              JSON <code>{"{name}"}</code> stays literal text.
            </>
          }
        >
          <AnatomyExplorer defaultSampleId="page-html" />
        </Section>

        <Section
          step="02 · Pipeline"
          title="Pipeline — watch Parts stream through tools"
          intro={
            <>
              A flow runs as a concurrent pipeline: a reader, a chain of tools, and a
              writer, connected by channels of Parts. Run a file and drive it with{" "}
              <strong>Next</strong> — each step is one observable transition.
            </>
          }
        >
          <PipelineExplorer defaultSampleId="messages-json" defaultPipelineId="pseudo-translate" />
        </Section>

        <Section
          step="03 · Tools"
          title="Tools — configure a transform, see the effect"
          intro={
            <>
              A tool transforms the Blocks it cares about and relays the rest. Pick a
              tool, change its configuration, and read the per-Block before/after.
            </>
          }
        >
          <ToolLab defaultSampleId="messages-json" />
        </Section>

        <Section
          step="04 · Formats"
          title="Round-trip — structure in, structure out"
          intro={
            <>
              Readers split a document into translatable Blocks and a non-translatable
              skeleton; writers splice translations back into that skeleton. The
              result: only the translatable text changes, structure is reproduced
              exactly.
            </>
          }
        >
          <RoundTripExplorer defaultSampleId="page-html" />
        </Section>

        <Section
          step="05 · Flows"
          title="Build a flow, then run it"
          intro={
            <>
              A flow is an ordered composition of tools. Build one in the visual
              editor, then run it live and step through the result — the graph you
              build and the flow the CLI runs are the same thing.
            </>
          }
        >
          <FlowBuilderRunner defaultSampleId="messages-json" />
        </Section>
      </main>
    </Layout>
  );
}
