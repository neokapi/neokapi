// Conversion between FlowSpec (steps format) and React Flow graph (nodes + edges).
// Supports both sequential and parallel (fan-out/merge) topologies.

import type { Node, Edge } from "@xyflow/react";
import type { FlowSpec, FlowStep, ToolInfo } from "./types";

const NODE_WIDTH = 200;
const NODE_GAP = 60;
const Y_CENTER = 100;
const BRANCH_GAP = 80;

/**
 * Convert a steps-based FlowSpec into React Flow nodes and edges with auto-layout.
 *
 * Sequential steps produce a linear chain: reader → tool1 → tool2 → writer
 * Parallel steps produce fan-out/merge: prev → [branchA, branchB] → next
 */
export function stepsToGraph(
  spec: FlowSpec,
  toolMap?: Map<string, ToolInfo>,
): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = [];
  const edges: Edge[] = [];

  let x = 0;
  let toolCounter = 0;

  // Reader node
  nodes.push({
    id: "reader",
    type: "reader",
    position: { x, y: Y_CENTER },
    data: { label: "Input", formatName: "auto" },
  });
  x += NODE_WIDTH + NODE_GAP;

  let prevIds = ["reader"];

  for (const step of spec.steps) {
    if (step.parallel && step.parallel.length > 0) {
      // Fan-out: create parallel branch nodes
      const branchIds: string[] = [];
      const branchCount = step.parallel.length;
      const totalHeight = (branchCount - 1) * (60 + BRANCH_GAP);
      const startY = Y_CENTER - totalHeight / 2;

      for (let b = 0; b < branchCount; b++) {
        const branch = step.parallel[b];
        const id = `tool-${toolCounter++}`;
        const info = toolMap?.get(branch.tool);

        nodes.push({
          id,
          type: "tool",
          position: { x, y: startY + b * (60 + BRANCH_GAP) },
          data: {
            label: branch.label || branch.tool,
            toolName: branch.tool,
            config: branch.config,
            category: info?.category || "pipeline",
            description: info?.description,
            parallel: true,
          },
        });

        // Edges from all previous nodes to this branch
        for (const prev of prevIds) {
          edges.push({
            id: `e-${prev}-${id}`,
            source: prev,
            target: id,
          });
        }

        branchIds.push(id);
      }

      prevIds = branchIds;
      x += NODE_WIDTH + NODE_GAP;
    } else {
      // Sequential step
      const id = `tool-${toolCounter++}`;
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

      for (const prev of prevIds) {
        edges.push({
          id: `e-${prev}-${id}`,
          source: prev,
          target: id,
        });
      }

      prevIds = [id];
      x += NODE_WIDTH + NODE_GAP;
    }
  }

  // Writer node
  nodes.push({
    id: "writer",
    type: "writer",
    position: { x, y: Y_CENTER },
    data: { label: "Output", formatName: "auto" },
  });
  for (const prev of prevIds) {
    edges.push({
      id: `e-${prev}-writer`,
      source: prev,
      target: "writer",
    });
  }

  return { nodes, edges };
}

/**
 * Convert React Flow graph back to a FlowSpec (steps format).
 *
 * Groups nodes at the same X position — if multiple tool nodes share
 * an X position they form a parallel step; otherwise they're sequential.
 */
export function graphToSteps(nodes: Node[]): FlowSpec {
  const toolNodes = nodes
    .filter((n) => n.type === "tool")
    .sort((a, b) => a.position.x - b.position.x);

  if (toolNodes.length === 0) return { steps: [] };

  // Group by X position (with tolerance for layout jitter).
  const groups: Node[][] = [];
  let currentGroup: Node[] = [toolNodes[0]];

  for (let i = 1; i < toolNodes.length; i++) {
    if (Math.abs(toolNodes[i].position.x - currentGroup[0].position.x) < NODE_WIDTH / 2) {
      currentGroup.push(toolNodes[i]);
    } else {
      groups.push(currentGroup);
      currentGroup = [toolNodes[i]];
    }
  }
  groups.push(currentGroup);

  // Convert groups to steps.
  const steps: FlowStep[] = [];
  for (const group of groups) {
    if (group.length === 1) {
      const n = group[0];
      steps.push({
        tool: (n.data.toolName as string) || "",
        config: n.data.config as Record<string, unknown> | undefined,
        label: n.data.label as string | undefined,
      });
    } else {
      // Multiple nodes at the same X = parallel step.
      // Sort by Y for stable ordering.
      group.sort((a, b) => a.position.y - b.position.y);
      steps.push({
        tool: "",
        parallel: group.map((n) => ({
          tool: (n.data.toolName as string) || "",
          config: n.data.config as Record<string, unknown> | undefined,
          label: n.data.label as string | undefined,
        })),
      });
    }
  }

  return { steps };
}
