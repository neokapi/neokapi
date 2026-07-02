import { CheckCircle2, Loader2, PauseCircle } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, LocalePill } from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { RunEvent } from "../context/JobFeedContext";
import type { ConvergeOutput } from "../types/api";

export interface ConvergeRunViewProps {
  /** The run's event stream (from the job feed). */
  events: RunEvent[];
  /** Whether the run is still in flight (renders the working row). */
  running?: boolean;
  /** Open the Review page filtered to a parked (collection, locale) scope. */
  onOpenReview?: (scope: { collection?: string; locale?: string }) => void;
}

/**
 * The convergence view of a "Bring up to date" run: one row per pass
 * ("pass N: extracted X, produced Y, checks failing Z") instead of raw flow
 * logs, and — once the run settles — the structured outcome with each parked
 * (collection, locale) scope rendered as a deep link into the Review page.
 */
export function ConvergeRunView({ events, running, onOpenReview }: ConvergeRunViewProps) {
  const passes = events.filter((e) => e.type === "converge_pass" && e.converge);
  const result: ConvergeOutput | undefined = events.find(
    (e) => e.type === "complete" && e.converge_result,
  )?.converge_result;

  return (
    <Card data-slot="converge-run-view">
      <CardHeader className="px-4">
        <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          {t("Passes")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 px-4">
        {passes.length === 0 && running && (
          <p className="flex items-center gap-2 text-xs text-muted-foreground">
            <Loader2 size={12} className="animate-spin" />
            {t("Deriving state and running the first pass…")}
          </p>
        )}

        <ol className="space-y-1.5">
          {passes.map((e, i) => {
            const p = e.converge!;
            const pending = p.pendingLocales ?? [];
            return (
              <li
                key={i}
                className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs"
                data-slot="converge-pass-row"
              >
                <span className="font-medium">{t("Pass {n}", { n: p.pass })}</span>
                <span className="text-muted-foreground">
                  {t("extracted {count}", { count: p.extractedBlocks ?? 0 })}
                </span>
                <span className="text-muted-foreground">·</span>
                <span className="text-muted-foreground">
                  {t("produced {count}", { count: p.produced })}
                  {p.producedDelta > 0 ? ` (+${p.producedDelta})` : ""}
                </span>
                <span className="text-muted-foreground">·</span>
                <span
                  className={
                    (p.failingChecks ?? 0) > 0
                      ? "text-amber-600 dark:text-amber-500"
                      : "text-muted-foreground"
                  }
                >
                  {t("checks failing {count}", { count: p.failingChecks ?? 0 })}
                </span>
                <span className="text-muted-foreground">·</span>
                <span className="text-muted-foreground">
                  {t("parked {count}", { count: pending.length })}
                </span>
                {pending.map((loc) => (
                  <LocalePill key={loc} locale={loc} />
                ))}
              </li>
            );
          })}
          {running && passes.length > 0 && (
            <li className="flex items-center gap-2 text-xs text-muted-foreground">
              <Loader2 size={12} className="animate-spin" />
              {t("Running…")}
            </li>
          )}
        </ol>

        {result && (
          <div className="border-t border-border pt-2" data-slot="converge-result">
            {result.converged ? (
              <p className="flex items-center gap-1.5 text-xs text-green-600 dark:text-green-500">
                <CheckCircle2 size={13} />
                {t("Converged in {count} pass(es) — every gated scope is shippable.", {
                  count: result.passes,
                })}
              </p>
            ) : (
              <div className="space-y-1.5">
                <p className="flex items-center gap-1.5 text-xs text-amber-600 dark:text-amber-500">
                  <PauseCircle size={13} />
                  {t("{count} scope(s) parked — they need a human to advance.", {
                    count: (result.parkedScopes ?? []).length,
                  })}
                </p>
                <ul className="flex flex-wrap gap-1.5">
                  {(result.parkedScopes ?? []).map((s, i) => (
                    <li key={`${s.locale}-${s.collection}-${i}`}>
                      <button
                        type="button"
                        className="flex items-center gap-1 rounded-full border border-border px-2 py-0.5 text-xs hover:bg-accent"
                        onClick={() =>
                          onOpenReview?.({ collection: s.collection ?? "", locale: s.locale })
                        }
                        data-slot="parked-scope-link"
                        aria-label={t("Review {scope}", {
                          scope: s.collection ? `${s.locale} · ${s.collection}` : s.locale,
                        })}
                        title={t("Open Review filtered to this scope")}
                      >
                        <LocalePill locale={s.locale} />
                        {s.collection && (
                          <span className="text-muted-foreground" translate="no">
                            {s.collection}
                          </span>
                        )}
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            )}
            {(result.materializedFiles ?? 0) > 0 && (
              <p className="mt-1 text-[11px] text-muted-foreground">
                {t("Materialized {count} localized file(s) from the project store.", {
                  count: result.materializedFiles ?? 0,
                })}
              </p>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
