import { BaseEdge, getSmoothStepPath, type EdgeProps } from "@xyflow/react";

/**
 * Custom edge that renders a dot traversing the path instead of
 * the default dashed-line animation.
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
  label,
  labelStyle,
  labelBgStyle,
  labelBgPadding,
  labelBgBorderRadius,
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

  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        style={style}
        markerEnd={markerEnd}
        label={label}
        labelStyle={labelStyle}
        labelBgStyle={labelBgStyle}
        labelBgPadding={labelBgPadding}
        labelBgBorderRadius={labelBgBorderRadius}
        labelX={labelX}
        labelY={labelY}
      />
      {/* Traversing dot */}
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
