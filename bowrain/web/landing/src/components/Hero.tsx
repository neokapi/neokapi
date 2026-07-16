import { ArrowRight, Play } from "lucide-react";
import { useRef, useState } from "react";
import { BowArc } from "./Logo";
import { GITHUB_URL, KAPI_SITE_URL, SIGNUP_URL } from "../links";

export function Hero() {
  const base = import.meta.env.BASE_URL;

  // The sizzle reel is click-to-play (no autoplay on load): the poster shows
  // until the visitor presses play.
  const videoRef = useRef<HTMLVideoElement>(null);
  const [playing, setPlaying] = useState(false);
  const playSizzle = () => {
    setPlaying(true);
    void videoRef.current?.play();
  };

  return (
    <section className="relative flex flex-col items-center overflow-hidden px-6 pb-20 pt-36 grain">
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
          On-brand, in every language,{" "}
          <span className="prism-text block">everywhere you publish.</span>
        </h1>

        <p className="animate-fade-in-up-delay-2 mx-auto mt-6 max-w-xl text-lg text-muted-foreground md:text-xl">
          Bowrain keeps your content caught up and on brand across your app, site, docs, and posts.
          AI drafts against your brand voice, terminology, and translation memory — you review — and
          every correction becomes a rule the next draft follows. Built for one person with many
          surfaces, ready for a team.
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
            href="#loop"
            className="flex w-full items-center justify-center gap-2 rounded-xl border border-border bg-card/60 px-6 py-3 text-base font-medium transition hover:border-muted-foreground sm:w-auto"
          >
            How it works
          </a>
        </div>

        {/* kapi is the open foundation, deliberately a footnote to the outcome above. */}
        <p className="mt-6 text-sm text-muted-foreground/80">
          Open core — built on the Apache-2.0{" "}
          <a
            href={KAPI_SITE_URL}
            className="underline-offset-2 transition hover:text-foreground hover:underline"
          >
            kapi
          </a>{" "}
          toolchain.{" "}
          <a
            href={GITHUB_URL}
            target="_blank"
            rel="noopener"
            className="underline-offset-2 transition hover:text-foreground hover:underline"
          >
            View on GitHub
          </a>
        </p>
      </div>

      {/* Platform sizzle — a montage of the bowrain feature walkthroughs, produced
          by the harness from the same recordings as the docs videos. Served by the
          sibling docs deploy (/web/bowrain/docs/… today, /docs/… at launch); the
          poster is a committed still in the landing's own public/. */}
      <div className="animate-fade-in-up-delay-3 relative z-10 mt-16 w-full max-w-5xl">
        <div className="relative overflow-hidden rounded-xl border border-border shadow-2xl shadow-primary/10">
          <video
            ref={videoRef}
            className="block w-full"
            muted
            loop
            playsInline
            controls={playing}
            poster={`${base}sizzle.jpg`}
            aria-label="A montage of the Bowrain platform: the shared editor, real-time collaboration, translation memory and terminology, review and approval, and corrections that become versioned checks."
          >
            <source
              src={`${base}docs/video/bowrain-web/bowrain-sizzle-dark.webm`}
              type="video/webm"
            />
          </video>
          {!playing && (
            <button
              type="button"
              onClick={playSizzle}
              aria-label="Play the platform overview"
              className="group absolute inset-0 flex items-center justify-center bg-foreground/10 transition-colors hover:bg-foreground/5"
            >
              <span className="flex h-16 w-16 items-center justify-center rounded-full bg-card/95 shadow-lg transition-transform group-hover:scale-105">
                <Play className="ml-1 h-7 w-7 text-foreground" fill="currentColor" />
              </span>
            </button>
          )}
        </div>
        <p className="mt-3 text-center text-xs text-muted-foreground/70">
          The platform at a glance — a shared editor, real-time collaboration, memory and
          terminology, review, and checks that learn from every correction.
        </p>
      </div>
    </section>
  );
}
