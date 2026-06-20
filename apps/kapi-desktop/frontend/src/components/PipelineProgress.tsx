import { Loader2 } from "lucide-react";
import { Badge, cn } from "@neokapi/ui-primitives";
import type { StepSnapshot } from "../context/JobFeedContext";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type StepState = "pending" | "active" | "done";

export interface PipelineStep {
  /** Tool name (e.g., "translate"). */
  tool: string;
}

export interface PipelineProgressProps {
  /** Pipeline step definitions. */
  steps: PipelineStep[];
  /** Latest metrics snapshots from the backend. */
  snapshots?: StepSnapshot[];
  /** Overall run state — overrides per-step derivation. */
  runState?: "idle" | "running" | "complete" | "error" | "canceled";
  /** Additional class names on the root container. */
  className?: string;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Derives the visual state of a pipeline step from its metrics snapshot
 * and the overall run state.
 */
export function deriveStepState(
  snap: StepSnapshot | undefined,
  runState: PipelineProgressProps["runState"],
): StepState {
  if (runState === "idle") return "pending";
  if (runState === "complete" || runState === "error" || runState === "canceled") return "done";
  if (!snap || snap.parts_in === 0) return "pending";
  if (snap.parts_in > snap.parts_out) return "active";
  return "done";
}

// ---------------------------------------------------------------------------
// StepBadge
// ---------------------------------------------------------------------------

export interface StepBadgeProps {
  /** Tool name displayed inside the badge. */
  name: string;
  /** Visual state of this step. */
  state: StepState;
  /** Parts-in / parts-out counters (omitted when pending). */
  snapshot?: StepSnapshot;
  className?: string;
}

export function StepBadge({ name, state, snapshot, className }: StepBadgeProps) {
  return (
    <Badge
      variant={state === "active" ? "default" : state === "done" ? "secondary" : "outline"}
      className={cn(
        "gap-1 transition-all duration-300",
        state === "active" && "shadow-md ring-1 ring-primary/30",
        state === "done" &&
          "bg-green-500/15 text-green-600 dark:bg-green-500/20 dark:text-green-400",
        className,
      )}
    >
      {state === "active" && <Loader2 size={10} className="animate-spin" />}
      {name}
      {snapshot && snapshot.parts_in > 0 && (
        <span className="text-[10px] opacity-60 tabular-nums">
          {snapshot.parts_out}/{snapshot.parts_in}
        </span>
      )}
    </Badge>
  );
}

// ---------------------------------------------------------------------------
// PipelineProgress
// ---------------------------------------------------------------------------

/**
 * Visualizes a streaming pipeline's step-by-step progress.
 *
 * Each step is rendered as a `StepBadge` connected by arrow separators.
 * The visual state (pending / active / done) is derived from the latest
 * `StepSnapshot` counters emitted by the backend's `PipelineMetrics`.
 */
export function PipelineProgress({
  steps,
  snapshots = [],
  runState = "idle",
  className,
}: PipelineProgressProps) {
  return (
    <div className={cn("flex flex-wrap items-center gap-2", className)}>
      {steps.map((step, i) => {
        const snap = snapshots.find((s) => s.name === step.tool);
        const state = deriveStepState(snap, runState);

        return (
          <div key={i} className="flex items-center gap-2">
            {i > 0 && (
              <span
                className={cn(
                  "text-xs text-muted-foreground transition-colors duration-300",
                  state === "active" && "text-primary",
                )}
              >
                &rarr;
              </span>
            )}
            <StepBadge name={step.tool} state={state} snapshot={snap} />
          </div>
        );
      })}
    </div>
  );
}
