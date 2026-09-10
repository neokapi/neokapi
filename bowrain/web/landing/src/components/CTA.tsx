import { ArrowRight, MessageSquare } from "lucide-react";
import { BowArc } from "./Logo";
import { CONTACT_EMAIL, SIGNUP_URL } from "../links";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_CTA } from "../sections";

export function CTA() {
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_CTA);
  const ref = useReveal();

  return (
    <section
      id="get-started"
      ref={sectionRef}
      className="relative mx-auto max-w-6xl overflow-hidden px-6 py-24"
    >
      <div className="pointer-events-none absolute inset-x-0 bottom-0 h-[300px] overflow-hidden opacity-60">
        <BowArc className="absolute left-1/2 bottom-[-260px] h-[500px] w-[1200px] -translate-x-1/2" />
      </div>

      <div ref={ref} className="reveal relative z-10 mx-auto max-w-3xl text-center">
        <h2 className="font-display text-3xl font-semibold tracking-tight sm:text-4xl">
          Start with one project. Share the{" "}
          <span className="prism-text">decisions you approve.</span>
        </h2>
        <p className="mt-4 text-muted-foreground">
          Bring the content and writing guidance you already maintain. Review the terms and scope,
          then make that guidance available to the people and agents working on your projects.
        </p>

        <div className="mt-10 flex flex-col items-center gap-4 sm:flex-row sm:justify-center">
          <a
            href={SIGNUP_URL}
            className="group flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-6 py-3 text-base font-medium text-primary-foreground transition hover:opacity-90 sm:w-auto"
          >
            Start a workspace
            <ArrowRight className="h-5 w-5 transition group-hover:translate-x-0.5" />
          </a>
          <a
            href={`mailto:${CONTACT_EMAIL}`}
            className="flex w-full items-center justify-center gap-2 rounded-xl border border-border bg-card px-6 py-3 text-base font-medium transition hover:border-muted-foreground sm:w-auto"
          >
            <MessageSquare className="h-5 w-5" />
            Talk to us
          </a>
        </div>
      </div>
    </section>
  );
}
