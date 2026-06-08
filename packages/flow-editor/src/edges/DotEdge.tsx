import { BaseEdge, getSmoothStepPath, type EdgeProps } from "@xyflow/react";

/**
 * A clean smooth-step edge with an arrowhead. The data type crossing the edge
 * is shown by the port chips on the connected nodes' boundaries (see ToolNode's
 * BoundaryPorts), so the edge itself stays minimal — no mid-edge label, no
 * animation.
 */
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

  return <BaseEdge id={id} path={edgePath} style={style} markerEnd={markerEnd} />;
}
