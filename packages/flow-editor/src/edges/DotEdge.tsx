import { BaseEdge, getSmoothStepPath, type EdgeProps } from "@xyflow/react";

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
  const [edgePath] = getSmoothStepPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    borderRadius: 12,
  });

  const flowing = !!(data as { flowing?: boolean } | undefined)?.flowing;

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
    </>
  );
}
