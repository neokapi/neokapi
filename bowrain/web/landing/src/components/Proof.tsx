import { t } from "@neokapi/i18n-react/runtime";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_PROOF } from "../sections";
import { VoiceCheck } from "./VoiceCheck";

// Local rule illustration. Production checks resolve configured project
// context through kapi; this widget only evaluates its listed English patterns.
export function Proof() {
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_PROOF);
  const ref = useReveal();

  return (
    <section id="proof" ref={sectionRef} className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-border px-3 py-1 font-mono text-xs text-muted-foreground">
            LOCAL ENGLISH RULE EXAMPLE
          </div>
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            {t("Inspect the rule.")}{" "}
            <span className="prism-text">{t("Understand the finding.")}</span>
          </h2>
          <p className="mt-3 text-muted-foreground">
            {t(
              "Choose an example context and edit the English draft. This widget runs a fixed set of local wording patterns and shows their findings. It illustrates how a term can be permitted in help content and prohibited in an API reference.",
            )}
          </p>
        </div>

        <VoiceCheck />

        <p className="mx-auto mt-6 max-w-2xl text-center text-sm text-muted-foreground">
          {t(
            "The score subtracts the penalties for these example rules from 100. No production profile lookup runs here. Factual accuracy, overall writing quality and audience suitability are not assessed. In your project, inspect the configured checks and their results before approving content.",
          )}
        </p>

        {/* Where the same check runs, in the order a team meets it. */}
        <div className="mt-12 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-3">
          {[
            {
              where: t("In the editor"),
              body: t(
                "Inspect findings and suggestions beside the draft, then review the proposed change.",
              ),
            },
            {
              where: t("In your pipeline"),
              body: t(
                "A configured failing gate exits non-zero, so CI can require the reported issues to be resolved.",
              ),
            },
            {
              where: t("In your agents"),
              body: t(
                "Use MCP to retrieve applicable guidance before writing and inspect check findings after an edit.",
              ),
            },
          ].map((c) => (
            <div key={c.where} className="bg-card px-5 py-5">
              <div className="text-sm font-medium">{c.where}</div>
              <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{c.body}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
