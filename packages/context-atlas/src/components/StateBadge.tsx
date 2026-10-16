import { cn } from "@neokapi/ui-primitives";
import { stateLabel } from "../labels";
import { type StateTone, stateTone } from "../model/state";

// The badge reads the same scale StatusBadge does: filled green once a person
// established it, the info blue while it is a suggestion, amber ink while it
// waits on a disagreement or a moved source, muted once closed.
const TONE_CLASS: Record<StateTone, string> = {
  settled: "bg-success text-success-foreground",
  pending: "bg-info/12 text-info dark:bg-info/20",
  attention: "border border-warning/60 text-warning-ink",
  middle: "bg-foreground/10 text-foreground",
  start: "bg-muted text-muted-foreground",
  closed: "bg-muted text-muted-foreground line-through decoration-muted-foreground/60",
  unknown: "bg-muted text-muted-foreground",
  absent: "border border-dashed border-border text-muted-foreground",
};

export interface StateBadgeProps {
  state: string;
  compact?: boolean;
  className?: string;
}

/** A record's state, as a badge with its word. */
export function StateBadge({ state, compact = false, className }: StateBadgeProps) {
  const tone = stateTone(state);
  return (
    <span
      data-slot="atlas-state"
      data-state={state}
      data-tone={tone}
      className={cn(
        "inline-flex w-fit shrink-0 items-center rounded-4xl font-medium whitespace-nowrap",
        compact ? "h-4 px-1.5 text-[11px]" : "h-5 px-2 text-xs",
        TONE_CLASS[tone],
        className,
      )}
    >
      {stateLabel(state)}
    </span>
  );
}
