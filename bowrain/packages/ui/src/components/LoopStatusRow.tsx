import { Badge, Card, CardContent, cn } from "@neokapi/ui-primitives";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import type { ReactNode } from "react";
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  CircleCheck,
  Eye,
  Palette,
  RefreshCw,
  Rocket,
} from "./icons";

// ---------------------------------------------------------------------------
// Data shapes
//
// The row is purely presentational: the route (web) or host shell computes
// these summaries from data it already holds — the workspace activity feed,
// the caller's open task list, and the voice compliance rollup — and feeds
// them in as props. Absent fields degrade card by card, never the whole row.
// ---------------------------------------------------------------------------

/** Most recent loop-relevant entry from the workspace activity feed. */
export interface LoopActivitySummary {
  /** Human-readable summary from the feed (server-composed). */
  summary: string;
  /** ISO timestamp of the activity. */
  created_at: string;
}

/** Workspace voice compliance standing, folded from the voice rollup. */
export interface LoopVoiceHealth {
  /** Rounded mean of project brand scores; null while nothing is scored. */
  averageScore: number | null;
  /** Projects with at least one stored score. */
  scoredProjects: number;
  /** Projects flagged as drifted or trending down. */
  driftingProjects: number;
}

/**
 * The workspace's most recent convergence run, folded from the loop rollup
 * (GET /:ws/loop-rollup).
 */
export interface LoopRunStatus {
  /** running | converged | parked | failed | canceled */
  state: string;
  /** Why a parked/failed run did not converge (needs_credits | …). */
  stallReason?: string;
  /** The run's project, for attribution under the state. */
  projectName?: string;
  /** ISO timestamp of the run's last observable progress. */
  updatedAt?: string;
}

/**
 * Workspace ship-state rollup of project-locales, folded from the loop
 * rollup. Derived from cached per-project dashboard rollups: when
 * countedProjects < totalProjects the card says so — the counts never claim
 * the whole workspace.
 */
export interface LoopShipStatus {
  governed: number;
  aiShippable: number;
  pending: number;
  countedProjects: number;
  totalProjects: number;
}

/** The loop-status layer's data, one optional slot per card. */
export interface LoopStatusData {
  /** Latest loop activity (runs, pushes, checks, reviews); absent = none yet. */
  latestActivity?: LoopActivitySummary;
  /** Most recent convergence run; absent hides the run card entirely. */
  latestRun?: LoopRunStatus;
  /** Open review-type tasks for the current user; absent while loading. */
  openReviewTasks?: number;
  /** Ship-state rollup; absent hides the ship card entirely. */
  ship?: LoopShipStatus;
  /** Voice rollup summary; absent hides the voice card entirely. */
  brand?: LoopVoiceHealth;
}

export interface LoopStatusRowProps {
  status: LoopStatusData;
  /** Opens the workspace activity feed. */
  onOpenActivities?: () => void;
  /** Opens the latest run's runs page. */
  onOpenRuns?: () => void;
  /** Opens the workspace task queue. */
  onOpenTasks?: () => void;
  /** Opens the workspace review inbox (the "Awaiting review" card's target). */
  onOpenReview?: () => void;
  /** Opens the delivery (translation dashboard) surface. */
  onOpenDelivery?: () => void;
  /** Opens the voice dashboard. */
  onOpenVoiceDashboard?: () => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Compute relative time string from an ISO timestamp. */
function relativeTime(iso: string): string {
  const seconds = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

/** Human labels for the run lifecycle states. */
const RUN_STATE_LABELS: Record<string, string> = {
  running: "Running",
  converged: "Converged",
  parked: "Parked",
  failed: "Failed",
  canceled: "Canceled",
};

/** Human labels for the machine-readable stall reasons (epic 019 theme C). */
const STALL_REASON_LABELS: Record<string, string> = {
  needs_credits: "needs credits",
  needs_ai_key: "needs an AI key",
  rate_limited: "rate limited",
  no_progress: "no progress",
  checks_failing: "checks failing",
  source_not_ready: "source not ready",
};

// ---------------------------------------------------------------------------
// Card scaffold
// ---------------------------------------------------------------------------

function StatusCard({
  label,
  icon,
  footer,
  onOpen,
  testId,
  children,
}: {
  label: ReactNode;
  icon: ReactNode;
  footer: ReactNode;
  onOpen?: () => void;
  testId: string;
  children: ReactNode;
}) {
  return (
    <Card
      data-testid={testId}
      onClick={onOpen}
      className={cn("group flex flex-col", onOpen && "cursor-pointer transition-all")}
    >
      <CardContent className="flex flex-1 flex-col gap-3 px-5 pt-1 pb-2">
        <div className="flex items-center gap-2.5">
          <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-primary/10 [&_svg]:h-3.5 [&_svg]:w-3.5 [&_svg]:text-primary">
            {icon}
          </span>
          <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {label}
          </span>
        </div>
        <div className="flex-1">{children}</div>
        {onOpen && (
          <span className="flex items-center gap-1 text-xs font-medium text-primary transition-all group-hover:gap-2">
            {footer}
            <ArrowRight className="h-3 w-3" />
          </span>
        )}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Row
// ---------------------------------------------------------------------------

/** Grid column classes per visible-card count (2–5). */
const GRID_COLS: Record<number, string> = {
  2: "lg:grid-cols-2",
  3: "lg:grid-cols-3",
  4: "lg:grid-cols-2 xl:grid-cols-4",
  5: "lg:grid-cols-3 xl:grid-cols-5",
};

/**
 * The workspace dashboard's loop-status layer: where the loop stands right
 * now — latest loop activity, the latest convergence run, what awaits the
 * caller's review, ship readiness, and brand standing — each card
 * deep-linking to its surface.
 */
export function LoopStatusRow({
  status,
  onOpenActivities,
  onOpenRuns,
  onOpenTasks,
  onOpenReview,
  onOpenDelivery,
  onOpenVoiceDashboard,
}: LoopStatusRowProps) {
  const { latestActivity, latestRun, openReviewTasks, ship, brand } = status;
  const showRun = latestRun !== undefined;
  const showShip = ship !== undefined;
  const showVoice = brand !== undefined;
  const visibleCards = 2 + (showRun ? 1 : 0) + (showShip ? 1 : 0) + (showVoice ? 1 : 0);

  const shippableLocales = ship ? ship.governed + ship.aiShippable : 0;
  const totalLocales = ship ? shippableLocales + ship.pending : 0;

  return (
    <div
      data-testid="loop-status"
      className={cn("grid gap-4 sm:grid-cols-2", GRID_COLS[visibleCards] ?? "lg:grid-cols-3")}
    >
      <StatusCard
        label="Loop activity"
        icon={<Activity />}
        footer="View activity"
        onOpen={onOpenActivities}
        testId="loop-card-activity"
      >
        {latestActivity ? (
          <div className="space-y-1.5">
            <p className="line-clamp-2 text-sm leading-relaxed">{latestActivity.summary}</p>
            <p className="text-xs text-muted-foreground">
              {relativeTime(latestActivity.created_at)}
            </p>
          </div>
        ) : (
          <p className="text-sm leading-relaxed text-muted-foreground">
            No loop activity yet. Runs, pushes, and checks report here.
          </p>
        )}
      </StatusCard>

      {showRun && (
        <StatusCard
          label="Latest run"
          icon={<RefreshCw />}
          footer="View runs"
          onOpen={onOpenRuns}
          testId="loop-card-run"
        >
          <div className="space-y-1.5">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-lg font-semibold leading-none">
                {RUN_STATE_LABELS[latestRun.state] ?? latestRun.state}
              </span>
              {latestRun.stallReason && (
                <Badge variant="outline" className="gap-1 text-[11px] font-normal">
                  <AlertTriangle className="h-3 w-3" />
                  {STALL_REASON_LABELS[latestRun.stallReason] ?? latestRun.stallReason}
                </Badge>
              )}
            </div>
            <p className="text-xs text-muted-foreground">
              {latestRun.projectName}
              {latestRun.projectName && latestRun.updatedAt ? " · " : ""}
              {latestRun.updatedAt ? relativeTime(latestRun.updatedAt) : ""}
            </p>
          </div>
        </StatusCard>
      )}

      <StatusCard
        label="Awaiting review"
        icon={<Eye />}
        footer={onOpenReview ? "Open review inbox" : "Open tasks"}
        onOpen={onOpenReview ?? onOpenTasks}
        testId="loop-card-review"
      >
        {typeof openReviewTasks !== "number" ? (
          <p className="text-sm text-muted-foreground" translate="no">
            ·
          </p>
        ) : openReviewTasks > 0 ? (
          <div className="flex items-baseline gap-2">
            <span className="text-3xl font-semibold tabular-nums leading-none">
              {openReviewTasks}
            </span>
            <span className="text-sm text-muted-foreground">
              <Plural count={openReviewTasks}>
                <One>open review task for you</One>
                <Other>open review tasks for you</Other>
              </Plural>
            </span>
          </div>
        ) : (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <CircleCheck className="h-4 w-4 shrink-0" />
            Nothing is waiting on you.
          </div>
        )}
      </StatusCard>

      {showShip && (
        <StatusCard
          label="Ship readiness"
          icon={<Rocket />}
          footer="Delivery dashboard"
          onOpen={onOpenDelivery}
          testId="loop-card-ship"
        >
          <div className="space-y-1.5">
            <div className="flex items-baseline gap-2">
              <span className="text-3xl font-semibold tabular-nums leading-none">
                {shippableLocales}
              </span>
              <span className="text-sm text-muted-foreground">
                <Plural count={totalLocales}>
                  <One>of {totalLocales} locale shippable</One>
                  <Other>of {totalLocales} locales shippable</Other>
                </Plural>
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {ship.governed} governed · {ship.aiShippable} AI-shippable · {ship.pending} pending
              {ship.countedProjects < ship.totalProjects
                ? ` · across ${ship.countedProjects} of ${ship.totalProjects} projects`
                : ""}
            </p>
          </div>
        </StatusCard>
      )}

      {showVoice && (
        <StatusCard
          label="Voice health"
          icon={<Palette />}
          footer="Voice dashboard"
          onOpen={onOpenVoiceDashboard}
          testId="loop-card-brand"
        >
          {brand.scoredProjects > 0 && brand.averageScore !== null ? (
            <div className="space-y-1.5">
              <div className="flex items-baseline gap-2">
                <span className="text-3xl font-semibold tabular-nums leading-none">
                  {brand.averageScore}
                </span>
                <span className="text-sm text-muted-foreground">
                  <Plural count={brand.scoredProjects}>
                    <One>average across {brand.scoredProjects} scored project</One>
                    <Other>average across {brand.scoredProjects} scored projects</Other>
                  </Plural>
                </span>
              </div>
              {brand.driftingProjects > 0 && (
                <Badge variant="outline" className="gap-1 text-[11px] font-normal">
                  <AlertTriangle className="h-3 w-3" />
                  {brand.driftingProjects} drifting
                </Badge>
              )}
            </div>
          ) : (
            <p className="text-sm leading-relaxed text-muted-foreground">
              No voice scores yet. Bind a profile and run checks to see standing.
            </p>
          )}
        </StatusCard>
      )}
    </div>
  );
}
