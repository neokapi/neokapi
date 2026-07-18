import { Badge, Card, CardContent, cn } from "@neokapi/ui-primitives";
import type { ReactNode } from "react";
import { Activity, AlertTriangle, ArrowRight, CircleCheck, Eye, Palette } from "./icons";

// ---------------------------------------------------------------------------
// Data shapes
//
// The row is purely presentational: the route (web) or host shell computes
// these summaries from data it already holds — the workspace activity feed,
// the caller's open task list, and the brand-compliance rollup — and feeds
// them in as props. Absent fields degrade card by card, never the whole row.
// ---------------------------------------------------------------------------

/** Most recent loop-relevant entry from the workspace activity feed. */
export interface LoopActivitySummary {
  /** Human-readable summary from the feed (server-composed). */
  summary: string;
  /** ISO timestamp of the activity. */
  created_at: string;
}

/** Workspace brand-compliance standing, folded from the brand rollup. */
export interface LoopBrandHealth {
  /** Rounded mean of project brand scores; null while nothing is scored. */
  averageScore: number | null;
  /** Projects with at least one stored score. */
  scoredProjects: number;
  /** Projects flagged as drifted or trending down. */
  driftingProjects: number;
}

/** The loop-status layer's data, one optional slot per card. */
export interface LoopStatusData {
  /** Latest loop activity (runs, pushes, checks, reviews); absent = none yet. */
  latestActivity?: LoopActivitySummary;
  /** Open review-type tasks for the current user; absent while loading. */
  openReviewTasks?: number;
  /** Brand rollup summary; absent hides the brand card entirely. */
  brand?: LoopBrandHealth;
}

export interface LoopStatusRowProps {
  status: LoopStatusData;
  /** Opens the workspace activity feed. */
  onOpenActivities?: () => void;
  /** Opens the workspace task queue. */
  onOpenTasks?: () => void;
  /** Opens the brand dashboard. */
  onOpenBrandDashboard?: () => void;
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

/**
 * The workspace dashboard's loop-status layer: where the loop stands right
 * now — latest loop activity, what awaits the caller's review, and brand
 * standing — each card deep-linking to its surface.
 */
export function LoopStatusRow({
  status,
  onOpenActivities,
  onOpenTasks,
  onOpenBrandDashboard,
}: LoopStatusRowProps) {
  const { latestActivity, openReviewTasks, brand } = status;
  const showBrand = brand !== undefined;

  return (
    <div
      data-testid="loop-status"
      className={cn("grid gap-4 sm:grid-cols-2", showBrand ? "lg:grid-cols-3" : "lg:grid-cols-2")}
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

      <StatusCard
        label="Awaiting review"
        icon={<Eye />}
        footer="Open tasks"
        onOpen={onOpenTasks}
        testId="loop-card-review"
      >
        {typeof openReviewTasks !== "number" ? (
          <p className="text-sm text-muted-foreground" translate="no">
            —
          </p>
        ) : openReviewTasks > 0 ? (
          <div className="flex items-baseline gap-2">
            <span className="text-3xl font-semibold tabular-nums leading-none">
              {openReviewTasks}
            </span>
            <span className="text-sm text-muted-foreground">
              open review task{openReviewTasks !== 1 ? "s" : ""} for you
            </span>
          </div>
        ) : (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <CircleCheck className="h-4 w-4 shrink-0" />
            Nothing is waiting on you.
          </div>
        )}
      </StatusCard>

      {showBrand && (
        <StatusCard
          label="Brand health"
          icon={<Palette />}
          footer="Brand dashboard"
          onOpen={onOpenBrandDashboard}
          testId="loop-card-brand"
        >
          {brand.scoredProjects > 0 && brand.averageScore !== null ? (
            <div className="space-y-1.5">
              <div className="flex items-baseline gap-2">
                <span className="text-3xl font-semibold tabular-nums leading-none">
                  {brand.averageScore}
                </span>
                <span className="text-sm text-muted-foreground">
                  average across {brand.scoredProjects} scored project
                  {brand.scoredProjects !== 1 ? "s" : ""}
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
              No brand scores yet. Bind a profile and run checks to see standing.
            </p>
          )}
        </StatusCard>
      )}
    </div>
  );
}
