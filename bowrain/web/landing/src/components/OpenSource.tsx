import { Shield, Server, Package } from "lucide-react";
import { GITHUB_URL, KAPI_SITE_URL } from "../links";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_OPEN_SOURCE } from "../sections";

// Deliberately compact: the OSS foundation earns trust, but this page sells
// the Bowrain outcome. The kapi story lives on the kapi site.
export function OpenSource() {
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_OPEN_SOURCE);
  const ref = useReveal();

  // Hoisted so neokapi-i18n extracts the sentence as one block with a named
  // {kapiLink} placeholder (see Hero.tsx).
  const kapiLink = (
    <a
      href={KAPI_SITE_URL}
      className="underline underline-offset-2 transition hover:text-foreground"
    >
      kapi
    </a>
  );

  return (
    <section id="open-source" ref={sectionRef} className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            Built in the open.
          </h2>
          <p className="mt-3 text-muted-foreground">
            Bowrain is built on {kapiLink}, the Apache-2.0 toolchain for format-aware content
            processing: the same engine that reads and writes your formats, runs the checks, and
            keeps the memory. Your content, terminology, and memory stay in open formats you can
            take anywhere.
          </p>
        </div>

        <div className="mt-12 grid gap-6 md:grid-cols-3">
          <div className="rounded-xl border border-border bg-card p-6 text-center">
            <Shield className="mx-auto mb-3 h-8 w-8 text-success" />
            <h3 className="text-base font-semibold">Open toolchain</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              kapi and the neokapi framework are Apache-2.0. Inspect, fork, contribute, and run them
              on their own, without Bowrain.
            </p>
          </div>

          <div className="rounded-xl border border-border bg-card p-6 text-center">
            <Server className="mx-auto mb-3 h-8 w-8 text-primary" />
            <h3 className="text-base font-semibold">Self-host the platform</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              The Bowrain platform is available under AGPL or a commercial license. Run it on your
              own infrastructure with full control of your data.
            </p>
          </div>

          <div className="rounded-xl border border-border bg-card p-6 text-center">
            <Package className="mx-auto mb-3 h-8 w-8 text-foreground" />
            <h3 className="text-base font-semibold">No lock-in at any layer</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              Projects, terminology, and memory export in open formats. If you leave, your work
              leaves with you.
            </p>
          </div>
        </div>

        <p className="mt-8 text-center text-sm text-muted-foreground/80">
          <a
            href={GITHUB_URL}
            target="_blank"
            rel="noopener"
            className="underline underline-offset-2 transition hover:text-foreground"
          >
            github.com/neokapi
          </a>
        </p>
      </div>
    </section>
  );
}
