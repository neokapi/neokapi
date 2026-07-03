import React from "react";
import Layout from "@theme/Layout";
import Link from "@docusaurus/Link";
import { KlfConformance } from "@site/src/components/Lab/KlfConformance";
import { LabPageShell, LabFootnote } from "@site/src/components/Lab/LabPageShell";

// The KLF Tests page — a cross-engine conformance runner. It executes the KLF
// spec conformance suite in the browser against BOTH reference implementations:
// the canonical Go engine (core/klf, compiled to WebAssembly) and the
// TypeScript mirror (@neokapi/kapi-format). For every operation both implement,
// it asserts the two engines agree byte-for-byte — making the spec's parity
// guarantee executable rather than asserted.

export default function KlfTestsPage(): React.ReactElement {
  return (
    <Layout
      title="KLF Tests"
      description="A cross-engine KLF spec conformance suite that runs in your browser against both the canonical Go engine and the TypeScript mirror, proving they agree."
    >
      <LabPageShell
        title="KLF conformance suite"
        maxWidthClassName="max-w-[1100px]"
        lede={
          <>
            The <Link to="/reference/klf/spec">Kapi Localization Format</Link> has two reference
            implementations: the canonical Go engine (<code>core/klf</code>) and a TypeScript mirror
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
          <KlfConformance />
        </section>

        <LabFootnote>
          Curious how a document flows through these operations? Open the{" "}
          <Link to="/klf-lab">KLF Lab</Link> and edit one live, or read the{" "}
          <Link to="/reference/klf/spec">specification</Link>.
        </LabFootnote>
      </LabPageShell>
    </Layout>
  );
}
