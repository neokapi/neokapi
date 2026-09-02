import { t } from "@neokapi/i18n-react/runtime";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_RENAME } from "../sections";

// The rename is the argument, not an illustration of it.
//
// Every company has renamed something, so the reader needs no product knowledge
// to follow it — and it kills "just keep a list of preferred and banned terms"
// before they think of it, because the same word is required in one place and
// forbidden in another. That is why this is the FIRST section after the hero:
// it earns "context graph" before the phrase is used.
const RULES = [
  {
    surface: t("Help articles"),
    rule: t("Use the new name"),
    tone: "ok" as const,
  },
  {
    surface: t("The API parameter"),
    rule: t("Must not change"),
    tone: "hold" as const,
  },
  {
    surface: t("The changelog"),
    rule: t("Keeps the old name, which is what shipped"),
    tone: "hold" as const,
  },
  {
    surface: t("The migration guide"),
    rule: t("Needs both, side by side"),
    tone: "both" as const,
  },
  {
    surface: t("Support replies"),
    rule: t("Recognise the old name for a year, never use it"),
    tone: "expire" as const,
  },
];

const TONE_STYLES: Record<string, string> = {
  ok: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
  hold: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
  both: "bg-sky-500/10 text-sky-600 dark:text-sky-400",
  expire: "bg-violet-500/10 text-violet-600 dark:text-violet-400",
};

export function Rename() {
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_RENAME);
  const ref = useReveal();

  return (
    <section id="rename" ref={sectionRef} className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            {t("One decision.")} <span className="prism-text">{t("Five different rules.")}</span>
          </h2>
          <p className="mt-3 text-muted-foreground">
            {t(
              "You rename a feature. That is one decision, and it lands differently in every place the words appear.",
            )}
          </p>
        </div>

        <div className="mx-auto mt-12 max-w-3xl overflow-hidden rounded-xl border border-border/60">
          {RULES.map((r, i) => (
            <div
              key={r.surface}
              className={`grid grid-cols-1 gap-1 px-5 py-4 sm:grid-cols-[minmax(0,14rem)_1fr] sm:gap-6 ${
                i > 0 ? "border-t border-border/50" : ""
              }`}
            >
              <div className="font-medium">{r.surface}</div>
              <div className="flex items-start gap-3">
                <span
                  aria-hidden="true"
                  className={`mt-1.5 size-2 shrink-0 rounded-full ${TONE_STYLES[r.tone]}`}
                />
                <span className="text-muted-foreground">{r.rule}</span>
              </div>
            </div>
          ))}
        </div>

        <p className="mx-auto mt-8 max-w-2xl text-center text-muted-foreground">
          {t(
            "Written down as a list of preferred and banned terms, that is wrong nearly everywhere it lands. Which rule applies depends entirely on where the words sit, so the thing worth writing down is the place rather than the list.",
          )}
        </p>
      </div>
    </section>
  );
}
