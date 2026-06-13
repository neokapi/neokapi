// Visual vocabulary for the concept knowledge graph (AD-021). Pure, deterministic
// helpers shared by the React Flow canvas (GraphPanel), the legend, and the
// graph view. Kept free of React/React-Flow imports so the colour + relation
// rules can be unit-tested in isolation.
import type { GraphViz, GraphVizEdge, RelationType, TermStatus } from "../../types/brand-graph";
import { hierarchyPair } from "./graph-layout";

// ── Node status tone ─────────────────────────────────────────────────────────
//
// Nodes are coloured by their dominant term status, reusing the hub's status
// vocabulary so the graph legend matches the term badges everywhere else
// (shell/atoms.tsx). A node with no terms (status "") reads as muted.

export type StatusTone = "preferred" | "approved" | "admitted" | "neutral" | "forbidden";

const STATUS_TONE: Record<TermStatus, StatusTone> = {
  preferred: "preferred",
  approved: "approved",
  admitted: "admitted",
  proposed: "neutral",
  deprecated: "neutral",
  forbidden: "forbidden",
};

/** The tone bucket for a (possibly empty) dominant status. */
export function statusTone(status?: TermStatus | ""): StatusTone {
  if (!status) return "neutral";
  return STATUS_TONE[status];
}

/** A CSS colour expression (theme variable) for a status tone. */
export function statusColorVar(status?: TermStatus | ""): string {
  switch (statusTone(status)) {
    case "preferred":
      return "var(--color-success)";
    case "approved":
      return "var(--color-primary)";
    case "admitted":
      return "var(--color-warning)";
    case "forbidden":
      return "var(--color-destructive)";
    default:
      return "var(--color-muted-foreground)";
  }
}

/** Whether the dominant status should render the label struck-through (retired). */
export function isRetiredStatus(status?: TermStatus | ""): boolean {
  return status === "deprecated" || status === "forbidden";
}

// ── Relation edge styling ────────────────────────────────────────────────────
//
// Each relation family gets its own line treatment so the structure of the
// graph is legible at a glance: hierarchy is a plain directed line, succession
// (governed) is an emphasised primary line, guidance a dashed primary line,
// equivalence a symmetric dashed accent line, brand stance (competitor) a dashed
// destructive line, and "related" a faint dashed connector.

export type RelationFamily =
  | "hierarchy"
  | "succession"
  | "guidance"
  | "equivalence"
  | "competitor"
  | "related";

const RELATION_FAMILY: Record<RelationType, RelationFamily> = {
  BROADER: "hierarchy",
  NARROWER: "hierarchy",
  PART_OF: "hierarchy",
  HAS_PART: "hierarchy",
  RELATED: "related",
  REPLACED_BY: "succession",
  USE_INSTEAD: "guidance",
  EXACT_MATCH: "equivalence",
  CLOSE_MATCH: "equivalence",
  COMPETITOR: "competitor",
};

export function relationFamily(type: RelationType): RelationFamily {
  return RELATION_FAMILY[type];
}

export interface EdgeStyle {
  family: RelationFamily;
  /** CSS colour expression for the stroke. */
  color: string;
  width: number;
  /** SVG dasharray, or undefined for a solid line. */
  dash?: string;
  /** Flow the dashes (React Flow `animated`) — reserved for governed succession. */
  animated: boolean;
  /** Whether the edge carries a direction (arrowhead at the target). */
  directed: boolean;
}

const FAMILY_STYLE: Record<RelationFamily, Omit<EdgeStyle, "family">> = {
  hierarchy: {
    color: "var(--color-muted-foreground)",
    width: 1.5,
    animated: false,
    directed: true,
  },
  succession: {
    color: "var(--color-primary)",
    width: 2,
    animated: true,
    directed: true,
  },
  guidance: {
    color: "var(--color-primary)",
    width: 1.5,
    dash: "5 4",
    animated: false,
    directed: true,
  },
  equivalence: {
    color: "var(--color-accent-foreground)",
    width: 1.5,
    dash: "2 4",
    animated: false,
    directed: false,
  },
  competitor: {
    color: "var(--color-destructive)",
    width: 1.5,
    dash: "6 4",
    animated: false,
    directed: true,
  },
  related: {
    color: "var(--color-muted-foreground)",
    width: 1.25,
    dash: "1 5",
    animated: false,
    directed: false,
  },
};

/** The full line treatment for a relation type. */
export function relationEdgeStyle(type: RelationType): EdgeStyle {
  const family = relationFamily(type);
  return { family, ...FAMILY_STYLE[family] };
}

// ── Edge orientation ─────────────────────────────────────────────────────────

/**
 * The drawing direction of an edge. Hierarchy edges are oriented parent → child
 * (so the arrow flows down the level layout, matching graph-layout.ts); every
 * other relation keeps its stored source → target direction.
 */
export function orientEdge(edge: GraphVizEdge): { source: string; target: string } {
  const pair = hierarchyPair(edge);
  if (pair) return { source: pair.parent, target: pair.child };
  return { source: edge.source, target: edge.target };
}

// ── Neighbourhood (focus highlight) ──────────────────────────────────────────

export interface Neighbourhood {
  nodeIds: Set<string>;
  edgeIds: Set<string>;
}

/**
 * The focus concept plus everything one hop away: the node ids to keep
 * emphasised and the edge ids that touch the focus. An unknown/empty focus
 * yields empty sets (caller treats that as "nothing dimmed").
 */
export function neighbourhood(graph: GraphViz, focusId?: string): Neighbourhood {
  const nodeIds = new Set<string>();
  const edgeIds = new Set<string>();
  if (!focusId) return { nodeIds, edgeIds };
  const known = new Set(graph.nodes.map((n) => n.id));
  if (!known.has(focusId)) return { nodeIds, edgeIds };
  nodeIds.add(focusId);
  for (const e of graph.edges) {
    if (e.source === focusId || e.target === focusId) {
      edgeIds.add(e.id);
      nodeIds.add(e.source);
      nodeIds.add(e.target);
    }
  }
  return { nodeIds, edgeIds };
}

// ── Relation neighbours of one concept (for the side panel) ──────────────────

export interface RelationNeighbour {
  edgeId: string;
  /** The concept on the other end. */
  otherId: string;
  type: RelationType;
  /** True when the stored edge points away from the subject concept. */
  outgoing: boolean;
}

/** Every relation that touches `conceptId`, with the neighbour on the far end. */
export function relationNeighbours(graph: GraphViz, conceptId: string): RelationNeighbour[] {
  const out: RelationNeighbour[] = [];
  for (const e of graph.edges) {
    if (e.source === conceptId) {
      out.push({ edgeId: e.id, otherId: e.target, type: e.type, outgoing: true });
    } else if (e.target === conceptId) {
      out.push({ edgeId: e.id, otherId: e.source, type: e.type, outgoing: false });
    }
  }
  return out;
}
