// The context graph as the lens draws it: what a project holds (its content
// and the editions of it), the point that content sits at, and what the
// project knows there (a voice, the terms, the content memory) with the
// record of how it came to know it. Every node and edge is read from the
// engine's own answers; nothing here is modelled from the recipe.

export type NodeKind =
  | "project"
  | "collection"
  | "source"
  | "edition"
  | "point"
  | "voice"
  | "terms"
  | "concept"
  | "memory"
  | "record";

export interface Coverage {
  draft: number;
  established: number;
  translated: number;
}

export interface GraphNode {
  id: string;
  kind: NodeKind;
  label: string;
  /** A second line under the label. */
  sub?: string;
  /** A concept's status (preferred, forbidden, deprecated, admitted) or an edition's ship state. */
  status?: string;
  /** Entries, blocks, operations: whatever the node counts. */
  count?: number;
  /** An edition's coverage, in percent. */
  pct?: Coverage;
  /** Fields the detail panel lists, in order. */
  detail?: [string, string][];
}

export type EdgeKind =
  | "in"
  | "edition"
  | "at"
  | "governs"
  | "applies"
  | "has"
  | "uses"
  | "recalls"
  | "records";

export interface GraphEdge {
  id: string;
  from: string;
  to: string;
  kind: EdgeKind;
  /** A short label drawn on the edge (a count, a locale). */
  label?: string;
  /** A count the stroke width follows. */
  weight?: number;
}

export interface GraphModel {
  nodes: GraphNode[];
  edges: GraphEdge[];
  /** The context store's revision, when the engine reported one. */
  revision?: number;
  /** The engine has read no content yet, so term use counts are empty. */
  stale?: boolean;
  /** One line on what the graph holds. */
  summary: string;
}

/** What a chapter asks the lens to bring forward; everything else is dimmed. */
export type Focus =
  | "all"
  | "content"
  | "point"
  | "context"
  | "terms"
  | "memory"
  | "editions"
  | "record";

/** Runs a line in the lab shell and returns what it printed. */
export type LineRunner = (line: string) => Promise<{ code: number; out: string }>;
