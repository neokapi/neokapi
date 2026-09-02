import { t } from "@neokapi/i18n-react/runtime";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_PROOF } from "../sections";
import { VoiceCheck } from "./VoiceCheck";

// Where the argument has to stop being an argument.
//
// The proof is a check the reader runs here, on the page, against the same
// arithmetic the product uses — followed by the three places that same check
// fires. A reader who moves the sentence from clean to critical by changing
// only the point has verified the claim themselves, which no screenshot of the
// product doing it for them can match.
export function Proof() {
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_PROOF);
  const ref = useReveal();

  return (
    <section id="proof" ref={sectionRef} className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-border px-3 py-1 font-mono text-xs text-muted-foreground">
            TRY IT: THE CHECK RUNS IN THIS PAGE
          </div>
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            {t("A check is")} <span className="prism-text">{t("a test for prose.")}</span>
          </h2>
          <p className="mt-3 text-muted-foreground">
            {t(
              "A rule in a document is advice. Bound to a point, it is executable: the check resolves the profile in force, scores the draft against it, and reports what it penalised and why. The bar is yours, and under it the run fails. Pick a point below to see the rules in force there, and edit the draft to watch the score follow.",
            )}
          </p>
        </div>

        <VoiceCheck />

        <p className="mx-auto mt-6 max-w-2xl text-center text-sm text-muted-foreground">
          {t(
            "Findings carry a severity, severity carries a penalty, and the score is what is left of 100. The arithmetic in this page is the arithmetic in the product. The same check gates a pull request, sits in the editor beside the draft, and answers your agents before they write, because all three resolve the same point.",
          )}
        </p>

        {/* Where the same check runs, in the order a team meets it. */}
        <div className="mt-12 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-3">
          {[
            {
              where: t("In the editor"),
              body: t(
                "Beside the draft, as it is written, with the finding and the suggestion inline.",
              ),
            },
            {
              where: t("In your pipeline"),
              body: t(
                "A failing gate exits non-zero, so content that is off profile does not merge.",
              ),
            },
            {
              where: t("In your agents"),
              body: t(
                "Over MCP: the agent asks what applies here before it writes, not after it is corrected.",
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
