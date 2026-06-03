import { ArrowRight, BookOpen } from "lucide-react";

export function Hero() {
  // Relative to the deploy base (see Nav): /web/bowrain/ today, / at launch.
  const base = import.meta.env.BASE_URL;
  const docsUrl = `${base}docs/`;

  return (
    <section className="relative flex flex-col items-center px-6 pb-20 pt-32">
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute left-1/2 top-1/4 h-[600px] w-[600px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-brand-500/5 blur-[120px]" />
      </div>

      <div className="relative z-10 mx-auto max-w-3xl text-center">
        <div className="animate-fade-in-up mb-6 inline-flex items-center gap-2 rounded-full border border-neutral-800 bg-neutral-900/50 px-4 py-1.5 text-sm text-neutral-400">
          <span className="h-1.5 w-1.5 rounded-full bg-suggestion" />
          Content governance for teams
        </div>

        <h1 className="animate-fade-in-up-delay-1 text-4xl font-bold leading-[1.05] tracking-tight text-white sm:text-5xl md:text-6xl lg:text-7xl">
          Your content checks,{" "}
          <span className="bg-gradient-to-r from-brand-400 to-brand-300 bg-clip-text text-transparent">
            learning from every correction.
          </span>
        </h1>

        <p className="animate-fade-in-up-delay-2 mx-auto mt-6 max-w-xl text-lg text-neutral-400 md:text-xl">
          Bowrain is where a team governs AI-generated content: a shared brand voice, terminology,
          and translation memory — and a closed loop that turns every human correction into a
          versioned check, enforced on every future generation. Collaborative editing, review, and
          history, built in.
        </p>

        <div className="animate-fade-in-up-delay-3 mt-10 flex flex-col items-center gap-4 sm:flex-row sm:justify-center">
          <a
            href="#plans"
            className="group flex w-full items-center justify-center gap-2 rounded-xl bg-brand-500 px-6 py-3 text-base font-medium text-white transition hover:bg-brand-600 sm:w-auto"
          >
            Get started
            <ArrowRight className="h-5 w-5 transition group-hover:translate-x-0.5" />
          </a>
          <a
            href={docsUrl}
            className="group flex w-full items-center justify-center gap-2 rounded-xl border border-neutral-700 bg-neutral-900/50 px-6 py-3 text-base font-medium text-neutral-200 transition hover:border-neutral-500 hover:text-white sm:w-auto"
          >
            <BookOpen className="h-5 w-5" />
            Read the docs
          </a>
        </div>

        {/* kapi is the open foundation, deliberately secondary to the outcome above. */}
        <p className="mt-6 text-sm text-neutral-500">
          Open core, built on the{" "}
          <a
            href="https://neokapi.github.io/web/neokapi/"
            className="text-neutral-400 underline-offset-2 transition hover:text-neutral-300 hover:underline"
          >
            kapi
          </a>{" "}
          toolchain —{" "}
          <a
            href="https://github.com/neokapi"
            target="_blank"
            rel="noopener"
            className="text-neutral-400 underline-offset-2 transition hover:text-neutral-300 hover:underline"
          >
            view on GitHub
          </a>
        </p>
      </div>

      {/* Real product demo — the translation editor, recorded from the live app.
          Served by the sibling docs deploy (/web/bowrain/docs/… today, /docs/… at
          launch), so it is never committed to git; the poster is a real screenshot
          in the landing's own public/, shown until the video loads (and in isolated
          landing-only dev, where the docs path isn't served). */}
      <div className="animate-fade-in-up-delay-3 relative z-10 mt-16 w-full max-w-5xl">
        <div className="overflow-hidden rounded-xl border border-neutral-800 bg-neutral-900/50 shadow-2xl shadow-brand-500/10">
          <div className="flex items-center gap-2 border-b border-neutral-800 px-4 py-3">
            <div className="h-2.5 w-2.5 rounded-full bg-red-500/60" />
            <div className="h-2.5 w-2.5 rounded-full bg-yellow-500/60" />
            <div className="h-2.5 w-2.5 rounded-full bg-green-500/60" />
            <span className="ml-2 font-mono text-xs text-neutral-600">
              Bowrain — Company Website · en → fr
            </span>
          </div>
          <video
            className="block w-full"
            autoPlay
            muted
            loop
            playsInline
            poster={`${base}editor.png`}
            aria-label="The Bowrain translation editor: source and target side by side, with translation-memory and terminology matches in a context panel and a live document preview."
          >
            <source src={`${base}docs/video/bowrain-web/bowrain-web-editor-dark.webm`} type="video/webm" />
          </video>
        </div>
        <p className="mt-3 text-center text-xs text-neutral-600">
          The shared editor — translation memory and terminology matches inline, a live preview,
          every locale.
        </p>
      </div>
    </section>
  );
}
