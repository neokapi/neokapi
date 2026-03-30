import { useMemo } from "react";
import { Clock, AlertCircle, CheckCircle2 } from "lucide-react";
import type { TraceEvent, NodeTraceStats } from "./traceTypes";
import { computeNodeStats } from "./traceTypes";
import { theme } from "./theme";

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
      style={{
        borderTop: `1px solid ${theme.border}`,
        background: theme.bg,
        padding: "8px 12px",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 8 }}>
        <Clock size={12} style={{ color: theme.fgMuted }} />
        <span style={{ fontSize: 11, fontWeight: 600, color: theme.fg }}>
          Trace
        </span>
        {totalDurationUs !== undefined && (
          <span style={{ fontSize: 10, color: theme.fgMuted, marginLeft: "auto" }}>
            Total: {formatDuration(totalDurationUs)}
          </span>
        )}
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        {sortedNodes.map(([nodeId, s]) => {
          const name = nodeNames?.get(nodeId) ?? nodeId;
          const barWidth = Math.max(2, (s.durationUs / maxDuration) * 100);

          return (
            <div key={nodeId} style={{ display: "flex", alignItems: "center", gap: 8 }}>
              {/* Status icon */}
              {s.hasError ? (
                <AlertCircle size={12} style={{ color: theme.destructive, flexShrink: 0 }} />
              ) : (
                <CheckCircle2 size={12} style={{ color: "oklch(0.65 0.15 145)", flexShrink: 0 }} />
              )}

              {/* Node name */}
              <span
                style={{
                  fontSize: 11,
                  color: s.hasError ? theme.destructive : theme.fg,
                  fontWeight: 500,
                  width: 100,
                  flexShrink: 0,
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                }}
                title={name}
              >
                {name}
              </span>

              {/* Duration bar */}
              <div
                style={{
                  flex: 1,
                  height: 6,
                  borderRadius: 3,
                  background: theme.bgMuted,
                  overflow: "hidden",
                }}
              >
                <div
                  style={{
                    height: "100%",
                    width: `${barWidth}%`,
                    borderRadius: 3,
                    background: s.hasError ? theme.destructive : theme.accent,
                    transition: "width 300ms ease",
                  }}
                />
              </div>

              {/* Stats */}
              <span style={{ fontSize: 10, color: theme.fgMuted, width: 50, textAlign: "right", flexShrink: 0 }}>
                {formatDuration(s.durationUs)}
              </span>
              <span style={{ fontSize: 10, color: theme.fgMuted, width: 40, textAlign: "right", flexShrink: 0 }}>
                {s.partsProcessed} pts
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
