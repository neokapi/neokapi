import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";

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
// the localization machinery, then the format-specific and media labs.
const LABS: LabEntry[] = [
  {
    to: "/lab",
    name: "Content Model Workspace",
    teaches:
      "The heart of the framework: watch the engine parse a file into Parts, Blocks, and Runs, run tools on it, and write it back. Start here.",
  },
  {
    to: "/playground-cli",
    name: "CLI Playground",
    teaches: "Run real kapi commands in your browser, the way you would from a terminal.",
  },
  {
    to: "/lab/models",
    name: "Models & Providers",
    teaches:
      "The models kapi translates with — the same on-device models on web and desktop, plus cloud providers — and a real local translation in your browser.",
  },
  {
    to: "/lab/segmentation",
    name: "Segmentation",
    teaches:
      "Compare segmentation engines (SRX, UAX-29, hybrid, Intl.Segmenter, SaT, LLM) on your own text and see where they disagree.",
  },
  {
    to: "/lab/convert",
    name: "File Conversion",
    teaches: "Re-express one format as another and inspect what survives the round trip.",
  },
  {
    to: "/lab/structure",
    name: "Structure & Layout",
    teaches: "Recover reading order, outline, and geometry from a PDF.",
  },
  {
    to: "/lab/vision",
    name: "Vision",
    teaches: "Run OCR and layout recognition on an image or an image embedded in a document.",
  },
  {
    to: "/lab/media",
    name: "Audio & Video",
    teaches: "Transcribe audio and pull text out of video, the first step toward subtitles.",
  },
  {
    to: "/klf-lab",
    name: "KLF Format",
    teaches: "Edit a KLF document live and watch the engine canonicalize, render, and validate it.",
  },
];

export default function LabsOverviewPage(): React.ReactElement {
  return (
    <Layout
      title="Labs"
      description="Interactive, in-browser labs that run the real kapi WebAssembly engine on your own files — the content model, translation, segmentation, conversion, structure, vision, and media — with a suggested order to explore them."
    >
      <main className="container margin-vert--lg">
        <h1>Labs</h1>
        <p style={{ maxWidth: "44rem" }}>
          Every lab below runs the real <code>kapi</code> engine in your browser via WebAssembly —
          no install, no server, and (cloud providers aside) no API key. Drop in your own file and
          watch what the engine does. They are ordered as a suggested path: begin with the{" "}
          <strong>Content Model Workspace</strong> to see how kapi represents any document, then
          explore the labs that interest you.
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
