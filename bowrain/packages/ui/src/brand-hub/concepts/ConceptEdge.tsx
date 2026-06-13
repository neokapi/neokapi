// A floating, relation-typed edge on the concept graph (AD-021). The line
// treatment (colour, dash, arrowhead, label) follows the relation family from
// graph-style.ts; the path attaches to node borders via the floating geometry
// in graphFlow.ts.
import {
  BaseEdge,
  EdgeLabelRenderer,
  getBezierPath,
  useInternalNode,
  type EdgeProps,
  type Edge,
} from "@xyflow/react";
import type { RelationType } from "../../types/brand-graph";
import { relationEdgeStyle } from "./graph-style";
import { getEdgeParams } from "./graphFlow";

export interface ConceptEdgeData extends Record<string, unknown> {
  relation: RelationType;
  label: string;
  /** Outside the focused neighbourhood (faded back). */
  dimmed?: boolean;
}

export type ConceptFlowEdge = Edge<ConceptEdgeData, "concept">;

function ConceptEdgeImpl({ id, source, target, markerEnd, data }: EdgeProps<ConceptFlowEdge>) {
  const sourceNode = useInternalNode(source);
  const targetNode = useInternalNode(target);
  if (!sourceNode || !targetNode) return null;

  const { sx, sy, tx, ty, sourcePos, targetPos } = getEdgeParams(sourceNode, targetNode);
  const [path, labelX, labelY] = getBezierPath({
    sourceX: sx,
    sourceY: sy,
    sourcePosition: sourcePos,
    targetX: tx,
    targetY: ty,
    targetPosition: targetPos,
  });

  const relation: RelationType = data?.relation ?? "RELATED";
  const style = relationEdgeStyle(relation);
  const dimmed = data?.dimmed ?? false;

  return (
    <>
      <BaseEdge
        id={id}
        path={path}
        markerEnd={style.directed ? markerEnd : undefined}
        style={{
          stroke: style.color,
          strokeWidth: style.width,
          strokeDasharray: style.dash,
          opacity: dimmed ? 0.12 : 0.85,
        }}
      />
      {!dimmed && data?.label && (
        <EdgeLabelRenderer>
          <div
            style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
            className="pointer-events-none absolute rounded bg-background/80 px-1 py-px text-[9px] font-medium text-muted-foreground backdrop-blur-[1px]"
          >
            {data.label}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}

export const ConceptEdge = ConceptEdgeImpl;
