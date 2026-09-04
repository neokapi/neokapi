// Adapter between the node/edge flow-definition model (FlowDefinitionInfo) used
// by the bowrain Wails backend's FlowStore and the steps-based FlowSpec consumed
// by FlowEditor.
//
// A flow is composition only and owns no I/O (AD-026): the persisted node graph
// contains tool nodes only — reader/writer are no longer nodes. Where content
// enters and leaves is a binding (`def.binding.source` / `def.binding.sink`),
// carried alongside the graph, not as nodes. FlowEditor works in FlowSpec
// (steps + source/sink). These two functions bridge the gap
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

import type {
  FlowSpec,
  FlowStep,
  FlowBinding,
  FlowDefinitionInfo,
  FlowNodeInfo,
  FlowEdgeInfo,
  ToolInfo,
} from "./types";

// Persisted-graph geometry. The node/edge FlowDefinition stores parallel groups
// as a fan-out: sibling nodes sharing the same chain-axis position, joined to
// the previous and next step by edges. specToDef lays the chain out along y.
// defToSpec reads the structure back from the edges, and from these positions
// only when a definition carries no edges. The editor never reads the
// positions for layout; it lays out from the spec.
const PERSIST_NODE_SIZE = 200;
const PERSIST_NODE_GAP = 60;
const PERSIST_CROSS = 200;
const PERSIST_BRANCH_GAP = 80;

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

/** A tool node as the step it runs. */
function nodeToStep(n: FlowNodeInfo): FlowStep {
  return {
    tool: n.name,
    ...(n.config && Object.keys(n.config).length > 0 ? { config: n.config } : {}),
    ...(n.label ? { label: n.label } : {}),
  };
}

/**
 * Layer the nodes by their edges: each node sits at the length of the longest
 * path from an entry node (one with no incoming edge), so a fan-out puts its
 * branches side by side in one layer and the merge target in the next. A node
 * on a cycle never becomes ready; it is appended as its own layer, in node
 * order, so no node is lost. Within a layer the persisted cross-axis position
 * (x) orders the branches.
 */
function layersByEdges(nodes: FlowNodeInfo[], edges: FlowEdgeInfo[]): FlowNodeInfo[][] {
  const ids = new Set(nodes.map((n) => n.id));
  const out = new Map<string, string[]>();
  const pending = new Map<string, number>(nodes.map((n) => [n.id, 0]));
  for (const e of edges) {
    if (!ids.has(e.source) || !ids.has(e.target)) continue;
    out.set(e.source, [...(out.get(e.source) ?? []), e.target]);
    pending.set(e.target, (pending.get(e.target) ?? 0) + 1);
  }

  const depth = new Map<string, number>();
  const queue = nodes.filter((n) => pending.get(n.id) === 0).map((n) => n.id);
  for (const id of queue) depth.set(id, 0);
  while (queue.length > 0) {
    const id = queue.shift()!;
    for (const next of out.get(id) ?? []) {
      depth.set(next, Math.max(depth.get(next) ?? 0, depth.get(id)! + 1));
      const left = pending.get(next)! - 1;
      pending.set(next, left);
      if (left === 0) queue.push(next);
    }
  }
  let tail = Math.max(-1, ...depth.values()) + 1;
  for (const n of nodes) {
    if (!depth.has(n.id)) depth.set(n.id, tail++);
  }

  const layers = new Map<number, FlowNodeInfo[]>();
  for (const n of nodes) {
    const d = depth.get(n.id)!;
    layers.set(d, [...(layers.get(d) ?? []), n]);
  }
  return [...layers.keys()]
    .sort((a, b) => a - b)
    .map((d) => [...layers.get(d)!].sort((a, b) => (a.position?.x ?? 0) - (b.position?.x ?? 0)));
}

/**
 * Layer the nodes by position along the persisted chain axis (y), for a
 * definition that carries no edges: nodes sharing a position are one layer.
 */
function layersByPosition(nodes: FlowNodeInfo[]): FlowNodeInfo[][] {
  const primary = (n: FlowNodeInfo) => n.position?.y ?? 0;
  const all = [...nodes].sort((a, b) => primary(a) - primary(b));
  const layers: FlowNodeInfo[][] = [];
  let i = 0;
  while (i < all.length) {
    const group = [all[i]];
    let j = i + 1;
    while (j < all.length && Math.abs(primary(all[j]) - primary(all[i])) < PERSIST_NODE_SIZE / 2) {
      group.push(all[j]);
      j++;
    }
    layers.push(group);
    i = j;
  }
  return layers;
}

/**
 * Convert a node/edge FlowDefinitionInfo into a steps-based FlowSpec.
 *
 * The persisted graph is tool nodes only (a flow owns no I/O); transformers
 * are ordinary ordered steps (AD-006). Order and fan-out come from the edges:
 * a layer of one node is a step, a layer of several is a `parallel` step. That
 * is what `core/flow` executes (its topological order follows the edges), so
 * a definition laid out along either axis reads the same. A definition with
 * no edges is ordered along the persisted chain axis (y), with nodes sharing
 * a position grouped in parallel. The definition's I/O binding is carried onto
 * `source`/`sink`.
 */
export function defToSpec(def: FlowDefinitionInfo): FlowSpec {
  const nodes = def.nodes ?? [];
  const edges = def.edges ?? [];
  const layers = edges.length > 0 ? layersByEdges(nodes, edges) : layersByPosition(nodes);
  const steps: FlowStep[] = layers.map((layer) =>
    layer.length === 1 ? nodeToStep(layer[0]) : { tool: "", parallel: layer.map(nodeToStep) },
  );

  const spec: FlowSpec = { steps };
  if (def.binding?.source) spec.source = def.binding.source;
  if (def.binding?.sink) spec.sink = def.binding.sink;
  if (def.description) spec.description = def.description;
  return spec;
}

/**
 * Convert a steps-based FlowSpec back into a node/edge FlowDefinitionInfo,
 * carrying over identity fields (id, name, source) from `base`.
 *
 * Emits the legacy tool-node persistence graph: steps along the primary axis,
 * with a parallel step expanded into sibling nodes sharing a primary-axis
 * position plus fan-out / merge edges. This is independent of the composite
 * parallel node the editor *renders* — the editor re-lays-out from the spec,
 * never from these positions.
 */
export function specToDef(
  spec: FlowSpec,
  base: Pick<FlowDefinitionInfo, "id" | "name" | "source"> & { description?: string },
  _tools?: ToolInfo[],
): FlowDefinitionInfo {
  const pos = (main: number, cross: number) => ({ x: cross, y: main });

  const nodeInfos: FlowNodeInfo[] = [];
  const edgeInfos: FlowEdgeInfo[] = [];
  let counter = 0;
  let primary = 0;
  let prevIds: string[] = [];

  const emit = (step: FlowStep, cross: number): string => {
    const id = `tool-${counter++}`;
    nodeInfos.push({
      id,
      type: "tool",
      name: step.tool,
      ...(step.label ? { label: step.label } : {}),
      ...(step.config && Object.keys(step.config).length > 0 ? { config: step.config } : {}),
      position: pos(primary, cross),
    });
    for (const p of prevIds) edgeInfos.push({ id: `e-${p}-${id}`, source: p, target: id });
    return id;
  };

  for (const step of spec.steps) {
    if (step.parallel && step.parallel.length > 0) {
      const n = step.parallel.length;
      const total = (n - 1) * (PERSIST_NODE_SIZE + PERSIST_BRANCH_GAP);
      const start = PERSIST_CROSS - total / 2;
      const ids = step.parallel.map((b, k) =>
        emit(b, start + k * (PERSIST_NODE_SIZE + PERSIST_BRANCH_GAP)),
      );
      prevIds = ids;
    } else {
      prevIds = [emit(step, PERSIST_CROSS)];
    }
    primary += PERSIST_NODE_SIZE + PERSIST_NODE_GAP;
  }

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
