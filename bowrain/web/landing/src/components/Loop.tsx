import { Bot, Users, Lightbulb, ShieldCheck, ArrowRight } from "lucide-react";
import { t } from "@neokapi/kapi-react/runtime";
import { useReveal } from "../useReveal";

const STEPS = [
  {
    icon: Bot,
    title: t("Draft"),
    body: t(
      "AI translates and drafts with your terminology, content memory, and brand voice in the prompt — not generic model output.",
    ),
  },
  {
    icon: Users,
    title: t("Review"),
    body: t(
      "Review in a shared editor: suggestions, term highlights, notes, history. Solo, it is your quality gate; with a team, live presence shows who's working where.",
    ),
  },
  {
    icon: Lightbulb,
    title: t("Learn"),
    body: t(
      "Accepted corrections update the shared memory. Recurring ones surface as candidate rules you can promote into the brand profile — or reject.",
    ),
  },
  {
    icon: ShieldCheck,
    title: t("Enforce"),
    body: t(
      "The next run drafts against the updated rules, in every locale and on every surface. Checks score drafts and flag violations before anything ships.",
    ),
  },
];

export function Loop() {
  const ref = useReveal();

  return (
    <section id="loop" className="relative mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-border px-3 py-1 font-mono text-xs text-muted-foreground">
            THE LEARNING LOOP
          </div>
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            Every correction makes the next draft better.
          </h2>
          <p className="mt-3 text-muted-foreground">
            Bowrain closes the loop between generating content and trusting it. Corrections don't
            evaporate in a review thread — they become versioned rules, enforced on every future
            generation.
          </p>
        </div>

        <div className="relative mt-12">
          {/* The loop line connecting the steps. */}
          <div className="prism-line absolute left-0 right-0 top-[52px] hidden h-px lg:block" />
          <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
            {STEPS.map((s, i) => (
              <div key={s.title} className="relative rounded-xl border border-border bg-card p-6">
                <div className="mb-4 flex items-center gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
                    <s.icon className="h-5 w-5 text-primary" />
                  </div>
                  <div translate="no" className="font-mono text-xs text-muted-foreground">
                    0{i + 1}
                  </div>
                </div>
                <h3 translate="no" className="text-lg font-semibold">
                  {s.title}
                </h3>
                <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{s.body}</p>
                {i < STEPS.length - 1 && (
                  <ArrowRight className="absolute -right-4 top-[44px] hidden h-4 w-4 text-muted-foreground/50 lg:block" />
                )}
              </div>
            ))}
          </div>
          <p className="mt-6 text-center text-xs text-muted-foreground/70">
            …and Enforce feeds Draft: the loop runs continuously as your source content changes.
          </p>
        </div>
      </div>
    </section>
  );
}
