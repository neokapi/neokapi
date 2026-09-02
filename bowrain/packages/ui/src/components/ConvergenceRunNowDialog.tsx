import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Skeleton,
  cn,
} from "@neokapi/ui-primitives";
import type { ConvergenceEstimate, ConvergenceRunScope } from "../types/api";

/**
 * ConvergenceRunNowDialog is the run-now consent gate (roadmap epic 019, theme
 * B4). It sequences SOURCE READINESS FIRST — "M of K blocks ready; N held —
 * settle first" — then, for the ready portion, the credit/$ estimate and "your
 * balance covers ~N", with scope choices:
 *
 *   [Translate all now]  — the full estimate (if it exceeds balance, an inline
 *                          buy-credits/upgrade path)
 *   [Translate ready source only]
 *   [Transport only]     — push, don't translate (scope "none")
 *
 * There is NO "use your own key" option (owner steer 2026-07-17: consumers rely
 * on platform models). Presentational: the caller supplies the estimate and
 * wires start/settle/buy-credits.
 */
export interface ConvergenceRunNowDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The pre-flight estimate; undefined while loading. */
  estimate?: ConvergenceEstimate;
  loading?: boolean;
  error?: string;
  /** True while a start request is in flight. */
  starting?: boolean;
  /** Start a run with the chosen scope (or transport-only for "none"). */
  onStart: (scope: ConvergenceRunScope) => void;
  /** Open the source-review worklist ("settle first" link). */
  onSettleSource?: () => void;
  /** Open the buy-credits / upgrade path (inline when the estimate exceeds balance). */
  onBuyCredits?: () => void;
}

function creditLine(estimate: ConvergenceEstimate): string | null {
  const c = estimate.credits;
  if (!c) return null;
  const usd = c.estimated_usd >= 0.005 ? ` (~$${c.estimated_usd.toFixed(2)})` : "";
  if (c.covers_all_ai) {
    return `~${c.estimated_credits.toLocaleString()} credits${usd} for the AI work. Your balance of ${c.balance.toLocaleString()} covers it.`;
  }
  return `~${c.estimated_credits.toLocaleString()} credits${usd} for the AI work. Your balance of ${c.balance.toLocaleString()} covers ~${c.covers_ai_units.toLocaleString()} of ${estimate.totals.via_ai.toLocaleString()} AI units.`;
}

export function ConvergenceRunNowDialog({
  open,
  onOpenChange,
  estimate,
  loading,
  error,
  starting,
  onStart,
  onSettleSource,
  onBuyCredits,
}: ConvergenceRunNowDialogProps) {
  const src = estimate?.source;
  const totals = estimate?.totals;
  const held = src?.held ?? 0;
  const ready = src?.ready ?? 0;
  const total = src?.total ?? 0;
  const pending = totals?.pending ?? 0;
  const viaAI = totals?.via_ai ?? 0;
  const exceedsBalance = !!estimate?.credits && !estimate.credits.covers_all_ai && viaAI > 0;
  const nothingToTranslate = !loading && !error && pending === 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-[540px]"
        onInteractOutside={(e: Event) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>Run now</DialogTitle>
          <DialogDescription>
            Bowrain settles your source first, then translates the approved source.
          </DialogDescription>
        </DialogHeader>

        {loading ? (
          <div className="space-y-2 py-2">
            <Skeleton className="h-5 w-3/4" />
            <Skeleton className="h-5 w-2/3" />
          </div>
        ) : error ? (
          <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
            Could not load the estimate: {error}
          </div>
        ) : (
          <div className="space-y-4 py-1">
            {/* 1. Source readiness FIRST. */}
            <section className="space-y-1">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                Source
              </h3>
              {held > 0 ? (
                <div className="text-sm">
                  <p>
                    <span className="font-medium">{ready}</span> of {total} blocks ready;{" "}
                    <span className="font-medium text-warning">{held} held</span>. Settle these
                    first.
                  </p>
                  {onSettleSource && (
                    <Button variant="link" className="h-auto p-0 text-sm" onClick={onSettleSource}>
                      Review source →
                    </Button>
                  )}
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">
                  {total > 0 ? `All ${total} blocks ready to translate.` : "No source to gate."}
                </p>
              )}
            </section>

            {/* 2. Translation estimate for the READY source. */}
            {nothingToTranslate ? (
              <p className="text-sm text-muted-foreground">
                Nothing pending over the ready source. Everything is already translated.
              </p>
            ) : (
              <section className="space-y-1">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                  Translate (ready source)
                </h3>
                <p className="text-sm tabular-nums">
                  {pending.toLocaleString()} pending · Memory{" "}
                  {(totals?.via_tm ?? 0).toLocaleString()} (free) · AI {viaAI.toLocaleString()}
                </p>
                {estimate && creditLine(estimate) && (
                  <p
                    className={cn(
                      "text-sm",
                      exceedsBalance ? "text-warning" : "text-muted-foreground",
                    )}
                  >
                    {creditLine(estimate)}
                  </p>
                )}
                {exceedsBalance && onBuyCredits && (
                  <Button variant="link" className="h-auto p-0 text-sm" onClick={onBuyCredits}>
                    Buy credits / upgrade →
                  </Button>
                )}
              </section>
            )}
          </div>
        )}

        <DialogFooter className="flex-col gap-2 sm:flex-col sm:items-stretch sm:space-x-0">
          {!nothingToTranslate && (
            <Button disabled={loading || !!error || starting} onClick={() => onStart("all")}>
              {starting ? "Starting…" : "Translate all now"}
            </Button>
          )}
          {!nothingToTranslate && (
            <Button
              variant="outline"
              disabled={loading || !!error || starting || pending === 0}
              onClick={() => onStart("ready-only")}
            >
              Translate ready source only
            </Button>
          )}
          <Button variant="ghost" disabled={starting} onClick={() => onStart("none")}>
            Transport only
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
