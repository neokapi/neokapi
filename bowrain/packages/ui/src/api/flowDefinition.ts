import type { FlowDefinitionInfo } from "../types/api";

/**
 * A flow definition as it arrives from the server or the desktop backend,
 * read so that `nodes` and `edges` are always lists. A server older than the
 * one that writes empty lists sends `null` for a flow with no edges (every
 * one-step built-in), and a stored definition may omit either field; the
 * editor and the flow list iterate both. Every adapter passes a definition it
 * receives through here, so no consumer has to guard on its own.
 */
export function flowDefinitionFromWire(raw: unknown): FlowDefinitionInfo {
  const def = (raw ?? {}) as Partial<FlowDefinitionInfo> & Record<string, unknown>;
  return {
    ...def,
    nodes: Array.isArray(def.nodes) ? def.nodes : [],
    edges: Array.isArray(def.edges) ? def.edges : [],
  } as FlowDefinitionInfo;
}

/** flowDefinitionFromWire over a list; a missing list reads as empty. */
export function flowDefinitionsFromWire(raw: unknown): FlowDefinitionInfo[] {
  return Array.isArray(raw) ? raw.map(flowDefinitionFromWire) : [];
}
