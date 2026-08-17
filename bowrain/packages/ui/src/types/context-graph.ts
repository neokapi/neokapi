/**
 * The workspace context graph's read surface, mirroring
 * `GET /:ws/context/concepts/:cid/projects`.
 *
 * The graph is written on every push and answers "which projects use this
 * concept" in two hops out of one workspace-scoped vocabulary node. That is a
 * different question from the change-set blast radius, which asks what a draft
 * would do to stored content and has to read the text to know — so the two
 * answers have different shapes and neither stands in for the other.
 */

/** One of a concept's terms as one project actually uses it. */
export interface ConceptTermUse {
  term: string;
  term_locale?: string;
  blocks: number;
  occurrences: number;
}

/** One project's share of a concept's use. */
export interface ConceptProjectUse {
  project_id: string;
  project_name?: string;
  blocks: number;
  occurrences: number;
  collections?: string[];
  terms?: ConceptTermUse[];
}

/** One recorded use: where it sits, which term it used, and its wording. */
export interface ConceptUseRow {
  project_id: string;
  project_name?: string;
  stream?: string;
  collection?: string;
  document?: string;
  block_id?: string;
  locale?: string;
  term?: string;
  occurrences: number;
  text?: string;
}

/** The workspace's cross-project answer for one concept. */
export interface ConceptProjects {
  concept_id: string;
  /**
   * Where the answer came from: `graph` when the traversal answered it, `scan`
   * when the graph held no record of the concept and a bounded content walk
   * answered instead.
   */
  source: "graph" | "scan";
  /** The instant the answer was resolved at. */
  at: string;
  projects: ConceptProjectUse[];
  uses: ConceptUseRow[];
  blocks: number;
  occurrences: number;
  /** How many uses the answer holds before `uses` was paged. */
  uses_total: number;
  /**
   * True when the answer is a floor: a project absent from `projects` means
   * "not reached", not "does not use it".
   */
  partial?: boolean;
  partial_reason?: string;
  notes?: string[];
}

/** How a cross-project answer is narrowed. */
export interface ConceptProjectsQuery {
  /** Narrow to one project without changing which query answers. */
  project?: string;
  /** Resolve at an instant (RFC 3339); defaults to now. */
  at?: string;
  /** Resolve at a market, the validity tag a term is scoped by. */
  market?: string;
  /** Cap on the uses returned. */
  limit?: number;
}
