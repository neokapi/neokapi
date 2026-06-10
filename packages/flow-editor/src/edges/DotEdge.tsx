import { BaseEdge, EdgeLabelRenderer, getSmoothStepPath, type EdgeProps } from "@xyflow/react";

/**
 * A clean smooth-step edge with an arrowhead. The data type crossing the edge
 * is shown by the port chips on the connected nodes' boundaries (see ToolNode's
 * BoundaryPorts), so the edge itself stays minimal — no mid-edge label.
 *
 * Run review renders the trace literally on the edge:
 *  - `data.transit` — parts mid-hop on this edge at the playback cursor
 *    (between exiting the source and entering the target). One dot per part:
 *    animated along the path while `data.flowing` (the transport is playing),
 *    frozen at the midpoint while paused. No decorative looping — a dot means
 *    a part is actually crossing here, right now.
 *  - `data.traversed` — how many parts have crossed at the cursor, as a tiny
 *    count chip at the midpoint. The trace lives on the graph instead of a
 *    separate timeline covering the canvas.
 *
 * Each edge uses the SAME travel DURATION (FLOW_DUR) regardless of length, so
 * every node-to-node hop reads as equal time — `<animateMotion>` traverses the
 * whole path in `dur` no matter how long it is.
 */

// Constant per-edge travel time. Equal node-to-node cadence; speed scales with
// edge length so long wrap edges are covered quickly.
const FLOW_DUR = 1.1;
// Spread multiple in-transit dots along the cycle so they don't overlap.
const dotPhase = (i: number, n: number) => (i * FLOW_DUR) / Math.max(1, n);

export function DotEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style,
  markerEnd,
  data,
}: EdgeProps) {
  // For a wrap edge the editor supplies wrapCenterY — the middle of the gap below
  // the source row — so the horizontal sweep routes through that gap and clears a
  // tall parallel group, instead of cutting through it at the default Y midpoint.
  const wrapCenterY = (data as { wrapCenterY?: number } | undefined)?.wrapCenterY;
  const [edgePath, labelX, labelY] = getSmoothStepPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    borderRadius: 12,
    ...(wrapCenterY !== undefined ? { centerY: wrapCenterY } : {}),
  });

  const flowing = !!(data as { flowing?: boolean } | undefined)?.flowing;
  const traversed = (data as { traversed?: number } | undefined)?.traversed ?? 0;
  const transit = (data as { transit?: number } | undefined)?.transit ?? 0;
  const transitDots = Math.min(transit, 4); // cap the dots; the chip has the number

  return (
    <>
      <BaseEdge id={id} path={edgePath} style={style} markerEnd={markerEnd} />
      {/* One dot per in-transit part: moving while playing, frozen mid-edge
          while paused — the dot IS the part. */}
      {transitDots > 0 &&
        flowing &&
        Array.from({ length: transitDots }, (_, i) => (
          <circle key={i} r={3.5} fill="var(--primary)" className="pointer-events-none">
            <animateMotion
              dur={`${FLOW_DUR}s`}
              repeatCount="indefinite"
              path={edgePath}
              // Phase-shift so simultaneous in-transit parts are spaced apart.
              begin={`${-dotPhase(i, transitDots)}s`}
            />
          </circle>
        ))}
      {transitDots > 0 && !flowing && (
        <circle
          r={3.5}
          cx={labelX}
          cy={labelY - 8}
          fill="var(--primary)"
          className="pointer-events-none"
        >
          <title>{`${transit} part(s) crossing this edge at the cursor`}</title>
        </circle>
      )}
      {traversed > 0 && (
        <EdgeLabelRenderer>
          <div
            className="pointer-events-none absolute z-[1] rounded-full border border-border bg-card px-1 py-px font-mono text-[8px] font-semibold text-muted-foreground"
            style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
            title={`${traversed} part(s) crossed this edge`}
          >
            {traversed}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}
