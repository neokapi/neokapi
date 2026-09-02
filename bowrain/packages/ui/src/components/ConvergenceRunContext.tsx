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
 *  - no_target_locales → "No target languages configured" (add one in settings)
 *  - parked (checks)  → "Parked — open review"
 *  - running w/ frozen last_activity → "waiting…"
 *
 * A completed run with newly produced translations also renders the awaiting-
 * review element ("N blocks awaiting review" + per-locale breakdown, from the
 * run's locale standing): governed review is on by default, so the closing
 * frame of a run is the review hand-off, not a silent dashboard update.
 *
 * Presentational: the caller wires the next-action navigation via callbacks.
 */
export interface ConvergenceRunContextProps {
  run: ConvergenceRun;
  /** True while the run's SSE stream is live (drives the "waiting…" idle hint). */
  live?: boolean;
  /**
   * Whether the project's review workflow is enabled (default true — governed
   * review is opt-out). When false, the awaiting-review element is suppressed.
   */
  reviewEnabled?: boolean;
  /** Open the source-review worklist/tasks (source_not_ready next action). */
  onSettleSource?: () => void;
  /** Open the buy-credits / upgrade path (needs_credits next action). */
  onBuyCredits?: () => void;
  /** Open the review queue (parked-on-checks next action). */
  onOpenReview?: () => void;
  /**
   * Open the review surface for this project/stream (awaiting-review next
   * action after a completed run). Falls back to onOpenReview when unset.
   */
  onOpenReviewSurface?: () => void;
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

/** Memory/AI totals across the run's locale standing (truthful "Memory N · AI M"). */
function memoryAiSummary(run: ConvergenceRun): string | null {
  const locales = run.locales ?? [];
  let tm = 0;
  let ai = 0;
  for (const l of locales) {
    tm += l.viaTM ?? 0;
    ai += l.viaAI ?? 0;
  }
  if (tm === 0 && ai === 0) return null;
  return `Memory ${tm} · AI ${ai}`;
}

export function ConvergenceRunContext({
  run,
  live,
  reviewEnabled,
  onSettleSource,
  onBuyCredits,
  onOpenReview,
  onOpenReviewSurface,
}: ConvergenceRunContextProps) {
  const position = positionLine(run, !!live);
  const memoryAi = memoryAiSummary(run);

  // The labeled stall/hold banner: reason + next action. Only terminal-ish
  // states carry a stall_reason; a running run without one just shows position.
  const banner = stallBanner(run, { onSettleSource, onBuyCredits, onOpenReview });

  // The review hand-off: a converged run whose new translations await human
  // sign-off. Parked runs carry the count inside the parked banner instead.
  const review =
    reviewEnabled !== false && run.state === "converged"
      ? awaitingReviewBanner(run, onOpenReviewSurface ?? onOpenReview)
      : null;

  if (!position && !memoryAi && !banner && !review) return null;

  return (
    <div className="space-y-3">
      {(position || memoryAi) && (
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          {position && <span className="font-medium text-foreground">{position}</span>}
          {memoryAi && <span className="tabular-nums">{memoryAi}</span>}
        </div>
      )}
      {banner}
      {review}
    </div>
  );
}

/** Blocks this run produced (Memory + AI) per locale — the translations that now
 * sit in the review queue under the default-on governed-review workflow. */
function awaitingReview(run: ConvergenceRun): { total: number; breakdown: string } {
  const locales = run.locales ?? [];
  let total = 0;
  const parts: string[] = [];
  for (const l of locales) {
    const produced = l.produced ?? 0;
    if (produced > 0) {
      total += produced;
      parts.push(`${l.locale} ${produced}`);
    }
  }
  return { total, breakdown: parts.join(" · ") };
}

/** The completion element for a converged run: "N blocks awaiting review" with
 * the per-locale breakdown and a deep link into the review surface. Null when
 * the run produced nothing (nothing new to review). */
function awaitingReviewBanner(run: ConvergenceRun, onOpenReview?: () => void): ReactNode {
  const { total, breakdown } = awaitingReview(run);
  if (total === 0) return null;
  return (
    <BannerAlert
      variant="info"
      title={`${total} block${total === 1 ? "" : "s"} awaiting review`}
      body={
        breakdown
          ? `The run converged and queued the new translations for review (${breakdown}).`
          : "The run converged and queued the new translations for review."
      }
      action={onOpenReview && { label: "Open review", onClick: onOpenReview }}
    />
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
            ? `${n} block${n === 1 ? "" : "s"} need settling or source review before translating. Settle your source first (terminology, brand, source checks).`
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

  if (reason === "no_target_locales") {
    return (
      <BannerAlert
        variant="warning"
        title="No target languages configured"
        body="This project has no target languages, so there is nothing to converge toward. Add a target language in the project settings; the next run will translate into it."
      />
    );
  }

  // A parked run with no more specific reason: pending human review. The body
  // carries the produced count so the parked frame also says how much new work
  // sits in the review queue.
  if (run.state === "parked") {
    const { total, breakdown } = awaitingReview(run);
    let body = "The run advanced everything it could and parked the remainder for review.";
    if (total > 0) {
      body += ` ${total} block${total === 1 ? "" : "s"} awaiting review${breakdown ? ` (${breakdown})` : ""}.`;
    }
    return (
      <BannerAlert
        variant="warning"
        title="Parked, pending review"
        body={body}
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

type BannerVariant = "info" | "warning" | "destructive";

const bannerClass: Record<BannerVariant, string> = {
  info: "border-primary/30 bg-primary/5",
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
