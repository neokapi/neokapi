import type { ReactNode } from "react";
import { Alert, AlertDescription, AlertTitle, Button, cn } from "@neokapi/ui-primitives";
import type { ConvergenceRun } from "../types/api";

/**
 * ConvergenceRunContext renders a run's live loop position and a labeled
 * stall/hold banner with a next action (roadmap epic 019, theme B4). It reads
 * the run-context fields the server now carries — current_stage, current_locale,
 * stall_reason, blocked_on_source, last_activity — so a stalled run shows WHY it
 * stopped and what to do, never a silent spinner:
 *
 *  - source_not_ready → "Source not ready — settle N blocks" + Review source
 *  - needs_credits    → "Out of credits — N AI units remaining" + Buy credits
 *  - parked (checks)  → "Parked — open review"
 *  - running w/ frozen last_activity → "waiting…"
 *
 * Presentational: the caller wires the next-action navigation via callbacks.
 */
export interface ConvergenceRunContextProps {
  run: ConvergenceRun;
  /** True while the run's SSE stream is live (drives the "waiting…" idle hint). */
  live?: boolean;
  /** Open the source-review worklist/tasks (source_not_ready next action). */
  onSettleSource?: () => void;
  /** Open the buy-credits / upgrade path (needs_credits next action). */
  onBuyCredits?: () => void;
  /** Open the review queue (parked-on-checks next action). */
  onOpenReview?: () => void;
}

/** Human labels for the loop stage the run reports. */
const stageLabel: Record<string, string> = {
  "settle-source": "settle source",
  settle_source: "settle source",
  sync: "sync",
  derive: "derive",
  recycle: "recycle",
  translate: "translate",
  ai_translate: "translate",
  checks: "checks",
  materialize: "materialize",
};

/** Whether the run's last observable progress is stale (> 45s), i.e. "waiting". */
function isIdle(run: ConvergenceRun): boolean {
  if (!run.last_activity) return false;
  const last = new Date(run.last_activity).getTime();
  if (Number.isNaN(last)) return false;
  return Date.now() - last > 45_000;
}

/** The one-line loop position: "Pass 2 · settle source · nb". */
function positionLine(run: ConvergenceRun, live: boolean): string {
  const parts: string[] = [];
  if (run.passes > 0) parts.push(`Pass ${run.passes}`);
  if (run.current_stage) parts.push(stageLabel[run.current_stage] ?? run.current_stage);
  if (run.current_locale) parts.push(run.current_locale);
  let line = parts.join(" · ");
  if (run.state === "running" && live && isIdle(run)) {
    line = line ? `${line} · waiting…` : "waiting…";
  }
  return line;
}

/** TM/AI totals across the run's locale standing (truthful "TM N · AI M"). */
function tmAiSummary(run: ConvergenceRun): string | null {
  const locales = run.locales ?? [];
  let tm = 0;
  let ai = 0;
  for (const l of locales) {
    tm += l.viaTM ?? 0;
    ai += l.viaAI ?? 0;
  }
  if (tm === 0 && ai === 0) return null;
  return `TM ${tm} · AI ${ai}`;
}

export function ConvergenceRunContext({
  run,
  live,
  onSettleSource,
  onBuyCredits,
  onOpenReview,
}: ConvergenceRunContextProps) {
  const position = positionLine(run, !!live);
  const tmAi = tmAiSummary(run);

  // The labeled stall/hold banner: reason + next action. Only terminal-ish
  // states carry a stall_reason; a running run without one just shows position.
  const banner = stallBanner(run, { onSettleSource, onBuyCredits, onOpenReview });

  if (!position && !tmAi && !banner) return null;

  return (
    <div className="space-y-3">
      {(position || tmAi) && (
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          {position && <span className="font-medium text-foreground">{position}</span>}
          {tmAi && <span className="tabular-nums">{tmAi}</span>}
        </div>
      )}
      {banner}
    </div>
  );
}

/** Build the labeled stall/hold banner for a run, or null when none applies. */
function stallBanner(
  run: ConvergenceRun,
  actions: {
    onSettleSource?: () => void;
    onBuyCredits?: () => void;
    onOpenReview?: () => void;
  },
): ReactNode {
  const reason = run.stall_reason;

  if (reason === "source_not_ready" || (run.blocked_on_source ?? 0) > 0) {
    const n = run.blocked_on_source ?? 0;
    return (
      <BannerAlert
        variant="warning"
        title="Source not ready"
        body={
          n > 0
            ? `${n} block${n === 1 ? "" : "s"} need settling or source review before translating. Settle your source first (terminology, brand, source QA).`
            : "Some blocks need settling or source review before translating."
        }
        action={
          actions.onSettleSource && { label: "Review source", onClick: actions.onSettleSource }
        }
      />
    );
  }

  if (reason === "needs_credits") {
    const remaining = aiRemaining(run);
    return (
      <BannerAlert
        variant="destructive"
        title="Out of credits"
        body={
          remaining > 0
            ? `${remaining} AI unit${remaining === 1 ? "" : "s"} remaining. Add credits to finish translating; work so far is saved.`
            : "This workspace is out of credits. Add credits to finish translating; work so far is saved."
        }
        action={actions.onBuyCredits && { label: "Buy credits", onClick: actions.onBuyCredits }}
      />
    );
  }

  if (reason === "needs_ai_key") {
    return (
      <BannerAlert
        variant="destructive"
        title="AI provider unavailable"
        body="The run could not reach an AI provider. This is a workspace configuration issue."
      />
    );
  }

  // A parked run with no more specific reason: pending human review.
  if (run.state === "parked") {
    return (
      <BannerAlert
        variant="warning"
        title="Parked — pending review"
        body="The run advanced everything it could and parked the remainder for review."
        action={actions.onOpenReview && { label: "Open review", onClick: actions.onOpenReview }}
      />
    );
  }

  if (run.state === "failed" && run.error) {
    return <BannerAlert variant="destructive" title="Run failed" body={run.error} />;
  }

  return null;
}

/** AI units still pending across the run's locale standing. */
function aiRemaining(run: ConvergenceRun): number {
  const locales = run.locales ?? [];
  let remaining = 0;
  for (const l of locales) {
    const units = l.units ?? 0;
    const produced = l.produced ?? 0;
    remaining += Math.max(units - produced, 0);
  }
  return remaining;
}

type BannerVariant = "warning" | "destructive";

const bannerClass: Record<BannerVariant, string> = {
  warning: "border-warning/40 bg-warning/5 text-warning-foreground",
  destructive: "border-destructive/40 bg-destructive/5",
};

function BannerAlert({
  variant,
  title,
  body,
  action,
}: {
  variant: BannerVariant;
  title: string;
  body: string;
  action?: { label: string; onClick: () => void };
}) {
  return (
    <Alert className={cn(bannerClass[variant])}>
      <div className="flex items-start justify-between gap-4">
        <div>
          <AlertTitle>{title}</AlertTitle>
          <AlertDescription>{body}</AlertDescription>
        </div>
        {action && (
          <Button
            size="sm"
            variant={variant === "destructive" ? "default" : "outline"}
            className="shrink-0"
            onClick={action.onClick}
          >
            {action.label}
          </Button>
        )}
      </div>
    </Alert>
  );
}
