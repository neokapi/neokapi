import { CheckCircle2, Circle, Loader2, PauseCircle, XCircle } from "lucide-react";
import {
  Badge,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  LocalePill,
  Progress,
  SimpleTooltip,
  cn,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type {
  ConvergenceLocaleRow,
  ConvergenceOutcome,
  ConvergencePassView,
  ConvergenceRunHeader,
  ConvergenceRunModel,
} from "./convergence-model";

export interface ConvergenceRunViewProps {
  /**
   * The reduced run model (folded from the caller's event/SSE stream). The
   * reduction is app-side; this view is purely presentational.
   */
  model: ConvergenceRunModel;
  /** Whether the run is still in flight (renders the working affordances). */
  running?: boolean;
  /** Whether the run was cancelled (renders the terminal cancelled row). */
  canceled?: boolean;
  /** True while a live connection (SSE) is opening/reconnecting. */
  connecting?: boolean;
  /**
   * Optional run metadata — when present, renders a header with the run's
   * trigger, id, timestamp, and live/terminal badge (bowrain's Runs surface).
   */
  run?: ConvergenceRunHeader;
  /**
   * Optional structured terminal outcome, when the caller holds it out-of-band
   * from the model (kapi-desktop). Renders the converged / parked-scopes block.
   * When omitted, the view falls back to a compact done-footer from the model.
   */
  result?: ConvergenceOutcome;
  /** Open a review surface filtered to a parked (collection, locale) scope. */
  onOpenReview?: (scope: { collection?: string; locale?: string }) => void;
}

/** "shippable | parked | pending" → badge tint. */
const localeStateColor: Record<string, string> = {
  shippable: "bg-green-500/10 text-green-600 dark:text-green-500",
  parked: "bg-amber-500/10 text-amber-600 dark:text-amber-500",
  pending: "bg-muted text-muted-foreground",
};

/**
 * The live convergence view of a "Bring up to date" run. It renders the reduced
 * event stream into passes, each showing one row per locale with a unit-progress
 * bar, the content memory-vs-AI split, and the locale's state (queued → running → done),
 * then the pass's post-derivation summary (produced delta, failing checks, still
 * pending locales). Once the run settles it shows the structured outcome, with
 * each parked (collection, locale) scope rendered as a deep link. Parity with
 * the CLI's live renderer. Shared across kapi-desktop and bowrain.
 */
export function ConvergenceRunView({
  model,
  running,
  canceled,
  connecting,
  run,
  result,
  onOpenReview,
}: ConvergenceRunViewProps) {
  const activePassIndex = model.passes.length - 1;
  const logs = model.logs ?? [];

  return (
    <Card data-slot="converge-run-view">
      {run && (
        <CardHeader className="flex-row items-center justify-between gap-2 px-4">
          <div className="min-w-0">
            <CardTitle className="text-sm font-semibold">{triggerLabel(run.trigger)}</CardTitle>
            <p className="text-[11px] text-muted-foreground">
              {t("Run {id}", { id: run.id.slice(0, 8) })}
              {run.created_at && (
                <> · {t("started {when}", { when: new Date(run.created_at).toLocaleString() })}</>
              )}
            </p>
          </div>
          <RunStatusBadge model={model} running={running} connecting={connecting} />
        </CardHeader>
      )}

      {!run && (
        <CardHeader className="px-4">
          <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {t("Passes")}
          </CardTitle>
        </CardHeader>
      )}

      <CardContent className="space-y-3 px-4">
        {model.passes.length === 0 && (
          <p className="flex items-center gap-2 text-xs text-muted-foreground">
            {(running || connecting) && <Loader2 size={12} className="animate-spin" />}
            {connecting
              ? t("Connecting to the run…")
              : running
                ? t("Deriving state and running the first pass…")
                : t("Waiting for the first pass…")}
          </p>
        )}

        <ol className="space-y-3">
          {model.passes.map((pass, i) => (
            <ConvergePassBlock
              key={pass.pass || i}
              pass={pass}
              live={model.live !== false}
              running={!!running && i === activePassIndex}
            />
          ))}
        </ol>

        {logs.length > 0 && (
          <details className="text-[11px]" data-slot="converge-logs">
            <summary className="cursor-pointer text-muted-foreground">
              {t("Log ({count})", { count: logs.length })}
            </summary>
            <div className="mt-1 max-h-40 space-y-0.5 overflow-y-auto rounded-md bg-muted/50 p-2 font-mono">
              {logs.map((line, i) => (
                <div key={i} className="break-all">
                  {line}
                </div>
              ))}
            </div>
          </details>
        )}

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

        {result ? (
          <ConvergeOutcomeBlock result={result} onOpenReview={onOpenReview} />
        ) : (
          model.done && <ConvergeDoneFooter model={model} />
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Display label for a run's terminal state. The machine value `converged`
 * (from the server/CLI) reads as the product's "up to date"; other states pass
 * through with their own casing.
 */
function finalStateLabel(state?: string): string {
  if (state === "converged") return t("Up to date");
  if (state === "parked") return t("Parked");
  return state ?? t("done");
}

/** The header's live/terminal status badge (bowrain's Runs surface). */
function RunStatusBadge({
  model,
  running,
  connecting,
}: {
  model: ConvergenceRunModel;
  running?: boolean;
  connecting?: boolean;
}) {
  if (model.done) {
    const converged = model.finalState === "converged";
    return (
      <Badge
        className={cn(
          "shrink-0 text-[11px]",
          converged
            ? "bg-green-500/10 text-green-600 dark:text-green-500"
            : "bg-amber-500/10 text-amber-600 dark:text-amber-500",
        )}
      >
        {finalStateLabel(model.finalState)}
      </Badge>
    );
  }
  return (
    <Badge className="shrink-0 animate-pulse text-[11px] bg-blue-500/10 text-blue-600 dark:text-blue-500">
      {connecting ? t("connecting…") : running ? t("running") : t("waiting")}
    </Badge>
  );
}

/** The compact done-footer used when no structured outcome is supplied. */
function ConvergeDoneFooter({ model }: { model: ConvergenceRunModel }) {
  const lastPass = model.passes[model.passes.length - 1];
  const finalFailing =
    lastPass?.failingChecks ?? model.passes.reduce((n, p) => n + (p.failingChecks ?? 0), 0);
  return (
    <div
      className="flex items-center gap-3 border-t border-border pt-2 text-xs text-muted-foreground"
      data-slot="converge-done-footer"
    >
      <span className="font-medium text-foreground">{finalStateLabel(model.finalState)}</span>
      {model.materializedFiles != null && (
        <span>{t("{count} files written", { count: model.materializedFiles })}</span>
      )}
      {finalFailing > 0 && (
        <span className="text-destructive">
          {t("{count} failing checks", { count: finalFailing })}
        </span>
      )}
    </div>
  );
}

/** The structured converged / parked-scopes outcome block (kapi-desktop). */
function ConvergeOutcomeBlock({
  result,
  onOpenReview,
}: {
  result: ConvergenceOutcome;
  onOpenReview?: (scope: { collection?: string; locale?: string }) => void;
}) {
  const parked = result.parkedScopes ?? [];
  return (
    <div className="border-t border-border pt-2" data-slot="converge-result">
      {result.converged ? (
        <p className="flex items-center gap-1.5 text-xs text-green-600 dark:text-green-500">
          <CheckCircle2 size={13} />
          {t("Up to date in {count} pass(es) — every gated scope is shippable.", {
            count: result.passes,
          })}
        </p>
      ) : (
        <div className="space-y-1.5">
          <p className="flex items-center gap-1.5 text-xs text-amber-600 dark:text-amber-500">
            <PauseCircle size={13} />
            {t("{count} scope(s) parked — they need a human to advance.", {
              count: parked.length,
            })}
          </p>
          <ul className="flex flex-wrap gap-1.5">
            {parked.map((s, i) => (
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
      {(result.blockedOnSource ?? 0) > 0 && (
        <div
          className="mt-2 rounded-md border border-amber-500/40 bg-amber-500/5 px-2 py-1.5"
          data-slot="convergence-source-hold"
        >
          <p className="text-[11px] text-amber-700 dark:text-amber-500">
            {t("{count} segments need source review before translating.", {
              count: result.blockedOnSource ?? 0,
            })}
          </p>
          <p className="mt-0.5 text-[11px] text-muted-foreground">
            {result.stallReason === "source_not_ready"
              ? t(
                  "Nothing was produced for any language: the source has not reached the {gate} gate. Settle it in the Review page's source lane.",
                  { gate: result.sourceGate || "source" },
                )
              : t(
                  "Their translations were held in every language. Settle them in the Review page's source lane.",
                )}
          </p>
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
  );
}

interface ConvergePassBlockProps {
  pass: ConvergencePassView;
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

/** One locale's live cell: state icon, progress bar, counts, Memory/AI split. */
function ConvergeLocaleRowView({ row }: { row: ConvergenceLocaleRow }) {
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
      {(row.viaMemory > 0 || row.viaAI > 0) && (
        <span className="shrink-0 text-[11px] text-muted-foreground">
          {t("Memory {mem} · AI {ai}", { mem: row.viaMemory, ai: row.viaAI })}
        </span>
      )}
      {row.localeState && (
        <Badge
          className={cn(
            "shrink-0 text-[10px] capitalize",
            localeStateColor[row.localeState] ?? "bg-muted text-muted-foreground",
          )}
        >
          {row.localeState}
        </Badge>
      )}
    </div>
  );
}

function LocaleStateIcon({ state }: { state: ConvergenceLocaleRow["state"] }) {
  if (state === "done") {
    return <CheckCircle2 size={13} className="shrink-0 text-green-600 dark:text-green-500" />;
  }
  if (state === "running") {
    return <Loader2 size={13} className="shrink-0 animate-spin text-primary" />;
  }
  return <Circle size={13} className="shrink-0 text-muted-foreground/50" />;
}

function triggerLabel(trigger: string): string {
  const labels: Record<string, string> = {
    manual: t("Manual run"),
    cli: t("kapi up (CLI)"),
    push: t("On push"),
  };
  return labels[trigger] || trigger;
}
