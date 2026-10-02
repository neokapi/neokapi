import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import Translate, { translate } from "@docusaurus/Translate";

// Labs overview: one map of every interactive in-browser lab, what each one
// teaches, and a suggested order to visit them. Every lab runs the real kapi
// WebAssembly engine on your own files — no install, no server, no API key
// (cloud providers excepted).

interface LabEntry {
  to: string;
  name: string;
  teaches: string;
}

// Ordered as a suggested learning sequence: start with the content model, then
// the CLI and the segmentation engines, then the format-specific and media labs.
const LABS: LabEntry[] = [
  {
    to: "/lab",
    name: translate({ id: "labs.lab.name", message: "Content Model Workspace" }),
    teaches: translate({
      id: "labs.lab.teaches",
      message:
        "The heart of the framework: watch the engine parse a file into Parts, Blocks, and Runs, run tools on it, and write it back. Start here.",
    }),
  },
  {
    to: "/playground-cli",
    name: translate({ id: "labs.playgroundcli.name", message: "CLI Playground" }),
    teaches: translate({
      id: "labs.playgroundcli.teaches",
      message: "Run real kapi commands in your browser, the way you would from a terminal.",
    }),
  },
  {
    to: "/lab/segmentation",
    name: translate({ id: "labs.lab.segmentation.name", message: "Segmentation" }),
    teaches: translate({
      id: "labs.lab.segmentation.teaches",
      message:
        "Compare segmentation engines (SRX, UAX-29, hybrid, Intl.Segmenter, SaT, LLM) on your own text and see where they disagree.",
    }),
  },
  {
    to: "/lab/convert",
    name: translate({ id: "labs.lab.convert.name", message: "File Conversion" }),
    teaches: translate({
      id: "labs.lab.convert.teaches",
      message: "Re-express one format as another and inspect what survives the round trip.",
    }),
  },
  {
    to: "/lab/structure",
    name: translate({ id: "labs.lab.structure.name", message: "Structure & Layout" }),
    teaches: translate({
      id: "labs.lab.structure.teaches",
      message: "Recover reading order, outline, and geometry from a PDF.",
    }),
  },
  {
    to: "/lab/vision",
    name: translate({ id: "labs.lab.vision.name", message: "Vision" }),
    teaches: translate({
      id: "labs.lab.vision.teaches",
      message: "Run OCR and layout recognition on an image or an image embedded in a document.",
    }),
  },
  {
    to: "/lab/media",
    name: translate({ id: "labs.lab.media.name", message: "Audio & Video" }),
    teaches: translate({
      id: "labs.lab.media.teaches",
      message: "Transcribe audio and pull text out of video, the first step toward subtitles.",
    }),
  },
  {
    to: "/kbf-lab",
    name: translate({ id: "labs.kbflab.name", message: "KBF Anatomy" }),
    teaches: translate({
      id: "labs.kbflab.teaches",
      message:
        "A worked reading of the Kapi Bundle Format — envelope, blocks, runs, targets, provenance — with a live round-trip through the engine.",
    }),
  },
];

export default function LabsOverviewPage(): React.ReactElement {
  return (
    <Layout
      title={translate({ id: "labs.page.title", message: "Labs" })}
      description={translate({
        id: "labs.page.description",
        message:
          "Interactive, in-browser labs that run the real kapi WebAssembly engine on your own files — the content model, translation, segmentation, conversion, structure, vision, and media — with a suggested order to explore them.",
      })}
    >
      <main className="container margin-vert--lg">
        <h1>
          <Translate id="labs.heading">Labs</Translate>
        </h1>
        <p style={{ maxWidth: "44rem" }}>
          <Translate id="labs.intro.lead">Every lab below runs the real</Translate>{" "}
          <code>kapi</code>{" "}
          <Translate id="labs.intro.mid">
            engine in your browser via WebAssembly — no install, no server, and (cloud providers
            aside) no API key. Drop in your own file and watch what the engine does. They are
            ordered as a suggested path: begin with the
          </Translate>{" "}
          <strong>
            <Translate id="labs.intro.first">Content Model Workspace</Translate>
          </strong>{" "}
          <Translate id="labs.intro.tail">
            to see how kapi represents any document, then explore the labs that interest you.
          </Translate>
        </p>
        <div className="row margin-top--md">
          {LABS.map((lab) => (
            <div key={lab.to} className="col col--6 margin-bottom--lg">
              <Link className="card padding--lg" to={lab.to} style={{ height: "100%" }}>
                <h3>{lab.name}</h3>
                <p style={{ marginBottom: 0 }}>{lab.teaches}</p>
              </Link>
            </div>
          ))}
        </div>
      </main>
    </Layout>
  );
}
