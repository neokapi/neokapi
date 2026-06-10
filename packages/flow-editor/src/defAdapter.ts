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
// as the legacy fan-out (sibling nodes sharing the same primary-axis position),
// independent of how the editor *renders* a flow (which uses the composite
// parallel node). The persisted graph uses a fixed vertical axis (y is the chain
// axis); defToSpec reconstructs groups along it. The editor never reads these
// positions for layout — it re-lays-out from the spec.
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

/**
 * Convert a node/edge FlowDefinitionInfo into a steps-based FlowSpec.
 *
 * The persisted graph is tool nodes only (a flow owns no I/O); transformers
 * are ordinary ordered steps (AD-006). Sibling nodes sharing a primary-axis
 * position are reconstructed into a `parallel` step. The definition's I/O
 * binding is carried onto `source`/`sink`. The persisted graph's chain axis
 * is y.
 */
export function defToSpec(def: FlowDefinitionInfo): FlowSpec {
  const primary = (n: FlowNodeInfo) => n.position?.y ?? 0;

  const toStep = (n: FlowNodeInfo): FlowStep => ({
    tool: n.name,
    ...(n.config && Object.keys(n.config).length > 0 ? { config: n.config } : {}),
    ...(n.label ? { label: n.label } : {}),
  });

  const all = [...(def.nodes ?? [])].sort((a, b) => primary(a) - primary(b));

  // Group nodes sharing a primary-axis position into a parallel step.
  const steps: FlowStep[] = [];
  let i = 0;
  while (i < all.length) {
    const group = [all[i]];
    let j = i + 1;
    while (j < all.length && Math.abs(primary(all[j]) - primary(all[i])) < PERSIST_NODE_SIZE / 2) {
      group.push(all[j]);
      j++;
    }
    steps.push(group.length === 1 ? toStep(group[0]) : { tool: "", parallel: group.map(toStep) });
    i = j;
  }

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
