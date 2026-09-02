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
      "Point it at what you already publish: the repository, the site, whatever style guide exists. It proposes the profiles, the coordinates they sit at and the starting vocabulary, and you correct a first draft instead of authoring one.",
    ),
  },
  {
    icon: PenTool,
    title: t("Correct"),
    body: t(
      "People edit where they already work. Each correction is captured with the point it was made at, so the same fix does not have to be made twenty times before anything remembers it.",
    ),
  },
  {
    icon: GitCompareArrows,
    title: t("Decide"),
    body: t(
      "Recurring corrections surface as candidate rules. A proposal shows its reach over existing content before anyone approves it, and can be trialled on one stream of real content before it merges, so a rule change is a measured experiment rather than a leap.",
    ),
  },
  {
    icon: ShieldCheck,
    title: t("Enforce"),
    body: t(
      "A promoted rule holds from then on: in the checks that gate a change, in what your agents are told applies here, and in every draft the next run produces. Rules start and stop on a date, so a campaign or a policy change expires on its own.",
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
            {t("The context comes from the work.")}{" "}
            <span className="prism-text">{t("Not from a workshop.")}</span>
          </h2>
          <p className="mt-3 text-muted-foreground">
            {t(
              "An empty graph is worth nothing, and nobody fills one in from a blank page. So the first act is discovery, and every act after it is the same loop: corrections aggregate into candidate rules, rules are promoted with provenance and a version, and what was decided is enforced from then on. The context improves as a by-product of people doing their jobs, which is the only way it stays current.",
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
              "…and Enforce feeds Discover: the rules you keep are the ones the work produced. There is no separate onboarding mode to maintain.",
            )}
          </p>
        </div>
      </div>
    </section>
  );
}
