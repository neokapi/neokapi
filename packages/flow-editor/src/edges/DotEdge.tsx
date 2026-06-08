import { BaseEdge, getSmoothStepPath, type EdgeProps } from "@xyflow/react";

/**
 * Custom edge: a thin line with an arrowhead and a subtle dot traversing it to
 * convey flow direction. The data type crossing the edge is shown by the port
 * chips on the connected nodes' boundaries (see ToolNode's BoundaryPorts), so
 * the edge itself stays clean — no mid-edge label.
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

  const dotId = `dot-${id}`;

  return (
    <>
      <BaseEdge id={id} path={edgePath} style={style} markerEnd={markerEnd} />
      {/* Traversing dot conveys flow direction */}
      <circle r={3} fill="var(--primary)" opacity={0.7}>
        <animateMotion dur="2s" repeatCount="indefinite" keyPoints="0;1" keyTimes="0;1">
          <mpath href={`#${dotId}`} />
        </animateMotion>
      </circle>
      {/* Hidden path for animateMotion reference (same shape as the edge) */}
      <path id={dotId} d={edgePath} fill="none" stroke="none" />
    </>
  );
}
