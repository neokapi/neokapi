import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import {
  AnatomyExplorer,
  BatchExplorer,
  SegmentationPreview,
} from "@site/src/components/Lab";
import { ToolLab } from "@site/src/components/Lab/ToolLab";
import { RoundTripExplorer } from "@site/src/components/Lab/RoundTripExplorer";
import { FlowBuilderRunner } from "@site/src/components/Lab/FlowBuilderRunner";
import { ScriptLab } from "@site/src/components/Lab/ScriptLab";
import { ConcurrencyExplorer } from "@site/src/components/Lab/ConcurrencyExplorer";
import { ProjectExplorer } from "@site/src/components/Lab/ProjectExplorer";
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
      description="Learn the neokapi architecture by running it — interactive, in-browser explorers for the content model, pipeline, tools, formats and flows."
    >
      <main className={styles.page}>
        <div className={styles.hero}>
          <h1>neokapi Lab</h1>
          <p className={styles.lede}>
            Learn the architecture by running it. Each explorer below runs the
            real kapi engine in your browser on a sample — or on a file you drop
            in. Nothing is mocked; this is the same engine the{" "}
            <Link to="/playground-cli">CLI</Link> runs. The same explorers
            appear inline next to the concepts they teach throughout the{" "}
            <Link to="/framework/architecture">Framework</Link> docs.
          </p>
        </div>

        <Section
          step="01 · Content model"
          title="Anatomy — how a reader sees your file"
          intro={
            <>
              A format reader turns raw bytes into the content model: nested
              Layers and Groups containing Blocks, whose text is a sequence of
              Runs. Watch an HTML <code>&lt;strong&gt;</code> become a paired
              inline code while a JSON <code>{"{name}"}</code> stays literal
              text.
            </>
          }
        >
          <AnatomyExplorer defaultSampleId="page-html" />
        </Section>

        <Section
          step="01b · Segmentation"
          title="Segmentation — split a block into sentences"
          intro={
            <>
              Segmentation is a stand-off overlay: it marks sentence boundaries
              without rewriting the runs. Switch the engine to compare the
              pure-Go <strong>SRX</strong> rules, the raw{" "}
              <strong>UAX-29</strong> Unicode baseline (ICU4X, a companion
              WebAssembly module), and the <strong>Hybrid</strong> — ICU4X
              breaks refined by SRX exceptions, how neokapi segments natively.
              The <strong>SaT</strong> ML segmenter is a native plugin
              (kapi-sat), shown here but disabled in the browser. Watch how each
              treats abbreviations, decimals, and quotes.
            </>
          }
        >
          <SegmentationPreview defaultSampleId="page-html" />
        </Section>

        <Section
          step="02 · Pipeline & flows"
          title="Design a flow, run it, step through the run"
          intro={
            <>
              A flow is an ordered composition of tools, and it runs as a
              concurrent pipeline: a reader, the tool chain, a writer, connected
              by channels of Parts. The graph you design here is exactly what
              runs — press <strong>Run</strong> and the trace plays back on the
              same nodes: scrub the transport, then click a node to open its{" "}
              <strong>run inspector</strong> and see every block that passed
              through with what the step attached — segmentation spans, entity
              tags, vaulted redaction secrets, QA findings, the written target.
              Pick a scenario to study one story (redaction, segmentation,
              annotations), or build your own; the{" "}
              <strong>Redaction</strong> scenario also shows project scope: its
              rules are a project preset (<code>defaults.tools</code>) the bare{" "}
              <code>redact</code> step inherits — and the placement check keeps
              the redactor ahead of any remote-egress step.
            </>
          }
        >
          <FlowBuilderRunner defaultScenarioId="annotations" />
        </Section>

        <Section
          step="03 · Tools"
          title="Tools — configure a transform, see the effect"
          intro={
            <>
              A tool transforms the Blocks it cares about and relays the rest.
              Pick a tool, change its configuration, and read the per-Block
              before/after.
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
              Readers split a document into Blocks of text and a structural
              skeleton; writers splice content back into that skeleton. The
              result: only the text in the blocks changes, structure is
              reproduced exactly.
            </>
          }
        >
          <RoundTripExplorer defaultSampleId="page-html" />
        </Section>

        <Section
          step="05 · Projects"
          title="Project — capture config once, run flows, merge files"
          intro={
            <>
              A <code>.kapi</code> project is the day-to-day working model:
              languages, content globs, and flows live in a committed recipe
              beside a <code>.kapi/</code> state dir. Run the lifecycle &mdash;{" "}
              <code>extract</code> the recipe&apos;s content, <code>run</code> a
              declared flow (process-only, committing to the project store),
              then <code>merge</code> the localized files out. Pick the content
              and which flow to run, and watch the recipe, the per-locale state,
              and the merged output update. Same engine as the single-file{" "}
              <code>.klz</code> workspace, organized for teams and servers.
            </>
          }
        >
          <ProjectExplorer defaultSampleId="json" />
        </Section>

        <Section
          step="06 · Concurrency"
          title="Concurrency — replay a pipeline trace"
          intro={
            <>
              The pipeline is concurrent: each tool runs in its own goroutine,
              connected by buffered channels of Parts. Replay a recorded trace
              to watch Parts move through the stages &mdash; including parallel
              workers, channel buffering, and the Java bridge&apos;s gRPC
              boundary. Drop in your own <code>kapi run --trace</code> output
              too.
            </>
          }
        >
          <ConcurrencyExplorer />
        </Section>

        <Section
          step="07 · Scripting"
          title="Script — write your own transform"
          intro={
            <>
              When no built-in tool fits, the <code>script</code> tool runs a
              small JavaScript program over each Part. Edit it with full
              autocomplete, load an example, and run it on your file — modify
              the text, filter parts, redact, or just <code>log()</code> what
              flows through.
            </>
          }
        >
          <ScriptLab defaultSampleId="messages-json" />
        </Section>

        <Section
          step="08 · Files & output"
          title="Run a tool across a selection, inspect what it wrote"
          intro={
            <>
              Pick one file, several, or a glob like <code>*.json</code> across
              the samples and anything you upload, then run a tool over the
              whole selection. Each output is shown three ways — the
              content-model <strong>Blocks</strong> (with targets, overlays and
              annotations), the <strong>Structure</strong>, and the
              syntax-highlighted <strong>Native</strong> bytes — downloadable,
              with the changed blocks and lines highlighted.
            </>
          }
        >
          <BatchExplorer defaultPattern="*.json" />
        </Section>
      </main>
    </Layout>
  );
}
