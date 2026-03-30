// Conversion between FlowSpec (steps format) and React Flow graph (nodes + edges).
// Uses auto-layout — no persisted positions.

import type { Node, Edge } from "@xyflow/react";
import type { FlowSpec, ToolInfo } from "./types";

const NODE_WIDTH = 200;
const NODE_GAP = 60;
const Y_CENTER = 100;

/**
 * Convert a steps-based FlowSpec into React Flow nodes and edges with auto-layout.
 * Generates: reader → tool1 → tool2 → ... → writer
 *
 * When a tool lookup map is provided, tool nodes include category and description metadata.
 */
export function stepsToGraph(
  spec: FlowSpec,
  toolMap?: Map<string, ToolInfo>,
): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = [];
  const edges: Edge[] = [];

  let x = 0;

  // Reader node
  nodes.push({
    id: "reader",
    type: "reader",
    position: { x, y: Y_CENTER },
    data: { label: "Input", formatName: "auto" },
  });
  x += NODE_WIDTH + NODE_GAP;

  // Tool nodes
  let prevId = "reader";
  spec.steps.forEach((step, i) => {
    const id = `tool-${i}`;
    const info = toolMap?.get(step.tool);
    nodes.push({
      id,
      type: "tool",
      position: { x, y: Y_CENTER },
      data: {
        label: step.label || step.tool,
        toolName: step.tool,
        config: step.config,
        category: info?.category || "pipeline",
        description: info?.description,
      },
    });
    edges.push({
      id: `e-${prevId}-${id}`,
      source: prevId,
      target: id,
    });
    prevId = id;
    x += NODE_WIDTH + NODE_GAP;
  });

  // Writer node
  nodes.push({
    id: "writer",
    type: "writer",
    position: { x, y: Y_CENTER },
    data: { label: "Output", formatName: "auto" },
  });
  edges.push({
    id: `e-${prevId}-writer`,
    source: prevId,
    target: "writer",
  });

  return { nodes, edges };
}

/**
 * Convert React Flow graph back to a FlowSpec (steps format).
 * Extracts tool nodes in left-to-right order by x position.
 */
export function graphToSteps(nodes: Node[]): FlowSpec {
  const toolNodes = nodes
    .filter((n) => n.type === "tool")
    .sort((a, b) => a.position.x - b.position.x);

  return {
    steps: toolNodes.map((n) => ({
      tool: (n.data.toolName as string) || "",
      config: n.data.config as Record<string, unknown> | undefined,
      label: n.data.label as string | undefined,
    })),
  };
}
