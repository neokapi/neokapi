import { Apple, Monitor, Download, Globe2, Laptop, Terminal } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_APPS } from "../sections";
import { docsUrl, KAPI_SITE_URL } from "../links";

// Three places the same workspace is reachable from. The third is the point of
// the open core: a developer never has to leave their checkout to be inside the
// same graph as the reviewer.
const PLACES = [
  {
    icon: Globe2,
    where: t("In the browser"),
    body: t("Nothing to install. Projects, review queue and context, at app.bowrain.cloud."),
  },
  {
    icon: Laptop,
    where: t("As a desktop app"),
    body: t(
      "A signed native build for macOS, Windows and Linux. It keeps working offline and syncs when you reconnect.",
    ),
  },
  {
    icon: Terminal,
    where: t("From your checkout"),
    body: t(
      "kapi resolves the same context locally, gates the same checks in CI, and answers your agents over MCP.",
    ),
  },
];

export function Apps() {
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_APPS);
  const ref = useReveal();

  return (
    <section id="apps" ref={sectionRef} className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-border px-3 py-1 font-mono text-xs text-muted-foreground">
            WHERE YOU WORK
          </div>
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            {t("One workspace,")} <span className="prism-text">{t("three ways in.")}</span>
          </h2>
          <p className="mt-3 text-muted-foreground">
            {t(
              "Same projects, same context, same review queue, whichever door you come through, because all three read the graph rather than a copy of it.",
            )}
          </p>
        </div>

        <div className="mt-12 grid gap-6 md:grid-cols-3">
          {PLACES.map((p) => (
            <div key={p.where} className="rounded-xl border border-border bg-card p-6">
              <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
                <p.icon className="h-5 w-5 text-primary" />
              </div>
              <h3 className="text-base font-semibold">{p.where}</h3>
              <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{p.body}</p>
            </div>
          ))}
        </div>

        {/* macOS ships via the Homebrew cask; Windows and Linux builds are
            published to GitHub Releases. The install docs cover all three. */}
        <div className="mt-8 flex flex-wrap items-center justify-center gap-4">
          <a
            href={`${docsUrl()}installation`}
            className="flex items-center gap-2 rounded-lg border border-border bg-card px-5 py-2.5 text-sm transition hover:border-muted-foreground"
          >
            <Apple className="h-4 w-4" />
            macOS
          </a>
          <a
            href="https://github.com/neokapi/neokapi/releases"
            target="_blank"
            rel="noopener"
            className="flex items-center gap-2 rounded-lg border border-border bg-card px-5 py-2.5 text-sm transition hover:border-muted-foreground"
          >
            <Monitor className="h-4 w-4" />
            Windows
          </a>
          <a
            href="https://github.com/neokapi/neokapi/releases"
            target="_blank"
            rel="noopener"
            className="flex items-center gap-2 rounded-lg border border-border bg-card px-5 py-2.5 text-sm transition hover:border-muted-foreground"
          >
            <Download className="h-4 w-4" />
            Linux
          </a>
        </div>
        <p className="mt-3 text-center text-xs text-muted-foreground/70">
          macOS installs with{" "}
          <code translate="no" className="rounded bg-secondary px-1 text-secondary-foreground">
            brew install --cask neokapi/tap/bowrain
          </code>
          .{" "}
          <a
            href={KAPI_SITE_URL}
            className="underline underline-offset-2 transition hover:text-foreground"
          >
            kapi
          </a>{" "}
          installs on its own and needs no account.
        </p>
      </div>
    </section>
  );
}
