import { BaseEdge, EdgeLabelRenderer, getSmoothStepPath, type EdgeProps } from "@xyflow/react";
import { PortChip } from "../nodes/PortChip";

/**
 * Custom edge that renders a dot traversing the path instead of the default
 * dashed-line animation, and labels the midpoint with typed port chips for the
 * data flowing across it (from the upstream node's produces, via edge.data.ports).
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
  data,
}: EdgeProps) {
  const [edgePath, labelX, labelY] = getSmoothStepPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    borderRadius: 12,
  });

  const dotId = `dot-${id}`;
  const ports = (data?.ports as string[] | undefined) ?? [];

  return (
    <>
      <BaseEdge id={id} path={edgePath} style={style} markerEnd={markerEnd} />
      {/* Traversing dot */}
      <circle r={3} fill="var(--primary)" opacity={0.7}>
        <animateMotion dur="2s" repeatCount="indefinite" keyPoints="0;1" keyTimes="0;1">
          <mpath href={`#${dotId}`} />
        </animateMotion>
      </circle>
      {/* Hidden path for animateMotion reference (same shape as the edge) */}
      <path id={dotId} d={edgePath} fill="none" stroke="none" />
      {/* Typed port chips at the edge midpoint */}
      {ports.length > 0 && (
        <EdgeLabelRenderer>
          <div
            className="nodrag nopan absolute flex items-center gap-0.5 rounded bg-background/85 px-0.5 py-px"
            style={{
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              pointerEvents: "all",
            }}
          >
            {ports.slice(0, 3).map((p, i) => (
              <PortChip key={`${id}-${p}-${i}`} type={p} />
            ))}
            {ports.length > 3 && (
              <span className="text-[8px] text-muted-foreground">+{ports.length - 3}</span>
            )}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}
