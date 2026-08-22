import { cn } from "@neokapi/ui-primitives";
import type { DriftResult } from "./types";

interface DriftAlertProps {
  drift: DriftResult;
  /** Show a "stable" confirmation when there is no drift (default false: render nothing). */
  showStable?: boolean;
  className?: string;
}

/**
 * DriftAlert surfaces a brand-compliance drift result. When compliance has
 * drifted it renders a warning banner with the recent vs. baseline average and
 * the reason; otherwise it renders nothing (or, with showStable, a quiet
 * confirmation).
 */
export function DriftAlert({ drift, showStable = false, className }: DriftAlertProps) {
  if (!drift.drifted) {
    if (!showStable) return null;
    return (
      <div
        className={cn(
          "rounded-md border border-success/30 bg-success/5 px-3 py-2 text-sm text-success",
          className,
        )}
      >
        Brand compliance is stable — recent average {drift.recent_avg.toFixed(1)} over the last{" "}
        {drift.recent_days} days.
      </div>
    );
  }

  return (
    <div
      role="alert"
      className={cn(
        "rounded-md border border-warning/40 bg-warning/10 px-3 py-2 text-sm",
        className,
      )}
    >
      <p className="font-medium text-warning">Brand compliance is drifting</p>
      <p className="text-muted-foreground">
        Recent average{" "}
        <span className="font-medium text-foreground">{drift.recent_avg.toFixed(1)}</span> vs.
        baseline{" "}
        <span className="font-medium text-foreground">{drift.baseline_avg.toFixed(1)}</span> (down{" "}
        {drift.drop.toFixed(1)} over {drift.recent_days} days)
        {drift.reason ? ` — ${drift.reason}.` : "."}
      </p>
    </div>
  );
}
