import { useMemo } from "react";
import { Clock, AlertCircle, CheckCircle2 } from "lucide-react";
import { cn, PanelHeader } from "@neokapi/ui-primitives";
import type { TraceEvent, NodeTraceStats } from "./traceTypes";
import { computeNodeStats } from "./traceTypes";

interface TraceTimelineProps {
  events: TraceEvent[];
  /** Node names keyed by ID for display. */
  nodeNames?: Map<string, string>;
  totalDurationUs?: number;
}

function formatDuration(us: number): string {
  if (us < 1000) return `${us}µs`;
  if (us < 1_000_000) return `${(us / 1000).toFixed(1)}ms`;
  return `${(us / 1_000_000).toFixed(2)}s`;
}

/** Single bar in the trace timeline showing a node's execution stats. */
function TimelineBar({
  name,
  stats: s,
  maxDuration,
}: {
  name: string;
  stats: NodeTraceStats;
  maxDuration: number;
}) {
  const barWidth = Math.max(2, (s.durationUs / maxDuration) * 100);

  return (
    <div className="flex items-center gap-2">
      {/* Status icon */}
      {s.hasError ? (
        <AlertCircle size={12} className="shrink-0 text-destructive" />
      ) : (
        <CheckCircle2 size={12} className="shrink-0" style={{ color: "oklch(0.65 0.15 145)" }} />
      )}

      {/* Node name */}
      <span
        className={cn(
          "w-[100px] shrink-0 truncate text-[11px] font-medium",
          s.hasError ? "text-destructive" : "text-foreground",
        )}
        title={name}
      >
        {name}
      </span>

      {/* Duration bar */}
      <div className="flex-1 h-1.5 rounded-sm overflow-hidden bg-muted">
        <div
          className={cn("h-full rounded-sm", s.hasError ? "bg-destructive" : "bg-accent")}
          style={{
            width: `${barWidth}%`,
            animation: "barFill 0.3s ease-out",
            transition: "width 300ms ease",
          }}
        />
      </div>

      {/* Stats */}
      <span className="w-[50px] shrink-0 text-right text-[10px] text-muted-foreground">
        {formatDuration(s.durationUs)}
      </span>
      <span className="w-[40px] shrink-0 text-right text-[10px] text-muted-foreground">
        {s.partsProcessed} parts
      </span>
    </div>
  );
}

/**
 * Horizontal trace timeline showing per-node execution statistics.
 * Displays part counts, duration, and error state for each node.
 */
export function TraceTimeline({ events, nodeNames, totalDurationUs }: TraceTimelineProps) {
  const stats = useMemo(() => computeNodeStats(events), [events]);

  const sortedNodes = useMemo(() => {
    const entries = [...stats.entries()];
    // Sort by first appearance in events.
    const firstSeen = new Map<string, number>();
    for (const evt of events) {
      if (!firstSeen.has(evt.nodeId)) firstSeen.set(evt.nodeId, evt.ts);
    }
    entries.sort((a, b) => (firstSeen.get(a[0]) ?? 0) - (firstSeen.get(b[0]) ?? 0));
    return entries;
  }, [stats, events]);

  if (sortedNodes.length === 0) return null;

  const maxDuration = Math.max(...sortedNodes.map(([, s]) => s.durationUs), 1);

  return (
    <div
      className="border-t border-border bg-background"
      style={{ animation: "slideDrawer 0.2s ease-out" }}
    >
      <PanelHeader className="border-b-0">
        <Clock size={12} className="text-muted-foreground" />
        <span className="text-[11px] font-semibold text-foreground">Trace</span>
        {totalDurationUs !== undefined && (
          <span className="ml-auto text-[10px] text-muted-foreground">
            Total: {formatDuration(totalDurationUs)}
          </span>
        )}
      </PanelHeader>

      <div className="flex flex-col gap-1 px-3 pb-2">
        {sortedNodes.map(([nodeId, s]) => (
          <TimelineBar
            key={nodeId}
            name={nodeNames?.get(nodeId) ?? nodeId}
            stats={s}
            maxDuration={maxDuration}
          />
        ))}
      </div>
    </div>
  );
}
