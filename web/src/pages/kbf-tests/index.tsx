import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import { KbfConformance } from "@site/src/components/Lab/KbfConformance";
import { LabPageShell, LabFootnote } from "@site/src/components/Lab/LabPageShell";

// The KBF Tests page — a cross-engine conformance runner. It executes the KBF
// spec conformance suite in the browser against BOTH reference implementations:
// the canonical Go engine (core/kbf, compiled to WebAssembly) and the
// TypeScript mirror (@neokapi/kapi-format). For every operation both implement,
// it asserts the two engines agree byte-for-byte — making the spec's parity
// guarantee executable rather than asserted.

export default function KbfTestsPage(): React.ReactElement {
  return (
    <Layout
      title="KBF Tests"
      description="A cross-engine KBF spec conformance suite that runs in your browser against both the canonical Go engine and the TypeScript mirror, proving they agree."
    >
      <LabPageShell
        title="KBF conformance suite"
        maxWidthClassName="max-w-[1100px]"
        lede={
          <>
            The <Link to="/reference/serialization/content-bundle">Kapi Bundle Format</Link> has two reference
            implementations: the canonical Go engine (<code>core/kbf</code>) and a TypeScript mirror
            (<code>@neokapi/kapi-format</code>). The spec promises they are kept byte-for-byte
            equivalent. This page makes that promise <em>executable</em>: it runs the conformance
            suite in your browser against both engines — the Go one compiled to WebAssembly, the
            TypeScript one natively — and checks that, for every operation both implement, they
            agree.
          </>
        }
        heroExtra={
          <p className="mt-3 max-w-[75ch] text-sm leading-relaxed text-muted-foreground">
            Each row reports the result from each engine and whether they <strong>agree</strong>.
            Serialization, HTML preview, annotation anchor resolution, and required-placeholder
            target validation run on <strong>both</strong> engines. The structural and envelope
            checks run on the canonical Go engine only — the TypeScript mirror does not expose an
            identical API for those, so those rows are marked <em>canonical only</em>.
          </p>
        }
      >
        <section className="mb-8 overflow-x-auto">
          <KbfConformance />
        </section>

        <LabFootnote>
          Curious how a document flows through these operations? Read the{" "}
          <Link to="/kbf-lab">anatomy of a KBF document</Link> and round-trip one live, or consult
          the <Link to="/reference/serialization/content-bundle">specification</Link>.
        </LabFootnote>
      </LabPageShell>
    </Layout>
  );
}
