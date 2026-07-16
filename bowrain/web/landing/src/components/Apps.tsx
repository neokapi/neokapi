import { Apple, Monitor, Download } from "lucide-react";
import { useReveal } from "../useReveal";
import { docsUrl } from "../links";

export function Apps() {
  const base = import.meta.env.BASE_URL;
  const ref = useReveal();

  return (
    <section id="apps" className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-border px-3 py-1 font-mono text-xs text-muted-foreground">
            WEB & DESKTOP
          </div>
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            The same workspace, wherever you work.
          </h2>
          <p className="mt-3 text-muted-foreground">
            Bowrain runs in the browser and as a native desktop app — same projects, same memory,
            same terminology, same review queue. The desktop app keeps working offline and syncs
            your changes when you reconnect.
          </p>
        </div>

        {/* Real desktop screenshot, captured from the live app. */}
        <div className="mt-12 overflow-hidden rounded-xl border border-border bg-card shadow-2xl shadow-primary/5">
          <div className="flex items-center gap-2 border-b border-border bg-muted px-4 py-2.5">
            <div className="h-2.5 w-2.5 rounded-full bg-destructive/60" />
            <div className="h-2.5 w-2.5 rounded-full bg-warning/60" />
            <div className="h-2.5 w-2.5 rounded-full bg-success/60" />
            <span className="ml-2 text-xs text-muted-foreground">Bowrain — Flows</span>
          </div>
          <img
            src={`${base}desktop-flows.png`}
            alt="The Bowrain flow builder: a visual node graph wiring Input → AI Translate → QA Check → Output, with a library of built-in flows."
            className="block w-full"
          />
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
          <code className="rounded bg-secondary px-1 text-secondary-foreground">
            brew install --cask neokapi/tap/bowrain
          </code>
          . Works offline; changes sync automatically when you reconnect.
        </p>
      </div>
    </section>
  );
}
