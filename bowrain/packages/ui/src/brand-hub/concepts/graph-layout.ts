// Deterministic layout for the concept knowledge graph (AD-021).
//
// The hierarchy relations (BROADER / NARROWER / PART_OF / HAS_PART) drive a
// dagre-like leveling: each edge implies a parent → child direction, levels are
// assigned by longest path from the roots, and nodes in a level are ordered to
// reduce crossings (a barycenter sweep against the level above). Concepts that
// take part in no hierarchy edge are "free": they settle near their relational
// neighbours, or — when the graph carries no hierarchy at all — flow into a
// deterministic grid. The result is stable (the same graph always lays out the
// same way), so React Flow only has to render it.
import type { GraphViz, GraphVizEdge, RelationType } from "../../types/brand-graph";

/** Relation types that impose a vertical hierarchy on the graph. */
export const HIERARCHY_RELATIONS: RelationType[] = ["BROADER", "NARROWER", "PART_OF", "HAS_PART"];

/**
 * Resolve a hierarchy edge to a (parent, child) pair — the parent sits a level
 * above the child. Non-hierarchy edges return null and never affect leveling.
 *
 *   BROADER  (s → t): s is broader than t, so s is the parent.
 *   NARROWER (s → t): s is narrower than t, so t is the parent.
 *   PART_OF  (s → t): s is part of t, so t (the whole) is the parent.
 *   HAS_PART (s → t): s has part t, so s (the whole) is the parent.
 */
export function hierarchyPair(edge: GraphVizEdge): { parent: string; child: string } | null {
  switch (edge.type) {
    case "BROADER":
    case "HAS_PART":
      return { parent: edge.source, child: edge.target };
    case "NARROWER":
    case "PART_OF":
      return { parent: edge.target, child: edge.source };
    default:
      return null;
  }
}

export interface LayoutOptions {
  /** Node footprint used for spacing (must match the rendered node box). */
  nodeWidth?: number;
  nodeHeight?: number;
  /** Gaps between node boxes within a row and between rows. */
  hGap?: number;
  vGap?: number;
  /** Columns used when the graph has no hierarchy and falls back to a grid. */
  gridColumns?: number;
}

export interface PositionedNode {
  id: string;
  x: number;
  y: number;
  level: number;
}

export interface GraphLayout {
  /** Position (top-left origin, normalised to start at 0,0) keyed by node id. */
  nodes: Record<string, PositionedNode>;
  width: number;
  height: number;
}

const DEFAULTS: Required<LayoutOptions> = {
  nodeWidth: 184,
  nodeHeight: 76,
  hGap: 44,
  vGap: 88,
  gridColumns: 4,
};

/**
 * Lay the graph out deterministically. Pure: no Date/Math.random, and equal
 * input always yields equal output.
 */
export function layoutConcepts(graph: GraphViz, options?: LayoutOptions): GraphLayout {
  const opts = { ...DEFAULTS, ...options };
  const ids = graph.nodes.map((n) => n.id);
  if (ids.length === 0) return { nodes: {}, width: 0, height: 0 };

  const labelOf = new Map(graph.nodes.map((n) => [n.id, n.label ?? n.id]));
  const idSet = new Set(ids);

  // ── Hierarchy graph ────────────────────────────────────────────────────────
  const childrenOf = new Map<string, Set<string>>();
  const parentsOf = new Map<string, Set<string>>();
  const inHierarchy = new Set<string>();
  for (const edge of graph.edges) {
    const pair = hierarchyPair(edge);
    if (!pair) continue;
    const { parent, child } = pair;
    if (!idSet.has(parent) || !idSet.has(child) || parent === child) continue;
    (childrenOf.get(parent) ?? childrenOf.set(parent, new Set()).get(parent)!).add(child);
    (parentsOf.get(child) ?? parentsOf.set(child, new Set()).get(child)!).add(parent);
    inHierarchy.add(parent);
    inHierarchy.add(child);
  }

  // ── Level assignment (longest path from roots, cycle-safe) ───────────────────
  const level = new Map<string, number>();
  for (const id of inHierarchy) level.set(id, 0);
  // Relax: a child must sit below its deepest parent. Bound the passes by the
  // participant count so a cyclic edge can never spin forever.
  for (let pass = 0; pass < inHierarchy.size; pass++) {
    let changed = false;
    for (const edge of graph.edges) {
      const pair = hierarchyPair(edge);
      if (!pair) continue;
      const { parent, child } = pair;
      if (!inHierarchy.has(parent) || !inHierarchy.has(child)) continue;
      const want = (level.get(parent) ?? 0) + 1;
      if (want > (level.get(child) ?? 0)) {
        level.set(child, want);
        changed = true;
      }
    }
    if (!changed) break;
  }

  // ── Free nodes (no hierarchy edge) ───────────────────────────────────────────
  // Neighbour map over ALL edges, so a relation-only node can drift toward the
  // concept it relates to.
  const neighbours = new Map<string, Set<string>>();
  for (const edge of graph.edges) {
    if (!idSet.has(edge.source) || !idSet.has(edge.target)) continue;
    (neighbours.get(edge.source) ?? neighbours.set(edge.source, new Set()).get(edge.source)!).add(
      edge.target,
    );
    (neighbours.get(edge.target) ?? neighbours.set(edge.target, new Set()).get(edge.target)!).add(
      edge.source,
    );
  }

  const free = ids.filter((id) => !inHierarchy.has(id));
  const maxHierLevel = inHierarchy.size > 0 ? Math.max(...level.values()) : -1;

  if (inHierarchy.size === 0) {
    // No hierarchy anywhere: deterministic grid in input order.
    free.forEach((id, i) => level.set(id, Math.floor(i / opts.gridColumns)));
  } else {
    // Settle each free node near its leveled neighbours; otherwise a trailing row.
    const trailing = maxHierLevel + 1;
    for (const id of free) {
      const leveledNeighbourLevels = [...(neighbours.get(id) ?? [])]
        .filter((n) => inHierarchy.has(n))
        .map((n) => level.get(n)!);
      if (leveledNeighbourLevels.length > 0) {
        const avg =
          leveledNeighbourLevels.reduce((a, b) => a + b, 0) / leveledNeighbourLevels.length;
        level.set(id, Math.round(avg) + 1);
      } else {
        level.set(id, trailing);
      }
    }
  }

  // ── Order within each level (barycenter against the level above) ─────────────
  const byLevel = new Map<number, string[]>();
  for (const id of ids) {
    const l = level.get(id) ?? 0;
    (byLevel.get(l) ?? byLevel.set(l, []).get(l)!).push(id);
  }
  const levels = [...byLevel.keys()].sort((a, b) => a - b);

  const orderIndex = new Map<string, number>();
  for (const l of levels) {
    const row = byLevel.get(l)!;
    const above = l > 0 ? byLevel.get(l - 1) : undefined;
    row.sort((a, b) => {
      const ba = barycenter(a, parentsOf, orderIndex, above);
      const bb = barycenter(b, parentsOf, orderIndex, above);
      if (ba !== bb) return ba - bb;
      const la = (labelOf.get(a) ?? a).toLowerCase();
      const lb = (labelOf.get(b) ?? b).toLowerCase();
      if (la !== lb) return la < lb ? -1 : 1;
      return a < b ? -1 : 1;
    });
    row.forEach((id, i) => orderIndex.set(id, i));
  }

  // ── Coordinates (rows centered to the widest row) ────────────────────────────
  const rowWidth = (count: number) =>
    count > 0 ? count * opts.nodeWidth + (count - 1) * opts.hGap : 0;
  const maxRowWidth = Math.max(0, ...levels.map((l) => rowWidth(byLevel.get(l)!.length)));

  const nodes: Record<string, PositionedNode> = {};
  for (const l of levels) {
    const row = byLevel.get(l)!;
    const rowOffset = (maxRowWidth - rowWidth(row.length)) / 2;
    row.forEach((id, i) => {
      nodes[id] = {
        id,
        x: rowOffset + i * (opts.nodeWidth + opts.hGap),
        y: l * (opts.nodeHeight + opts.vGap),
        level: l,
      };
    });
  }

  const maxLevel = levels[levels.length - 1] ?? 0;
  return {
    nodes,
    width: maxRowWidth,
    height: (maxLevel + 1) * opts.nodeHeight + maxLevel * opts.vGap,
  };
}

function barycenter(
  id: string,
  parentsOf: Map<string, Set<string>>,
  orderIndex: Map<string, number>,
  above?: string[],
): number {
  if (!above || above.length === 0) return orderIndex.get(id) ?? 0;
  const parents = [...(parentsOf.get(id) ?? [])]
    .map((p) => orderIndex.get(p))
    .filter((v): v is number => v !== undefined);
  if (parents.length === 0) return Number.MAX_SAFE_INTEGER; // unparented → trail the row
  return parents.reduce((a, b) => a + b, 0) / parents.length;
}
