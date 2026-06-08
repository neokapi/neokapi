// Adapter between the node/edge flow-definition model (FlowDefinitionInfo) used
// by the bowrain Wails backend's FlowStore and the steps-based FlowSpec consumed
// by FlowEditor.
//
// A flow is composition only and owns no I/O (AD-026): the persisted node graph
// contains tool nodes only — reader/writer are no longer nodes. Where content
// enters and leaves is a binding (`def.binding.source` / `def.binding.sink`),
// carried alongside the graph, not as nodes. FlowEditor works in FlowSpec
// (steps + sourceTransforms + source/sink). These two functions bridge the gap
// by reusing the canonical graph<->steps conversion (stepsToGraph /
// graphToSteps), so the editor sees exactly the same model that kapi-desktop
// renders — there is only one conversion implementation.
//
// Wire format (AD YAML / Go contract): bindings are *string locators*, not
// objects. The steps spec uses flat `source` / `sink` string fields; the
// FlowDefinition nests them under `binding: { source?: string; sink?: string }`
// (nested because `source` on the definition is already the provenance field).
// The editor parses these strings into the internal `FlowBinding` object for the
// endpoint pickers via `parseBinding`, and serializes back via `formatBinding`.

import type { Node, Edge } from "@xyflow/react";
import type {
  FlowSpec,
  FlowBinding,
  FlowDefinitionInfo,
  FlowNodeInfo,
  FlowEdgeInfo,
  ToolInfo,
} from "./types";
import { stepsToGraph, graphToSteps, type LayoutDirection } from "./conversion";

/** Interchange format ids that serialize as a bare locator (e.g. `source: xliff`). */
const INTERCHANGE_FORMATS = new Set(["xliff", "po", "tmx", "tbx"]);

/**
 * Parse a wire-format string locator into the internal `FlowBinding` object.
 *
 * Mapping (AD-026 / Go contract):
 *  - `file` | `store` | `none`      → that kind
 *  - `xliff` | `po` | `tmx` | `tbx` → `{ kind: "interchange", format }`
 *  - omitted / empty / unknown      → `{ kind: "file" }` (the default)
 */
export function parseBinding(locator: string | undefined | null): FlowBinding {
  if (!locator) return { kind: "file" };
  const value = locator.trim().toLowerCase();
  if (value === "store") return { kind: "store" };
  if (value === "none") return { kind: "none" };
  if (value === "file") return { kind: "file" };
  if (INTERCHANGE_FORMATS.has(value)) return { kind: "interchange", format: value };
  // Unknown locators fall back to the file default rather than throwing.
  return { kind: "file" };
}

/**
 * Serialize an internal `FlowBinding` into a wire-format string locator.
 *
 * Returns `undefined` for the file default (omitted = file on both ends), so
 * callers can drop the field entirely. Interchange bindings serialize as their
 * `format` id (e.g. `xliff`); a malformed interchange binding with no format
 * degrades to the file default.
 */
export function formatBinding(binding: FlowBinding | undefined | null): string | undefined {
  if (!binding) return undefined;
  switch (binding.kind) {
    case "file":
      return undefined; // omitted = file default
    case "store":
      return "store";
    case "none":
      return "none";
    case "interchange":
      return binding.format && INTERCHANGE_FORMATS.has(binding.format) ? binding.format : undefined;
  }
}

/**
 * Convert a node/edge FlowDefinitionInfo into a steps-based FlowSpec.
 *
 * Maps the definition's FlowNodeInfo[] (tool nodes only) onto @xyflow Node[]
 * (carrying toolName, stage and config on node.data, preserving node.type and
 * position) and then delegates to graphToSteps. The definition's I/O binding is
 * carried through onto `spec.source` / `spec.sink`. Source-transform stage and
 * parallel groups are handled by graphToSteps via node.data.stage and node
 * positions respectively.
 */
export function defToSpec(
  def: FlowDefinitionInfo,
  direction: LayoutDirection = "vertical",
): FlowSpec {
  const nodes: Node[] = (def.nodes ?? []).map((n: FlowNodeInfo) => {
    if (n.type === "parallel") {
      // A parallel group node carries its branches; graphToSteps reads them back
      // into a `parallel` step. (Category/validity are re-derived by the editor
      // when it rebuilds the graph from the spec.)
      return {
        id: n.id,
        type: "parallel",
        position: { x: n.position?.x ?? 0, y: n.position?.y ?? 0 },
        data: {
          parallelGroup: true,
          branches: (n.branches ?? []).map((b) => ({
            toolName: b.name,
            label: b.label ?? b.name,
            config: b.config,
            category: "pipeline",
            valid: true,
          })),
        },
      };
    }
    return {
      id: n.id,
      type: n.type,
      position: { x: n.position?.x ?? 0, y: n.position?.y ?? 0 },
      data: {
        label: n.label || n.name,
        toolName: n.name,
        config: n.config,
        stage: n.stage ?? "",
      },
    };
  });

  const spec = graphToSteps(nodes, direction, def.binding);
  if (def.description) spec.description = def.description;
  return spec;
}

/**
 * Convert a steps-based FlowSpec back into a node/edge FlowDefinitionInfo,
 * carrying over identity fields (id, name, source) from `base`.
 *
 * Uses stepsToGraph to produce the canonical tool-node layout and edges (no
 * reader/writer nodes — a flow owns no I/O), then maps the @xyflow
 * Node[]/Edge[] onto FlowNodeInfo[]/FlowEdgeInfo[]. The spec's I/O binding
 * (`source` / `sink`) is carried onto `def.binding`. Source-transform tools keep
 * their stage; parallel groups are preserved as the fan-out node topology
 * stepsToGraph emits.
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
    if (n.type === "parallel") {
      const branches =
        (n.data.branches as Array<{
          toolName: string;
          label?: string;
          config?: Record<string, unknown>;
        }>) ?? [];
      return {
        id: n.id,
        type: "parallel",
        name: "",
        branches: branches.map((b) => ({
          name: b.toolName,
          ...(b.label ? { label: b.label } : {}),
          ...(b.config && Object.keys(b.config).length > 0 ? { config: b.config } : {}),
        })),
        position: { x: n.position.x, y: n.position.y },
      };
    }
    const stage = (n.data.stage as string) || undefined;
    const config = n.data.config as Record<string, unknown> | undefined;
    return {
      id: n.id,
      type: "tool",
      name: (n.data.toolName as string) || n.id,
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

  const def: FlowDefinitionInfo = {
    id: base.id,
    name: base.name,
    description: spec.description ?? base.description ?? "",
    source: base.source,
    nodes: nodeInfos,
    edges: edgeInfos,
  };

  if (spec.source || spec.sink) {
    def.binding = {
      ...(spec.source ? { source: spec.source } : {}),
      ...(spec.sink ? { sink: spec.sink } : {}),
    };
  }

  return def;
}
