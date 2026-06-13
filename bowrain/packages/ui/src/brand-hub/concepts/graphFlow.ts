// Floating-edge geometry for the concept graph (AD-021). A small, well-worn
// React Flow recipe: edges attach to the border of each node along the line
// between node centres, so connections stay clean regardless of where two
// concepts sit relative to each other. Kept separate from the edge component so
// the maths reads on its own.
import { Position, type InternalNode, type Node } from "@xyflow/react";

interface Point {
  x: number;
  y: number;
}

function nodeDims(node: InternalNode<Node>): { w: number; h: number } {
  return {
    w: node.measured?.width ?? (node.width as number | undefined) ?? 184,
    h: node.measured?.height ?? (node.height as number | undefined) ?? 76,
  };
}

/** The point on `node`'s border that lies on the line toward `toward`'s centre. */
function intersection(node: InternalNode<Node>, toward: InternalNode<Node>): Point {
  const { w: nw, h: nh } = nodeDims(node);
  const { w: tw, h: th } = nodeDims(toward);
  const w = nw / 2;
  const h = nh / 2;

  const x2 = node.internals.positionAbsolute.x + w;
  const y2 = node.internals.positionAbsolute.y + h;
  const x1 = toward.internals.positionAbsolute.x + tw / 2;
  const y1 = toward.internals.positionAbsolute.y + th / 2;

  const xx1 = (x1 - x2) / (2 * w) - (y1 - y2) / (2 * h);
  const yy1 = (x1 - x2) / (2 * w) + (y1 - y2) / (2 * h);
  const a = 1 / (Math.abs(xx1) + Math.abs(yy1) || 1);
  const xx3 = a * xx1;
  const yy3 = a * yy1;
  const x = w * (xx3 + yy3) + x2;
  const y = h * (-xx3 + yy3) + y2;
  return { x, y };
}

/** Which side of `node` the intersection point sits on. */
function sideOf(node: InternalNode<Node>, point: Point): Position {
  const { w, h } = nodeDims(node);
  const nx = Math.round(node.internals.positionAbsolute.x);
  const ny = Math.round(node.internals.positionAbsolute.y);
  const px = Math.round(point.x);
  const py = Math.round(point.y);
  if (px <= nx + 1) return Position.Left;
  if (px >= nx + w - 1) return Position.Right;
  if (py <= ny + 1) return Position.Top;
  if (py >= ny + h - 1) return Position.Bottom;
  return Position.Top;
}

export interface EdgeParams {
  sx: number;
  sy: number;
  tx: number;
  ty: number;
  sourcePos: Position;
  targetPos: Position;
}

/** Source/target border points + sides for a floating edge between two nodes. */
export function getEdgeParams(source: InternalNode<Node>, target: InternalNode<Node>): EdgeParams {
  const sourcePoint = intersection(source, target);
  const targetPoint = intersection(target, source);
  return {
    sx: sourcePoint.x,
    sy: sourcePoint.y,
    tx: targetPoint.x,
    ty: targetPoint.y,
    sourcePos: sideOf(source, sourcePoint),
    targetPos: sideOf(target, targetPoint),
  };
}
