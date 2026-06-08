// Conversion between FlowSpec (steps format) and React Flow graph (nodes + edges).
// Supports both sequential and parallel (fan-out/merge) topologies.
// Layout direction is configurable: horizontal (left-to-right) or vertical (top-to-bottom).

import { MarkerType, Position, type Node, type Edge } from "@xyflow/react";
import type { IOPort, FlowSpec, FlowStep, ToolInfo } from "./types";

export type LayoutDirection = "horizontal" | "vertical" | "serpentine";

const NODE_SIZE = 200; // primary axis node size estimate
const NODE_GAP = 60;
const CENTER = 200; // cross-axis center
const BRANCH_GAP = 80;

// Serpentine layout geometry.
const SERP_COL_W = 220; // horizontal stride between columns
const SERP_ROW_H = 150; // vertical stride between wrapped rows
const SERP_BRANCH_DY = 96; // vertical offset between parallel branches in a column

const EDGE_MARKER = {
  type: MarkerType.Arrow,
  width: 16,
  height: 16,
  color: "var(--muted-foreground)",
};

/** Port type names of a produced/consumed contract, for typed edge chips. */
function portTypes(fs?: IOPort[]): string[] | undefined {
  if (!fs || fs.length === 0) return undefined;
  return fs.map((f) => f.type);
}

function makeEdge(source: string, target: string, ports?: string[]): Edge {
  return {
    id: `e-${source}-${target}`,
    source,
    target,
    type: "dot",
    markerEnd: EDGE_MARKER,
    // The DotEdge renders these produced port types as typed chips at the edge
    // midpoint, so the data flowing between tools is legible (target, qa, …).
    ...(ports && ports.length > 0 ? { data: { ports } } : {}),
  };
}

/** Build the data payload for a sequential tool node. */
function makeToolNodeData(
  step: FlowStep,
  toolMap: Map<string, ToolInfo> | undefined,
  extraData?: Record<string, unknown>,
): Record<string, unknown> {
  const info = toolMap?.get(step.tool);
  return {
    label: step.label || info?.display_name || step.tool,
    toolName: step.tool,
    config: step.config,
    category: info?.category || "pipeline",
    description: info?.description,
    consumes: info?.consumes,
    produces: info?.produces,
    cardinality: info?.cardinality,
    defaultLocale: info?.default_locale,
    sideEffects: info?.side_effects,
    isSourceTransform: info?.isSourceTransform,
    valid: toolMap ? !!info : true,
    ...extraData,
  };
}

/**
 * Convert a steps-based FlowSpec into React Flow nodes and edges with auto-layout.
 *
 * A flow is composition only and owns no I/O (AD-026): reader and writer are no
 * longer graph nodes. The graph contains tool nodes only — the first tool(s)
 * have no incoming edge and the last no outgoing edge. Where content enters and
 * leaves is a binding (`spec.source` / `spec.sink`) rendered as fixed endpoint
 * pickers by the editor, not as nodes.
 *
 * Sequential steps produce a chain: tool1 → tool2 → tool3
 * Parallel steps produce fan-out/merge: prev → [branchA, branchB] → next
 *
 * Source-transform steps are emitted first (before the main steps) and carry
 * `stage: "source-transform"` on their node data so the UI can render them with
 * a distinct visual treatment.
 */
export function stepsToGraph(
  spec: FlowSpec,
  toolMap?: Map<string, ToolInfo>,
  direction: LayoutDirection = "vertical",
): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = [];
  const edges: Edge[] = [];
  const isVertical = direction === "vertical";

  let primary = 0; // x for horizontal, y for vertical
  let toolCounter = 0;

  const pos = (main: number, cross: number) =>
    isVertical ? { x: cross, y: main } : { x: main, y: cross };

  // No reader node: the first tool(s) are the entry point. `prevIds` starts empty
  // so the leading tools get no incoming edge.
  let prevIds: string[] = [];

  // --- Source-transform stage (leading, before main steps) ---
  (spec.sourceTransforms ?? []).forEach((step, stIndex) => {
    // Source-transforms are always sequential (no parallel fan-out in this stage).
    const id = `tool-${toolCounter++}`;

    nodes.push({
      id,
      type: "tool",
      position: pos(primary, CENTER),
      // `stIndex` records the position of this step in `spec.sourceTransforms`
      // so selection/edit/delete can resolve it by identity, not by tool name.
      data: makeToolNodeData(step, toolMap, { stage: "source-transform", stIndex }),
    });

    for (const prev of prevIds) {
      const prevNode = nodes.find((n) => n.id === prev);
      edges.push(makeEdge(prev, id, portTypes(prevNode?.data.produces as IOPort[] | undefined)));
    }

    prevIds = [id];
    primary += NODE_SIZE + NODE_GAP;
  });

  // --- Main stage ---
  spec.steps.forEach((step, stepIndex) => {
    if (step.parallel && step.parallel.length > 0) {
      // Fan-out: create parallel branch nodes spread along cross-axis
      const branchIds: string[] = [];
      const branchCount = step.parallel.length;
      const totalCross = (branchCount - 1) * (NODE_SIZE + BRANCH_GAP);
      const startCross = CENTER - totalCross / 2;

      for (let b = 0; b < branchCount; b++) {
        const branch = step.parallel[b];
        const id = `tool-${toolCounter++}`;
        const info = toolMap?.get(branch.tool);

        nodes.push({
          id,
          type: "tool",
          position: pos(primary, startCross + b * (NODE_SIZE + BRANCH_GAP)),
          data: {
            label: branch.label || info?.display_name || branch.tool,
            toolName: branch.tool,
            config: branch.config,
            category: info?.category || "pipeline",
            description: info?.description,
            consumes: info?.consumes,
            produces: info?.produces,
            cardinality: info?.cardinality,
            defaultLocale: info?.default_locale,
            sideEffects: info?.side_effects,
            isSourceTransform: info?.isSourceTransform,
            valid: toolMap ? !!info : true,
            parallel: true,
            // `stepIndex` points at the parallel wrapper in `spec.steps`;
            // `branchIndex` points at this branch within `step.parallel`. Both
            // are needed so config edits land on the branch, not the wrapper.
            stepIndex,
            branchIndex: b,
          },
        });

        for (const prev of prevIds) {
          const prevNode = nodes.find((n) => n.id === prev);
          edges.push(
            makeEdge(prev, id, portTypes(prevNode?.data.produces as IOPort[] | undefined)),
          );
        }

        branchIds.push(id);
      }

      prevIds = branchIds;
      primary += NODE_SIZE + NODE_GAP;
    } else {
      // Sequential step
      const id = `tool-${toolCounter++}`;

      nodes.push({
        id,
        type: "tool",
        position: pos(primary, CENTER),
        // `stepIndex` records the position of this step in `spec.steps`.
        data: makeToolNodeData(step, toolMap, { stepIndex }),
      });

      for (const prev of prevIds) {
        const prevNode = nodes.find((n) => n.id === prev);
        edges.push(makeEdge(prev, id, portTypes(prevNode?.data.produces as IOPort[] | undefined)));
      }

      prevIds = [id];
      primary += NODE_SIZE + NODE_GAP;
    }
  });

  // No writer node: the last tool(s) are the exit point and get no outgoing edge.
  // `prevIds` is intentionally left dangling.

  // Inject layout direction into all nodes so handles render correctly.
  for (const node of nodes) {
    node.data.layoutDirection = direction;
  }

  return { nodes, edges };
}

/** Where an endpoint (Source/Sink) sits and which side its handle faces. */
export interface EndpointGeom {
  x: number;
  y: number;
  handlePosition: Position;
}

/**
 * Serpentine (boustrophedon) auto-layout: fold the chain into rows that wrap to
 * fit `cols` columns, alternating direction each row (row 0 left→right, row 1
 * right→left, …). Per-node handle sides flip with the row so the inter-row
 * U-turns route cleanly. Built by repositioning the horizontal graph, so node
 * identity + edges are unchanged; only positions and the in/out handle sides
 * (data.inPosition / data.outPosition) are rewritten. Returns the geometry for
 * the Source/Sink endpoints so the editor can place them in the same flow.
 */
export function serpentineGraph(
  spec: FlowSpec,
  toolMap: Map<string, ToolInfo> | undefined,
  cols: number,
): { nodes: Node[]; edges: Edge[]; ends?: { source: EndpointGeom; sink: EndpointGeom } } {
  const base = stepsToGraph(spec, toolMap, "horizontal");
  const columns = Math.max(1, cols);
  const toolNodes = base.nodes.filter((n) => n.type === "tool");
  if (toolNodes.length === 0) return base;

  // Order columns: source-transform steps first (by stIndex), then main steps
  // (by stepIndex). Parallel branches share a column.
  const stIndices = new Set<number>();
  for (const n of toolNodes) {
    if (n.data.stage === "source-transform") stIndices.add(n.data.stIndex as number);
  }
  const numST = stIndices.size;
  const colOrderOf = (n: Node) =>
    n.data.stage === "source-transform"
      ? (n.data.stIndex as number)
      : numST + (n.data.stepIndex as number);

  const colMap = new Map<number, Node[]>();
  for (const n of toolNodes) {
    const key = colOrderOf(n);
    const group = colMap.get(key);
    if (group) group.push(n);
    else colMap.set(key, [n]);
  }
  const colKeys = [...colMap.keys()].sort((a, b) => a - b);

  const colCenterY = (group: Node[]) =>
    group.reduce((sum, n) => sum + n.position.y, 0) / group.length;

  colKeys.forEach((key, g) => {
    const row = Math.floor(g / columns);
    const posInRow = g % columns;
    const evenRow = row % 2 === 0;
    const visualCol = evenRow ? posInRow : columns - 1 - posInRow;
    const x = visualCol * SERP_COL_W;
    const y = row * SERP_ROW_H;
    const inPosition = evenRow ? Position.Left : Position.Right;
    const outPosition = evenRow ? Position.Right : Position.Left;
    const group = colMap.get(key)!;
    const span = (group.length - 1) * SERP_BRANCH_DY;
    group.forEach((n, b) => {
      n.position = { x, y: y + b * SERP_BRANCH_DY - span / 2 };
      n.data.inPosition = inPosition;
      n.data.outPosition = outPosition;
    });
  });

  // Source sits left of the first column (which always faces in from the left);
  // Sink sits past the last column on its outgoing side.
  const firstGroup = colMap.get(colKeys[0])!;
  const source: EndpointGeom = {
    x: -SERP_COL_W,
    y: colCenterY(firstGroup),
    handlePosition: Position.Right,
  };
  const lastG = colKeys.length - 1;
  const lastEven = Math.floor(lastG / columns) % 2 === 0;
  const lastGroup = colMap.get(colKeys[lastG])!;
  const lastX = lastGroup[0].position.x;
  const sink: EndpointGeom = lastEven
    ? { x: lastX + SERP_COL_W, y: colCenterY(lastGroup), handlePosition: Position.Left }
    : { x: lastX - SERP_COL_W, y: colCenterY(lastGroup), handlePosition: Position.Right };

  return { nodes: base.nodes, edges: base.edges, ends: { source, sink } };
}

/**
 * Convert React Flow graph back to a FlowSpec (steps format).
 *
 * Groups nodes at the same primary-axis position — if multiple tool nodes
 * share that position they form a parallel step; otherwise they're sequential.
 *
 * Nodes whose `data.stage` is "source-transform" are collected into
 * `spec.sourceTransforms` (in primary-axis order); the rest become `spec.steps`.
 *
 * The graph holds tool nodes only; the `source` / `sink` bindings are not nodes,
 * so the caller passes them via `bindings` to carry them through unchanged.
 */
export function graphToSteps(
  nodes: Node[],
  direction: LayoutDirection = "vertical",
  bindings?: { source?: string; sink?: string },
): FlowSpec {
  const withBindings = (spec: FlowSpec): FlowSpec => {
    if (bindings?.source) spec.source = bindings.source;
    if (bindings?.sink) spec.sink = bindings.sink;
    return spec;
  };

  const isVertical = direction === "vertical";
  const primary = (n: Node) => (isVertical ? n.position.y : n.position.x);
  const cross = (n: Node) => (isVertical ? n.position.x : n.position.y);

  const toolNodes = nodes.filter((n) => n.type === "tool").sort((a, b) => primary(a) - primary(b));

  if (toolNodes.length === 0) return withBindings({ steps: [] });

  // Partition into source-transform nodes and main nodes.
  const stNodes = toolNodes.filter((n) => n.data.stage === "source-transform");
  const mainNodes = toolNodes.filter((n) => n.data.stage !== "source-transform");

  // Source-transform nodes are always sequential — convert directly.
  const sourceTransforms: FlowStep[] = stNodes.map((n) => ({
    tool: (n.data.toolName as string) || "",
    config: n.data.config as Record<string, unknown> | undefined,
    label: n.data.label as string | undefined,
  }));

  if (mainNodes.length === 0) {
    const result: FlowSpec = { steps: [] };
    if (sourceTransforms.length > 0) result.sourceTransforms = sourceTransforms;
    return withBindings(result);
  }

  // Group main nodes by primary-axis position (with tolerance for layout jitter).
  const groups: Node[][] = [];
  let currentGroup: Node[] = [mainNodes[0]];

  for (let i = 1; i < mainNodes.length; i++) {
    if (Math.abs(primary(mainNodes[i]) - primary(currentGroup[0])) < NODE_SIZE / 2) {
      currentGroup.push(mainNodes[i]);
    } else {
      groups.push(currentGroup);
      currentGroup = [mainNodes[i]];
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
      // Multiple nodes at the same primary position = parallel step.
      // Sort by cross-axis for stable ordering.
      group.sort((a, b) => cross(a) - cross(b));
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

  const result: FlowSpec = { steps };
  if (sourceTransforms.length > 0) result.sourceTransforms = sourceTransforms;
  return withBindings(result);
}
