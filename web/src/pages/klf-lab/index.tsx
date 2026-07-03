import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import KlfAnatomy from "@site/src/components/Lab/KlfAnatomy";
import { KlfExplorer } from "@site/src/components/Lab/KlfExplorer";
import { LabPageShell, LabFootnote } from "@site/src/components/Lab/LabPageShell";

// The KLF anatomy page: a linear reading of the Kapi Localization Format.
// First a static worked example — one realistic document, two panes: the
// highlighted .klf source on the left, the structured explanation of the
// selected part on the right — then an interactive round-trip through the
// real engine (core/klf compiled to WebAssembly) behind the standard RunGate.
// The conformance dashboard stays at /klf-tests and is linked as results.

function SectionHeading({ children }: { children: React.ReactNode }): React.ReactElement {
  return <h2 className="mb-2 mt-12 text-2xl font-semibold text-foreground">{children}</h2>;
}

export default function KlfLabPage(): React.ReactElement {
  return (
    <Layout
      title="Anatomy of a KLF document"
      description="A worked reading of the Kapi Localization Format: one realistic .klf document with every part — envelope, blocks, runs, targets, provenance — explained, and an interactive round-trip through the real engine."
    >
      <LabPageShell
        title="Anatomy of a KLF document"
        maxWidthClassName="max-w-[1160px]"
        lede={
          <>
            The <Link to="/reference/klf/spec">Kapi Localization Format</Link> (<code>.klf</code>)
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
            One React component, <code>CheckoutBanner</code>, extracted to KLF: a heading with
            inline markup and a variable — already translated into Norwegian Bokmål — and a plural
            still awaiting translation. Select a line on the left, or a term on the right, to see
            what that part of the document is for.
          </p>
          <KlfAnatomy />
        </section>

        <section aria-labelledby="roundtrip">
          <SectionHeading>
            <span id="roundtrip">Round-trip through the engine</span>
          </SectionHeading>
          <p className="mb-4 max-w-[78ch] text-sm leading-relaxed text-muted-foreground">
            The demonstration below runs <code>core/klf</code> — the canonical Go implementation,
            compiled to WebAssembly — on a document you can edit. The engine parses the text,
            renders each block&rsquo;s preview, validates the run structure against the rules of the
            specification, writes the document back in canonical form, and resolves a companion{" "}
            <code>.klfl</code> annotation overlay anchor by anchor. Nothing is mocked; this is the
            code the CLI runs.
          </p>
          <KlfExplorer defaultSampleId="full" />
        </section>

        <LabFootnote>
          Conformance results: the <Link to="/klf-tests">KLF conformance suite</Link> runs the spec
          tests against both the Go engine and the TypeScript mirror in your browser. The full
          schema is in the <Link to="/reference/klf/spec">specification</Link>; further editable
          examples are in <Link to="/reference/klf/examples">Examples</Link>.
        </LabFootnote>
      </LabPageShell>
    </Layout>
  );
}
