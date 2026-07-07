import { CheckCircle2, Circle, Loader2, PauseCircle, XCircle } from "lucide-react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  LocalePill,
  Progress,
  SimpleTooltip,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { RunEvent } from "../context/JobFeedContext";
import type { ConvergeOutput } from "../types/api";
import {
  reduceConvergeRun,
  type ConvergeLocaleRow,
  type ConvergePassView,
} from "../lib/convergeRun";

export interface ConvergeRunViewProps {
  /** The run's event stream (from the job feed). */
  events: RunEvent[];
  /** Whether the run is still in flight (renders the working row). */
  running?: boolean;
  /** Whether the run was cancelled (renders the terminal cancelled row). */
  canceled?: boolean;
  /** Open the Review page filtered to a parked (collection, locale) scope. */
  onOpenReview?: (scope: { collection?: string; locale?: string }) => void;
}

/**
 * The live convergence view of a "Bring up to date" run. It reduces the run's
 * typed convergence-event stream into passes, each showing one row per locale
 * with a unit-progress bar, the TM-vs-AI split, and the locale's state
 * (queued → running → done), then the pass's post-derivation summary (produced
 * delta, failing checks, still-pending locales). Once the run settles it shows
 * the structured outcome with each parked (collection, locale) scope rendered
 * as a deep link into the Review page. Parity with the CLI's live renderer.
 */
export function ConvergeRunView({ events, running, canceled, onOpenReview }: ConvergeRunViewProps) {
  const model = reduceConvergeRun(events);
  const result: ConvergeOutput | undefined = events.find(
    (e) => e.type === "complete" && e.converge_result,
  )?.converge_result;

  const activePassIndex = model.passes.length - 1;

  return (
    <Card data-slot="converge-run-view">
      <CardHeader className="px-4">
        <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          {t("Passes")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 px-4">
        {model.passes.length === 0 && running && (
          <p className="flex items-center gap-2 text-xs text-muted-foreground">
            <Loader2 size={12} className="animate-spin" />
            {t("Deriving state and running the first pass…")}
          </p>
        )}

        <ol className="space-y-3">
          {model.passes.map((pass, i) => (
            <ConvergePassBlock
              key={pass.pass || i}
              pass={pass}
              live={model.live}
              running={!!running && i === activePassIndex}
            />
          ))}
        </ol>

        {canceled && (
          <p
            className="flex items-center gap-1.5 border-t border-border pt-2 text-xs text-muted-foreground"
            data-slot="converge-canceled"
          >
            <XCircle size={13} />
            {t(
              "Cancelled — the run stopped before reaching the gates. Nothing is lost: run Bring up to date again to continue.",
            )}
          </p>
        )}

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
                      <SimpleTooltip content={t("Open Review filtered to this scope")}>
                        <button
                          type="button"
                          className="flex items-center gap-1 rounded-full border border-border px-2 py-0.5 text-xs hover:bg-accent"
                          onClick={() =>
                            onOpenReview?.({
                              collection: s.collection ?? "",
                              locale: s.locale,
                            })
                          }
                          data-slot="parked-scope-link"
                          aria-label={t("Review {scope}", {
                            scope: s.collection ? `${s.locale} · ${s.collection}` : s.locale,
                          })}
                        >
                          <LocalePill locale={s.locale} />
                          {s.collection && (
                            <span className="text-muted-foreground" translate="no">
                              {s.collection}
                            </span>
                          )}
                        </button>
                      </SimpleTooltip>
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

interface ConvergePassBlockProps {
  pass: ConvergePassView;
  /** Whether the model has live per-locale rows (vs the compat summary path). */
  live: boolean;
  /** This is the currently-running pass. */
  running: boolean;
}

/** One pass: its header, live locale rows, then the post-derivation summary. */
function ConvergePassBlock({ pass, live, running }: ConvergePassBlockProps) {
  const pending = pass.pending ?? [];
  return (
    <li className="space-y-1.5" data-slot="converge-pass-row" data-pass={pass.pass}>
      <div className="flex items-center gap-2 text-xs">
        <span className="font-medium">{t("Pass {n}", { n: pass.pass })}</span>
        {pass.maxPasses > 0 && (
          <span className="text-muted-foreground">{t("of {n}", { n: pass.maxPasses })}</span>
        )}
        {running && !pass.settled && <Loader2 size={11} className="animate-spin text-primary" />}
      </div>

      {pass.rows.length > 0 && (
        <div className="space-y-1" data-slot="converge-locale-rows">
          {pass.rows.map((row) => (
            <ConvergeLocaleRowView key={row.locale} row={row} />
          ))}
        </div>
      )}

      {pass.settled && (
        <div
          className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs"
          data-slot="converge-pass-summary"
        >
          {/* The compat path has no rows — echo the extracted/produced summary. */}
          <span className="text-muted-foreground">
            {t("produced {count}", { count: pass.produced ?? 0 })}
            {(pass.producedDelta ?? 0) > 0 ? ` (+${pass.producedDelta})` : ""}
          </span>
          <span className="text-muted-foreground">·</span>
          <span
            className={
              (pass.failingChecks ?? 0) > 0
                ? "text-amber-600 dark:text-amber-500"
                : "text-muted-foreground"
            }
          >
            {t("checks failing {count}", { count: pass.failingChecks ?? 0 })}
          </span>
          {pending.length > 0 && (
            <>
              <span className="text-muted-foreground">·</span>
              <span className="text-muted-foreground">{t("still pending")}</span>
              {pending.map((loc) => (
                <LocalePill key={loc} locale={loc} />
              ))}
            </>
          )}
        </div>
      )}

      {/* Compat path (no typed events): keep a bare "running" affordance. */}
      {!live && running && !pass.settled && (
        <p className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 size={12} className="animate-spin" />
          {t("Running…")}
        </p>
      )}
    </li>
  );
}

/** One locale's live cell: state icon, progress bar, counts, TM/AI split. */
function ConvergeLocaleRowView({ row }: { row: ConvergeLocaleRow }) {
  const pct = row.units > 0 ? Math.round((row.done / row.units) * 100) : 0;
  return (
    <div
      className="flex items-center gap-2 text-xs"
      data-slot="converge-locale-row"
      data-locale={row.locale}
      data-state={row.state}
    >
      <LocaleStateIcon state={row.state} />
      <div className="w-16 shrink-0">
        <LocalePill locale={row.locale} />
      </div>
      <Progress
        value={pct}
        className="h-1.5 flex-1"
        aria-label={t("{locale}: {done} of {units} units", {
          locale: row.locale,
          done: row.done,
          units: row.units,
        })}
      />
      <span className="shrink-0 tabular-nums text-muted-foreground">
        {row.done}/{row.units}
      </span>
      {(row.viaTM > 0 || row.viaAI > 0) && (
        <span className="shrink-0 text-[11px] text-muted-foreground">
          {t("TM {tm} · AI {ai}", { tm: row.viaTM, ai: row.viaAI })}
        </span>
      )}
    </div>
  );
}

function LocaleStateIcon({ state }: { state: ConvergeLocaleRow["state"] }) {
  if (state === "done") {
    return <CheckCircle2 size={13} className="shrink-0 text-green-600 dark:text-green-500" />;
  }
  if (state === "running") {
    return <Loader2 size={13} className="shrink-0 animate-spin text-primary" />;
  }
  return <Circle size={13} className="shrink-0 text-muted-foreground/50" />;
}
