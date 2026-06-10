import { BaseEdge, EdgeLabelRenderer, getSmoothStepPath, type EdgeProps } from "@xyflow/react";

/**
 * A clean smooth-step edge with an arrowhead. The data type crossing the edge
 * is shown by the port chips on the connected nodes' boundaries (see ToolNode's
 * BoundaryPorts), so the edge itself stays minimal — no mid-edge label.
 *
 * When `data.flowing` is set (the editor is showing a trace), a small stream of
 * dots travels the edge to visualize data moving through the pipeline. Each edge
 * uses the SAME travel DURATION (FLOW_DUR) regardless of its length, so every
 * node-to-node hop reads as equal time — a dot covering a long wrap edge (last
 * column → first column of the next row) simply moves faster than one on a short
 * in-row edge. `<animateMotion>` traverses the whole path in `dur` no matter how
 * long the path is, which gives exactly that constant-time-per-hop behaviour.
 *
 * When `data.traversed` is set (run review), a tiny count chip at the edge
 * midpoint shows how many parts crossed it at the playback cursor — the trace
 * lives on the graph instead of a separate timeline covering the canvas.
 */

// Constant per-edge travel time. Equal node-to-node cadence; speed scales with
// edge length so long wrap edges are covered quickly.
const FLOW_DUR = 1.1;
// Two dots per edge, half a cycle apart, for a steady stream.
const DOT_PHASES = [0, FLOW_DUR / 2];

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
  const traversed = (data as { traversed?: number } | undefined)?.traversed;

  return (
    <>
      <BaseEdge id={id} path={edgePath} style={style} markerEnd={markerEnd} />
      {flowing &&
        DOT_PHASES.map((phase, i) => (
          <circle key={i} r={3.5} fill="var(--primary)" className="pointer-events-none">
            <animateMotion
              dur={`${FLOW_DUR}s`}
              repeatCount="indefinite"
              path={edgePath}
              // Negative begin phase-shifts the dot into the loop so the two dots
              // are evenly spaced along the edge from the first frame.
              begin={`${-phase}s`}
            />
          </circle>
        ))}
      {traversed !== undefined && traversed > 0 && (
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
