// Adapter between the node/edge flow-definition model (FlowDefinitionInfo) used
// by the bowrain Wails backend's FlowStore and the steps-based FlowSpec consumed
// by FlowEditor.
//
// The framework persists flows as a node graph (reader → tool(s) → writer, with
// edges and positions). FlowEditor works in FlowSpec (steps + sourceTransforms).
// These two functions bridge the gap by reusing the canonical graph<->steps
// conversion (stepsToGraph / graphToSteps), so the editor sees exactly the same
// model that kapi-desktop renders — there is only one conversion implementation.

import type { Node, Edge } from "@xyflow/react";
import type { FlowSpec, FlowDefinitionInfo, FlowNodeInfo, FlowEdgeInfo, ToolInfo } from "./types";
import { stepsToGraph, graphToSteps, type LayoutDirection } from "./conversion";

/**
 * Convert a node/edge FlowDefinitionInfo into a steps-based FlowSpec.
 *
 * Maps the definition's FlowNodeInfo[] onto @xyflow Node[] (carrying toolName,
 * stage and config on node.data, preserving node.type and position) and then
 * delegates to graphToSteps. Source-transform stage and parallel groups are
 * handled by graphToSteps via node.data.stage and node positions respectively.
 */
export function defToSpec(
  def: FlowDefinitionInfo,
  direction: LayoutDirection = "vertical",
): FlowSpec {
  const nodes: Node[] = (def.nodes ?? []).map((n: FlowNodeInfo) => ({
    id: n.id,
    type: n.type,
    position: { x: n.position?.x ?? 0, y: n.position?.y ?? 0 },
    data: {
      label: n.label || n.name,
      toolName: n.name,
      formatName: n.name === "auto" ? "auto" : n.name,
      config: n.config,
      stage: n.stage ?? "",
    },
  }));

  const spec = graphToSteps(nodes, direction);
  if (def.description) spec.description = def.description;
  return spec;
}

/**
 * Convert a steps-based FlowSpec back into a node/edge FlowDefinitionInfo,
 * carrying over identity fields (id, name, source) from `base`.
 *
 * Uses stepsToGraph to produce the canonical reader/tool/writer node layout and
 * edges, then maps the @xyflow Node[]/Edge[] onto FlowNodeInfo[]/FlowEdgeInfo[].
 * Source-transform tools keep their stage; parallel groups are preserved as the
 * fan-out node topology stepsToGraph emits.
 */
export function specToDef(
  spec: FlowSpec,
  base: Pick<FlowDefinitionInfo, "id" | "name" | "source"> & { description?: string },
  tools?: ToolInfo[],
  direction: LayoutDirection = "vertical",
): FlowDefinitionInfo {
  const toolMap = tools ? new Map(tools.map((t) => [t.name, t])) : undefined;
  const { nodes, edges } = stepsToGraph(spec, toolMap, direction);

  const nodeInfos: FlowNodeInfo[] = nodes.map((n) => {
    const type = (n.type ?? "tool") as FlowNodeInfo["type"];
    const isReaderOrWriter = type === "reader" || type === "writer";
    const name = isReaderOrWriter ? "auto" : (n.data.toolName as string) || n.id;
    const stage = (n.data.stage as string) || undefined;
    const config = n.data.config as Record<string, unknown> | undefined;
    return {
      id: n.id,
      type,
      name,
      label: (n.data.label as string) || undefined,
      ...(stage ? { stage: stage as FlowNodeInfo["stage"] } : {}),
      ...(config && Object.keys(config).length > 0 ? { config } : {}),
      position: { x: n.position.x, y: n.position.y },
    };
  });

  const edgeInfos: FlowEdgeInfo[] = edges.map((e: Edge) => ({
    id: e.id,
    source: e.source,
    target: e.target,
  }));

  return {
    id: base.id,
    name: base.name,
    description: spec.description ?? base.description ?? "",
    source: base.source,
    nodes: nodeInfos,
    edges: edgeInfos,
  };
}
