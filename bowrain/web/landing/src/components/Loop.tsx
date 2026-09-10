import { SearchCheck, PenTool, GitCompareArrows, ShieldCheck, ArrowRight } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_LOOP } from "../sections";

// Four steps, numbered, because this genuinely is a sequence that closes on
// itself: what Enforce holds is what Discover proposed and Decide promoted.
const STEPS = [
  {
    icon: SearchCheck,
    title: t("Discover"),
    body: t(
      "Use existing content and writing guidance to propose an initial profile. Review the proposed terms and scope before applying them to a project.",
    ),
  },
  {
    icon: PenTool,
    title: t("Correct"),
    body: t(
      "Record a correction with its content context. Repeated corrections can provide evidence for a candidate rule.",
    ),
  },
  {
    icon: GitCompareArrows,
    title: t("Decide"),
    body: t(
      "Inspect recurring corrections and preview the content a candidate rule would flag. A reviewer explicitly promotes the rule into a versioned voice profile.",
    ),
  },
  {
    icon: ShieldCheck,
    title: t("Enforce"),
    body: t(
      "Run the promoted rule where that profile applies. Findings identify wording that violates the configured rule; reviewers remain responsible for decisions beyond its coverage.",
    ),
  },
];

export function Loop() {
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_LOOP);
  const ref = useReveal();

  return (
    <section id="loop" ref={sectionRef} className="relative mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-border px-3 py-1 font-mono text-xs text-muted-foreground">
            THE LOOP
          </div>
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            {t("Review the decision.")}{" "}
            <span className="prism-text">{t("Keep its source and scope.")}</span>
          </h2>
          <p className="mt-3 text-muted-foreground">
            {t(
              "A recurring correction can become a candidate rule. Review its evidence and affected content, then explicitly promote the decision into a versioned profile for the projects that use it.",
            )}
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
            {t(
              "Review findings and later corrections to decide whether a rule should be retained, revised or removed.",
            )}
          </p>
        </div>
      </div>
    </section>
  );
}
