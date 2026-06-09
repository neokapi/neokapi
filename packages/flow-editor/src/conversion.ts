// Conversion between FlowSpec (steps format) and React Flow graph (nodes + edges).
// Supports both sequential and parallel (fan-out/merge) topologies.
// Layout direction is configurable: horizontal (left-to-right) or vertical (top-to-bottom).

import { MarkerType, Position, type Node, type Edge } from "@xyflow/react";
import type { IOPort, FlowSpec, FlowStep, ToolInfo } from "./types";

export type LayoutDirection = "horizontal" | "vertical" | "serpentine";

const NODE_SIZE = 200; // primary axis node size estimate
const NODE_GAP = 60;
const CENTER = 200; // cross-axis center

// Serpentine layout geometry. SERP_COL_W must exceed the widest node so columns
// never overlap; SERP_ROW_H leaves room for the satellite chips above each node.
export const SERP_COL_W = 240; // horizontal stride between columns (node max-w 200 + side chips + gap)
const SERP_ROW_H = 200; // vertical stride between wrapped rows

// Known node box dimensions (ToolNode is fixed-height; the others are stable
// enough to center against). Rows are CENTER-aligned on a lane so mixed-height
// nodes (a tall parallel group beside short tool/endpoint nodes) share one
// centerline — every handle then sits at the same cross-axis coordinate and the
// connectors are straight.
export const TOOL_NODE_H = 84;
const TOOL_NODE_W = 200;
const ENDPOINT_H = 48;
const ENDPOINT_W = 190;
const PARALLEL_W = 220;
// Estimated parallel-group height: header + one row per branch. Kept in sync
// with ParallelGroupNode's layout so the group centers on the row lane.
const parallelHeight = (branches: number) => 40 + Math.max(1, branches) * 33;

const EDGE_MARKER = {
  type: MarkerType.Arrow,
  width: 16,
  height: 16,
  color: "var(--muted-foreground)",
};

/** One tool inside a parallel group, as carried on a composite node's data. */
export interface ParallelBranch {
  toolName: string;
  label: string;
  config?: Record<string, unknown>;
  category: string;
  consumes?: IOPort[];
  produces?: IOPort[];
  valid: boolean;
}

/** Port type names of a produced/consumed contract, for typed edge chips. */
function portTypes(fs?: IOPort[]): string[] | undefined {
  if (!fs || fs.length === 0) return undefined;
  return fs.map((f) => f.type);
}

/** Union of IO ports (dedupe by type@side) — the group contract of a parallel set. */
function unionPorts(ports: IOPort[]): IOPort[] {
  const seen = new Set<string>();
  const out: IOPort[] = [];
  for (const p of ports) {
    const key = `${p.type}@${p.side ?? "source"}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(p);
  }
  return out;
}

/** A flow node that represents a step (a single tool or a parallel group). */
function isFlowNode(n: Node): boolean {
  return n.type === "tool" || n.type === "parallel";
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
      // A parallel group is ONE composite node listing its branches inside, with
      // a single input and single output — no fan-out/merge edges. It occupies a
      // single slot like any other station.
      const id = `tool-${toolCounter++}`;
      const branches: ParallelBranch[] = step.parallel.map((branch) => {
        const info = toolMap?.get(branch.tool);
        return {
          toolName: branch.tool,
          label: branch.label || info?.display_name || branch.tool,
          config: branch.config,
          category: info?.category || "pipeline",
          consumes: info?.consumes,
          produces: info?.produces,
          valid: toolMap ? !!info : true,
        };
      });

      nodes.push({
        id,
        type: "parallel",
        position: pos(primary, CENTER),
        data: {
          parallelGroup: true,
          stepIndex,
          branches,
          // Group IO contract = the union across branches.
          consumes: unionPorts(branches.flatMap((b) => b.consumes ?? [])),
          produces: unionPorts(branches.flatMap((b) => b.produces ?? [])),
          valid: branches.every((b) => b.valid),
        },
      });

      for (const prev of prevIds) {
        const prevNode = nodes.find((n) => n.id === prev);
        edges.push(makeEdge(prev, id, portTypes(prevNode?.data.produces as IOPort[] | undefined)));
      }

      prevIds = [id];
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
 * Reading-order wrap auto-layout: fold the chain into rows that wrap to fit
 * `cols` columns, every row flowing left→right and wrapping top-to-bottom (like
 * text), so Source ends up top-left and Sink bottom-right. Built by
 * repositioning the horizontal graph, so node identity + edges are unchanged;
 * only positions and the in/out handle sides (data.inPosition /
 * data.outPosition) are rewritten. Returns the geometry for the Source/Sink
 * endpoints so the editor can place them in the same flow.
 */
export function serpentineGraph(
  spec: FlowSpec,
  toolMap: Map<string, ToolInfo> | undefined,
  cols: number,
): { nodes: Node[]; edges: Edge[]; ends?: { source: EndpointGeom; sink: EndpointGeom } } {
  const base = stepsToGraph(spec, toolMap, "horizontal");
  const columns = Math.max(1, cols);
  const toolNodes = base.nodes.filter(isFlowNode);
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

  // Reading-order wrap: rows fill left→right and wrap top-to-bottom (like text).
  // Source is the first station (top-left); Sink is pinned to the bottom-right
  // of the last row. Rows are CENTER-aligned on a lane so a tall parallel group
  // shares its centerline with the short tool/endpoint nodes beside it — every
  // handle then lands at the same cross-axis coordinate and the in-row connector
  // is dead straight. Handle SIDES follow the chain: an edge crossing to another
  // row uses Bottom→Top (a clean vertical wrap), within a row Right→Left.
  const vertical = columns === 1;
  const N = colKeys.length;
  const rowOf = (slot: number) => Math.floor(slot / columns);
  const slotX = (slot: number) => (slot % columns) * SERP_COL_W;
  const laneCenterY = (row: number) => row * SERP_ROW_H + TOOL_NODE_H / 2;
  const laneCenterX = TOOL_NODE_W / 2;
  const nodeHeight = (n: Node) =>
    n.type === "parallel"
      ? parallelHeight((n.data.branches as unknown[] | undefined)?.length ?? 1)
      : TOOL_NODE_H;
  const nodeWidth = (n: Node) => (n.type === "parallel" ? PARALLEL_W : TOOL_NODE_W);

  // Sink placement: same row to the right when there's room, else the rightmost
  // column of a new row (bottom-right). Computed first because the last tool's
  // output side depends on whether the sink wrapped to a new row.
  const lastRow = rowOf(N);
  const lastCol = N % columns;
  const sinkRow = vertical ? N + 1 : lastCol < columns - 1 ? lastRow : lastRow + 1;
  const sinkCol = vertical ? 0 : lastCol < columns - 1 ? lastCol + 1 : columns - 1;

  colKeys.forEach((key, g) => {
    const slot = g + 1; // tools occupy slots 1..N (slot 0 is Source)
    const row = rowOf(slot);
    const inPosition = row > rowOf(slot - 1) ? Position.Top : Position.Left;
    const nextRow = slot < N ? rowOf(slot + 1) : sinkRow;
    const outPosition = nextRow > row ? Position.Bottom : Position.Right;
    for (const n of colMap.get(key)!) {
      n.position = vertical
        ? { x: laneCenterX - nodeWidth(n) / 2, y: row * SERP_ROW_H }
        : { x: slotX(slot), y: laneCenterY(row) - nodeHeight(n) / 2 };
      n.data.inPosition = inPosition;
      n.data.outPosition = outPosition;
    }
  });

  const source: EndpointGeom = {
    x: vertical ? laneCenterX - ENDPOINT_W / 2 : 0,
    y: vertical ? 0 : laneCenterY(0) - ENDPOINT_H / 2,
    handlePosition: rowOf(1) > 0 ? Position.Bottom : Position.Right,
  };

  const sink: EndpointGeom = {
    x: vertical ? laneCenterX - ENDPOINT_W / 2 : sinkCol * SERP_COL_W,
    y: vertical ? sinkRow * SERP_ROW_H : laneCenterY(sinkRow) - ENDPOINT_H / 2,
    handlePosition: sinkRow > lastRow ? Position.Top : Position.Left,
  };

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

  const flowNodes = nodes.filter(isFlowNode).sort((a, b) => primary(a) - primary(b));
  if (flowNodes.length === 0) return withBindings({ steps: [] });

  // A node becomes its step: a parallel group node emits a `parallel` step from
  // its branches; any other emits a plain step. (No position-grouping needed —
  // the parallel group is already a single node.)
  const toStep = (n: Node): FlowStep => {
    if (n.type === "parallel") {
      const branches = (n.data.branches as ParallelBranch[] | undefined) ?? [];
      return {
        tool: "",
        parallel: branches.map((b) => ({ tool: b.toolName, config: b.config, label: b.label })),
      };
    }
    return {
      tool: (n.data.toolName as string) || "",
      config: n.data.config as Record<string, unknown> | undefined,
      label: n.data.label as string | undefined,
    };
  };

  // Source-transform stage (single tools only) vs the main stage.
  const sourceTransforms = flowNodes.filter((n) => n.data.stage === "source-transform").map(toStep);
  const steps = flowNodes.filter((n) => n.data.stage !== "source-transform").map(toStep);

  const result: FlowSpec = { steps };
  if (sourceTransforms.length > 0) result.sourceTransforms = sourceTransforms;
  return withBindings(result);
}
