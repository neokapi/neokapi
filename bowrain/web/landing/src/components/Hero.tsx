import { ArrowRight } from "lucide-react";
import { BowArc } from "./Logo";
import { GITHUB_URL, KAPI_SITE_URL, SIGNUP_URL } from "../links";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_HERO } from "../sections";

export function Hero() {
  // The hero carries an id so its CTA — the most-clicked link on the page —
  // attributes to a section rather than to null.
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_HERO);

  // Hoisted inline links: keeping these as JSX-expression variables (rather
  // than inline elements split across prettier-wrapped lines with {" "}
  // spacers) lets neokapi-i18n extract the surrounding sentences as single
  // blocks with named {kapiLink}/{githubLink} placeholders.
  const kapiLink = (
    <a
      href={KAPI_SITE_URL}
      className="underline-offset-2 transition hover:text-foreground hover:underline"
    >
      kapi
    </a>
  );
  const githubLink = (
    <a
      href={GITHUB_URL}
      target="_blank"
      rel="noopener"
      className="underline-offset-2 transition hover:text-foreground hover:underline"
    >
      View on GitHub
    </a>
  );

  return (
    <section
      id="hero"
      ref={sectionRef}
      className="relative flex flex-col items-center overflow-hidden px-6 pb-24 pt-36 grain"
    >
      {/* Atmosphere: the bow arc rising behind the headline. */}
      <div className="pointer-events-none absolute inset-x-0 top-0 h-[560px] overflow-hidden">
        <BowArc className="absolute left-1/2 top-10 h-[500px] w-[1400px] -translate-x-1/2 opacity-70" />
        <div
          className="absolute left-1/2 top-1/3 h-[480px] w-[900px] -translate-x-1/2 -translate-y-1/2 rounded-full blur-[140px]"
          style={{
            background:
              "linear-gradient(100deg, color-mix(in oklab, var(--prism-4) 12%, transparent), color-mix(in oklab, var(--prism-3) 10%, transparent), color-mix(in oklab, var(--prism-2) 8%, transparent))",
          }}
        />
      </div>

      <div className="relative z-10 mx-auto max-w-3xl text-center">
        <div className="animate-fade-in-up mb-6 inline-flex items-center gap-2 rounded-full border border-border bg-card/60 px-4 py-1.5 text-sm text-muted-foreground">
          <span className="h-1.5 w-1.5 rounded-full bg-success" />
          Content operations for everything you publish
        </div>

        <h1 className="animate-fade-in-up-delay-1 font-display text-4xl font-semibold leading-[1.08] tracking-tight sm:text-5xl md:text-6xl">
          The context graph
          <span className="prism-text block">for your content.</span>
        </h1>

        <p className="animate-fade-in-up-delay-2 mx-auto mt-6 max-w-xl text-lg text-muted-foreground md:text-xl">
          Human communication is contextual. Bowrain is the graph your people and your AI agents
          plug into. A legal notice is not a help article, so the rules that fix voice and tone move
          with the audience, the surface and the moment. Recorded once, applied everywhere,
          connected to the systems your content already lives in.
        </p>

        <div className="animate-fade-in-up-delay-3 mt-10 flex flex-col items-center gap-4 sm:flex-row sm:justify-center">
          <a
            href={SIGNUP_URL}
            className="group flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-6 py-3 text-base font-medium text-primary-foreground transition hover:opacity-90 sm:w-auto"
          >
            Start free
            <ArrowRight className="h-5 w-5 transition group-hover:translate-x-0.5" />
          </a>
          <a
            href="#how"
            className="flex w-full items-center justify-center gap-2 rounded-xl border border-border bg-card/60 px-6 py-3 text-base font-medium transition hover:border-muted-foreground sm:w-auto"
          >
            How it works
          </a>
        </div>

        {/* kapi is the open foundation, deliberately a footnote to the outcome above. */}
        <p className="mt-6 text-sm text-muted-foreground/80">
          Open core, built on the Apache-2.0 {kapiLink} toolchain. {githubLink}
        </p>
      </div>
    </section>
  );
}
