import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import KbfAnatomy from "@site/src/components/Lab/KbfAnatomy";
import { KbfExplorer } from "@site/src/components/Lab/KbfExplorer";
import { LabPageShell, LabFootnote } from "@site/src/components/Lab/LabPageShell";

// The KBF anatomy page: a linear reading of the Kapi Bundle Format.
// First a static worked example — one realistic document, two panes: the
// highlighted .kbf.json source on the left, the structured explanation of the
// selected part on the right — then an interactive round-trip through the
// real engine (core/kbf compiled to WebAssembly) behind the standard RunGate.
// The conformance dashboard stays at /kbf-tests and is linked as results.

function SectionHeading({ children }: { children: React.ReactNode }): React.ReactElement {
  return <h2 className="mb-2 mt-12 text-2xl font-semibold text-foreground">{children}</h2>;
}

export default function KbfLabPage(): React.ReactElement {
  return (
    <Layout
      title="Anatomy of a KBF document"
      description="A worked reading of the Kapi Bundle Format: one realistic .kbf.json document with every part — envelope, blocks, runs, targets, provenance — explained, and an interactive round-trip through the real engine."
    >
      <LabPageShell
        title="Anatomy of a KBF document"
        maxWidthClassName="max-w-[1160px]"
        lede={
          <>
            The <Link to="/reference/kbf/spec">Kapi Bundle Format</Link> (<code>.kbf.json</code>)
            is the interchange format of the kapi toolchain: one deterministic JSON document that
            carries a source file&rsquo;s translatable content as blocks of runs, its per-locale
            targets, and the provenance of every string. It exists so that extraction, translation,
            validation, and write-back can be separate steps — run by different tools, at different
            times — without losing structure or identity. This page reads one realistic document
            part by part, then round-trips it through the engine.
          </>
        }
      >
        <section aria-labelledby="anatomy">
          <SectionHeading>
            <span id="anatomy">A worked example</span>
          </SectionHeading>
          <p className="mb-4 max-w-[78ch] text-sm leading-relaxed text-muted-foreground">
            One React component, <code>CheckoutBanner</code>, extracted to KBF: a heading with
            inline markup and a variable — already translated into Norwegian Bokmål — and a plural
            still awaiting translation. Select a line on the left, or a term on the right, to see
            what that part of the document is for.
          </p>
          <KbfAnatomy />
        </section>

        <section aria-labelledby="roundtrip">
          <SectionHeading>
            <span id="roundtrip">Round-trip through the engine</span>
          </SectionHeading>
          <p className="mb-4 max-w-[78ch] text-sm leading-relaxed text-muted-foreground">
            The demonstration below runs <code>core/kbf</code> — the canonical Go implementation,
            compiled to WebAssembly — on a document you can edit. The engine parses the text,
            renders each block&rsquo;s preview, validates the run structure against the rules of the
            specification, writes the document back in canonical form, and resolves a companion{" "}
            <code>.overlays.jsonl</code> annotation overlay anchor by anchor. Nothing is mocked; this is the
            code the CLI runs.
          </p>
          <KbfExplorer defaultSampleId="full" />
        </section>

        <LabFootnote>
          Conformance results: the <Link to="/kbf-tests">KBF conformance suite</Link> runs the spec
          tests against both the Go engine and the TypeScript mirror in your browser. The full
          schema is in the <Link to="/reference/kbf/spec">specification</Link>; further editable
          examples are in <Link to="/reference/kbf/examples">Examples</Link>.
        </LabFootnote>
      </LabPageShell>
    </Layout>
  );
}
