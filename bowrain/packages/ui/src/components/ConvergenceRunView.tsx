import { Badge, Card, Progress, cn } from "@neokapi/ui-primitives";
import type { ConvergenceRun } from "../types/api";
import type {
  ConvergenceRunModel,
  ConvergencePassView,
  ConvergenceLocaleRow,
  LocaleRowState,
} from "../convergence/runReducer";

// ---------------------------------------------------------------------------
// Status helpers
// ---------------------------------------------------------------------------

const rowStateColor: Record<LocaleRowState, string> = {
  queued: "bg-muted text-muted-foreground",
  running: "bg-info/10 text-info dark:bg-info/20 dark:text-info",
  done: "bg-success/10 text-success dark:bg-success/20 dark:text-success",
};

/** shippable | parked | pending → badge tint. */
const localeStateColor: Record<string, string> = {
  shippable: "bg-success/10 text-success dark:bg-success/20 dark:text-success",
  parked: "bg-warning/10 text-warning dark:bg-warning/20 dark:text-warning",
  pending: "bg-muted text-muted-foreground",
};

/** converged | parked → the run's final badge tint. */
const finalStateColor: Record<string, string> = {
  converged: "bg-success/10 text-success dark:bg-success/20 dark:text-success",
  parked: "bg-warning/10 text-warning dark:bg-warning/20 dark:text-warning",
};

function pct(done: number, units: number): number {
  if (units <= 0) return 0;
  return Math.min(100, Math.round((done / units) * 100));
}

// ---------------------------------------------------------------------------
// Locale row
// ---------------------------------------------------------------------------

function LocaleRow({ row }: { row: ConvergenceLocaleRow }) {
  const label = row.localeState ?? row.state;
  const badgeClass = row.localeState
    ? (localeStateColor[row.localeState] ?? "bg-muted text-muted-foreground")
    : rowStateColor[row.state];
  return (
    <div className="grid grid-cols-[110px_1fr_auto] items-center gap-3 py-1.5">
      <span className="font-mono text-xs truncate" title={row.locale}>
        {row.locale}
      </span>
      <div className="flex items-center gap-2 min-w-0">
        <Progress
          value={pct(row.done, row.units)}
          className={cn("h-1.5", row.state === "running" && "animate-pulse")}
        />
        <span className="text-[11px] text-muted-foreground tabular-nums shrink-0 w-16 text-right">
          {row.done}/{row.units || "?"}
        </span>
      </div>
      <div className="flex items-center gap-2 justify-end">
        <span className="text-[11px] text-muted-foreground tabular-nums shrink-0">
          TM {row.viaTM} · AI {row.viaAI}
        </span>
        <Badge className={cn("text-[10px] capitalize", badgeClass)}>{label}</Badge>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Pass block
// ---------------------------------------------------------------------------

function PassBlock({ pass }: { pass: ConvergencePassView }) {
  return (
    <Card className="p-3 gap-2">
      <div className="flex items-center justify-between">
        <h4 className="text-xs font-semibold">
          Pass {pass.pass}
          {pass.maxPasses > 0 && (
            <span className="text-muted-foreground font-normal"> / {pass.maxPasses}</span>
          )}
        </h4>
        {pass.settled ? (
          <div className="flex items-center gap-2 text-[11px] text-muted-foreground tabular-nums">
            {pass.produced != null && (
              <span>
                {pass.produced} produced
                {pass.producedDelta != null && pass.producedDelta > 0 && (
                  <span className="text-success"> (+{pass.producedDelta})</span>
                )}
              </span>
            )}
            {pass.failingChecks != null && pass.failingChecks > 0 && (
              <Badge className="text-[10px] bg-destructive/10 text-destructive dark:bg-destructive/20 dark:text-destructive">
                {pass.failingChecks} failing
              </Badge>
            )}
          </div>
        ) : (
          <Badge className="text-[10px] bg-info/10 text-info dark:bg-info/20 dark:text-info animate-pulse">
            running
          </Badge>
        )}
      </div>
      <div className="divide-y divide-border/60">
        {pass.rows.length === 0 ? (
          <p className="text-[11px] text-muted-foreground py-1">No locales in this pass.</p>
        ) : (
          pass.rows.map((row) => <LocaleRow key={row.locale} row={row} />)
        )}
      </div>
      {pass.settled && pass.pending && pass.pending.length > 0 && (
        <p className="text-[11px] text-muted-foreground">
          Still short of gate: {pass.pending.join(", ")}
        </p>
      )}
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Run view
// ---------------------------------------------------------------------------

export interface ConvergenceRunViewProps {
  /** The reduced event model (folded from the run's SSE stream). */
  model: ConvergenceRunModel;
  /** Run metadata for the header (trigger, id, timestamps). Optional for stories. */
  run?: ConvergenceRun;
  /** True while the SSE connection is opening/reconnecting. */
  connecting?: boolean;
}

/**
 * ConvergenceRunView renders a single server-side convergence run's live
 * detail: per-locale progress rows grouped by pass, each pass's post-derivation
 * summary, run log lines, and the terminal state (converged / parked with
 * failing-check counts). It is presentational — the caller folds the SSE stream
 * into `model` (see convergence/runReducer) and passes it in.
 */
export function ConvergenceRunView({ model, run, connecting }: ConvergenceRunViewProps) {
  const totalFailing = model.passes.reduce((n, p) => n + (p.failingChecks ?? 0), 0);
  const lastPass = model.passes[model.passes.length - 1];
  const finalFailing = lastPass?.failingChecks ?? totalFailing;

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold">
            {run ? triggerLabel(run.trigger) : "Convergence run"}
          </h3>
          {run && (
            <p className="text-[11px] text-muted-foreground">
              Run {run.id.slice(0, 8)}
              {run.created_at && <> · started {new Date(run.created_at).toLocaleString()}</>}
            </p>
          )}
        </div>
        {model.done ? (
          <Badge
            className={cn(
              "text-[11px] capitalize",
              finalStateColor[model.finalState ?? ""] ?? "bg-muted text-muted-foreground",
            )}
          >
            {model.finalState ?? "done"}
          </Badge>
        ) : (
          <Badge className="text-[11px] bg-info/10 text-info dark:bg-info/20 dark:text-info animate-pulse">
            {connecting ? "connecting…" : "running"}
          </Badge>
        )}
      </div>

      {model.passes.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {connecting ? "Connecting to the run…" : "Waiting for the first pass…"}
        </p>
      )}

      <div className="space-y-2">
        {model.passes.map((pass) => (
          <PassBlock key={pass.pass} pass={pass} />
        ))}
      </div>

      {model.logs.length > 0 && (
        <details className="text-[11px]">
          <summary className="cursor-pointer text-muted-foreground">
            Log ({model.logs.length})
          </summary>
          <div className="mt-1 bg-muted/50 rounded-md p-2 font-mono space-y-0.5 max-h-40 overflow-y-auto">
            {model.logs.map((line, i) => (
              <div key={i} className="break-all">
                {line}
              </div>
            ))}
          </div>
        </details>
      )}

      {model.done && (
        <div className="flex items-center gap-3 text-xs text-muted-foreground border-t border-border pt-2">
          <span className="capitalize font-medium text-foreground">
            {model.finalState ?? "done"}
          </span>
          {model.materializedFiles != null && <span>{model.materializedFiles} files written</span>}
          {finalFailing > 0 && (
            <span className="text-destructive">{finalFailing} failing checks</span>
          )}
        </div>
      )}
    </div>
  );
}

function triggerLabel(trigger: string): string {
  const labels: Record<string, string> = {
    manual: "Manual run",
    cli: "kapi up (CLI)",
    push: "On push",
  };
  return labels[trigger] || trigger;
}
