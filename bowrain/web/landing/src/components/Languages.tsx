import { useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_LANGUAGES } from "../sections";
import { captureLandingEvent } from "../analytics";
import { markEngaged } from "../sectionSignals";

// Illustrate locale readiness from coverage, checks and review decisions.

type ShipState = "governed" | "ai_shippable" | "pending";

interface LocaleRow {
  locale: string;
  code: string;
  translated: number;
  total: number;
  approved: number;
  failingChecks: number;
}

/** Mirrors bowrain/core/store.DeriveShipState. */
function deriveShipState(row: LocaleRow): ShipState {
  if (row.total === 0 || row.translated < row.total || row.failingChecks > 0) return "pending";
  return row.approved >= row.total ? "governed" : "ai_shippable";
}

const LOCALES: LocaleRow[] = [
  { locale: "Deutsch", code: "de", translated: 412, total: 412, approved: 412, failingChecks: 0 },
  {
    locale: "Norsk bokmål",
    code: "nb",
    translated: 412,
    total: 412,
    approved: 412,
    failingChecks: 0,
  },
  { locale: "Français", code: "fr", translated: 412, total: 412, approved: 268, failingChecks: 0 },
  { locale: "Español", code: "es", translated: 412, total: 412, approved: 91, failingChecks: 0 },
  {
    locale: "Português (Brasil)",
    code: "pt-BR",
    translated: 412,
    total: 412,
    approved: 340,
    failingChecks: 3,
  },
  { locale: "日本語", code: "ja", translated: 398, total: 412, approved: 210, failingChecks: 0 },
];

const STATE_STYLES: Record<ShipState, string> = {
  governed: "bg-success/10 text-success",
  ai_shippable: "bg-info/10 text-info",
  pending: "bg-muted text-muted-foreground",
};

// Each delivery policy accepts a set of precomputed ship states.
const BARS = [
  { id: "governed", label: t("Ship what a person approved"), accepts: ["governed"] as ShipState[] },
  {
    id: "checks",
    label: t("Ship what passes checks"),
    accepts: ["governed", "ai_shippable"] as ShipState[],
  },
];

export function Languages() {
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_LANGUAGES);
  const ref = useReveal();
  const [bar, setBar] = useState(BARS[0].id);
  const accepts = BARS.find((b) => b.id === bar)!.accepts;

  function selectBar(id: string) {
    setBar(id);
    markEngaged("ship-states");
    captureLandingEvent("ship_state_explored", { bar: id });
  }

  const stateLabels: Record<ShipState, string> = {
    governed: t("governed"),
    ai_shippable: t("AI-shippable"),
    pending: t("pending"),
  };

  const rows = LOCALES.map((row) => ({ ...row, state: deriveShipState(row) }));
  const shippable = rows.filter((r) => accepts.includes(r.state));

  return (
    <section id="languages" ref={sectionRef} className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-border px-3 py-1 font-mono text-xs text-muted-foreground">
            ONE MORE AXIS
          </div>
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            {t("A language is")} <span className="prism-text">{t("one more coordinate.")}</span>
          </h2>
          <p className="mt-3 text-muted-foreground">
            {t(
              "Use language coordinates to select the terms and writing guidance that apply to each locale. Translation coverage, check results and review decisions determine which locales meet your delivery policy.",
            )}
          </p>
        </div>

        <div className="mt-12 grid gap-8 lg:grid-cols-[minmax(0,20rem)_1fr]">
          <div>
            <h3 className="text-base font-semibold">{t("How readiness is determined")}</h3>
            <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
              {t(
                "Readiness depends on translation coverage, failing checks and recorded approvals.",
              )}
            </p>
            <dl className="mt-5 space-y-4 text-sm">
              <div>
                <dt
                  translate="no"
                  className="font-mono text-xs uppercase tracking-wider text-success"
                >
                  governed
                </dt>
                <dd className="mt-1 text-muted-foreground">
                  {t(
                    "Every block translated, no failing check, every target carrying a review decision.",
                  )}
                </dd>
              </div>
              <div>
                <dt translate="no" className="font-mono text-xs uppercase tracking-wider text-info">
                  ai_shippable
                </dt>
                <dd className="mt-1 text-muted-foreground">
                  {t(
                    "Every block translated and no failing check, but review is incomplete: shippable on machine review only.",
                  )}
                </dd>
              </div>
              <div>
                <dt
                  translate="no"
                  className="font-mono text-xs uppercase tracking-wider text-muted-foreground"
                >
                  pending
                </dt>
                <dd className="mt-1 text-muted-foreground">
                  {t("Anything less: partial coverage, a failing check, or nothing to ship yet.")}
                </dd>
              </div>
            </dl>
          </div>

          <div className="overflow-hidden rounded-xl border border-border bg-card">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-2.5">
              <div className="flex flex-wrap gap-2">
                {BARS.map((b) => (
                  <button
                    key={b.id}
                    type="button"
                    onClick={() => selectBar(b.id)}
                    aria-pressed={b.id === bar}
                    className={`rounded-md px-3 py-1 text-xs transition ${
                      bar === b.id
                        ? "bg-primary/10 text-primary"
                        : "text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    {b.label}
                  </button>
                ))}
              </div>
              <span className="text-xs text-muted-foreground">
                {t("{count} of {total} clear the bar", {
                  count: shippable.length,
                  total: rows.length,
                })}
              </span>
            </div>

            <table className="w-full text-sm">
              <caption className="sr-only">
                {t("Per-locale delivery status against the selected bar")}
              </caption>
              <thead>
                <tr className="border-b border-border/60 text-left text-xs uppercase tracking-wider text-muted-foreground">
                  <th scope="col" className="px-4 py-2 font-normal">
                    {t("Locale")}
                  </th>
                  <th scope="col" className="px-4 py-2 font-normal">
                    {t("Coverage")}
                  </th>
                  <th scope="col" className="px-4 py-2 font-normal">
                    {t("Checks")}
                  </th>
                  <th scope="col" className="px-4 py-2 font-normal">
                    {t("State")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => {
                  const clears = accepts.includes(row.state);
                  return (
                    <tr
                      key={row.code}
                      className={`border-b border-border/40 transition last:border-0 ${
                        clears ? "" : "opacity-45"
                      }`}
                    >
                      <td className="px-4 py-2.5">
                        <span className="font-medium">{row.locale}</span>{" "}
                        <span translate="no" className="font-mono text-xs text-muted-foreground">
                          {row.code}
                        </span>
                      </td>
                      <td
                        translate="no"
                        className="px-4 py-2.5 font-mono text-xs text-muted-foreground"
                      >
                        {Math.round((row.translated / row.total) * 100)}%
                      </td>
                      <td
                        translate="no"
                        className="px-4 py-2.5 font-mono text-xs text-muted-foreground"
                      >
                        {row.failingChecks === 0 ? "·" : `−${row.failingChecks}`}
                      </td>
                      <td className="px-4 py-2.5">
                        <span
                          translate="no"
                          className={`inline-flex rounded-md px-2 py-0.5 text-xs ${STATE_STYLES[row.state]}`}
                        >
                          {stateLabels[row.state]}
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>

        <p className="mx-auto mt-10 max-w-2xl text-center text-muted-foreground">
          {t(
            "Set the bar where the content sits: a legal notice waits for a person, a help article may not have to. The delivery view answers one question (what can go out today) and answers it the same way every time.",
          )}
        </p>
      </div>
    </section>
  );
}
