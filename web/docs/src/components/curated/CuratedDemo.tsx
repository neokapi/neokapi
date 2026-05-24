import React from "react";
import BlockPreview from "./BlockPreview";
import BeforeAfter from "./BeforeAfter";
import DualExample from "./DualExample";

// CuratedDemo — a self-contained gallery proving the three curated result views
// render against the real kapi wasm. Rendered at the scratch route
// /curated-demo (see src/pages/curated-demo/index.tsx). Not part of the
// published docs; it exercises each component for verification + as a living
// usage reference.

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <section style={{ marginBottom: "2.5rem" }}>
      <h2 style={{ marginBottom: "0.75rem" }}>{title}</h2>
      {children}
    </section>
  );
}

export default function CuratedDemo(): React.ReactElement {
  return (
    <div style={{ maxWidth: 980, margin: "0 auto", padding: "2rem 1rem" }}>
      <h1>Curated result views — demo</h1>
      <p>
        Each widget below boots the real kapi CLI (WebAssembly) in your browser and shows what the
        framework produced. They share one warm runtime.
      </p>

      <Section title="BlockPreview — the “kapi reader”">
        <BlockPreview sample="messages.json" caption="How kapi sees a flat JSON message catalog." />
        <BlockPreview
          sample="strings.xml"
          caption="The same content model, parsed from an Android strings.xml file."
        />
        <BlockPreview
          sample="page.html"
          caption="An HTML page — inline markup shows up as span chips in the source text."
        />
        <BlockPreview
          sample={{
            name: "inline-demo.html",
            content:
              '<p>Click <a href="/go">here</a> to continue, or <strong>cancel</strong>.</p>\n',
          }}
          title="inline-demo.html (inline sample)"
          caption="An inline {name, content} sample — no bundled fixture needed."
        />
      </Section>

      <Section title="BeforeAfter — source → transformed result">
        <BeforeAfter
          sample="messages.json"
          command="kapi pseudo-translate messages.json -o out.json"
          outputPath="out.json"
          caption="Pseudo-translation: readable accented output, no API key."
        />
        <BeforeAfter
          sample="README.md"
          command="kapi pseudo-translate README.md -o README.out.md"
          outputPath="README.out.md"
          beforeLabel="Markdown in"
          afterLabel="Pseudo-translated"
          caption="The same transform on a Markdown document."
        />
      </Section>

      <Section title="DualExample — CLI command ⇄ curated result (split)">
        <DualExample
          command="kapi word-count messages.json"
          seed={["messages.json"]}
          result={{
            kind: "blocks",
            sample: "messages.json",
            title: "messages.json (parsed)",
            caption: "The same file, as kapi's content model.",
          }}
          caption="Left: the command and its real output. Right: what the framework parsed."
        />
      </Section>

      <Section title="DualExample — before/after result (tabs)">
        <DualExample
          layout="tabs"
          command="kapi pseudo-translate messages.json -o out.json"
          seed={["messages.json"]}
          result={{
            kind: "before-after",
            sample: "messages.json",
            command: "kapi pseudo-translate messages.json -o out.json",
            outputPath: "out.json",
          }}
          caption="Tabbed: switch between the CLI command and the before/after result."
        />
      </Section>
    </div>
  );
}
