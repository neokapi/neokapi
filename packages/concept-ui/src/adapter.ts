// The data-source contract the concept UI consumes (Apache-2.0). A single small
// interface lets the SAME components run against very different backends:
//
//   • kapi-desktop drives it against a LOCAL SQLite termbase — the required
//     CORE only (concepts, terms, relations, validity/status), plus the
//     editable-core mutations.
//   • bowrain drives it against its REST API — the core PLUS the optional rich
//     reads (named markets, observations, comments, a revision timeline,
//     where-used) and its own governance layered around them.
//
// Every method whose data a given source lacks is simply omitted; components
// gate their rich sections and edit affordances on the resolved capabilities
// (see resolveCapabilities), so a minimal read-only source works unchanged.

import type {
  Concept,
  ConceptSummary,
  Comment,
  Market,
  Observation,
  Relation,
  RelationInput,
  TermRef,
  TermStatus,
  TermSource,
  TimelineEvent,
  Validity,
  WhereUsed,
  TermPatch,
} from "./types";

/** A value that may be returned synchronously or as a promise. */
export type Awaitable<T> = T | Promise<T>;

/** Filters for listConcepts. All fields are optional and AND-combined. */
export interface ConceptQuery {
  /** Free-text match over concept terms/definition. */
  text?: string;
  status?: TermStatus;
  domain?: string;
  source?: TermSource;
  /** Market name (named markets) or derived-market key. */
  market?: string;
  locale?: string;
  offset?: number;
  limit?: number;
}

/** The result of a concept list query. */
export interface ConceptListResult {
  concepts: ConceptSummary[];
  /** Total matching the query before any paging cap. */
  total: number;
}

/**
 * The contract the concept UI consumes. Implement the three core reads plus the
 * editable-core mutations you support; add the optional rich reads when your
 * backend has that data.
 */
export interface ConceptDataSource {
  /**
   * Explicit capability flags. When omitted, capabilities are DERIVED from which
   * optional methods this source implements (see deriveCapabilities). Set this
   * only to override the derivation (e.g. to advertise a method that is present
   * but should stay hidden).
   */
  capabilities?: Partial<ConceptCapabilities>;

  // ── Core reads (required) ──────────────────────────────────────────────────

  /** Browse/search concepts for the list surface. */
  listConcepts(query: ConceptQuery): Awaitable<ConceptListResult>;
  /** Load one full concept, or null if it no longer exists. */
  getConcept(id: string): Awaitable<Concept | null>;
  /** The concept's DIRECT (1-hop) relations, both incoming and outgoing. */
  getRelations(conceptId: string): Awaitable<Relation[]>;

  // ── Optional rich reads ────────────────────────────────────────────────────

  /**
   * Resolve a lightweight summary for a related concept (so the relations widget
   * can label a neighbour without a full getConcept). Falls back to the id when
   * absent.
   */
  getConceptSummary?(id: string): Awaitable<ConceptSummary | null>;
  /** Named markets the workspace defines (the explicit geography axis). */
  getMarkets?(): Awaitable<Market[]>;
  /** External evidence attached to a concept. */
  getObservations?(conceptId: string): Awaitable<Observation[]>;
  /** Threaded discussion on a concept. */
  getComments?(conceptId: string): Awaitable<Comment[]>;
  /** The concept's full evolution timeline (revisions, status changes, …). */
  getTimeline?(conceptId: string): Awaitable<TimelineEvent[]>;
  /** Where the concept is used across stored content. */
  getWhereUsed?(conceptId: string): Awaitable<WhereUsed | null>;

  // ── Editable-core mutations (optional, capability-gated) ───────────────────

  /** Add a direct relation from `conceptId` to `input.targetId`. */
  addRelation?(conceptId: string, input: RelationInput): Awaitable<Relation>;
  /** Remove a relation by id. */
  removeRelation?(relationId: string): Awaitable<void>;
  /** Patch a term's text/note/validity (and optionally status). */
  updateTerm?(conceptId: string, ref: TermRef, patch: TermPatch): Awaitable<void>;
  /** Transition a term's lifecycle status, optionally scoped by validity. */
  setTermStatus?(
    conceptId: string,
    ref: TermRef,
    status: TermStatus,
    validity?: Validity,
  ): Awaitable<void>;
}

/**
 * Resolved feature flags the components read to decide which rich sections and
 * edit affordances to show. Always concrete booleans (no `undefined`).
 */
export interface ConceptCapabilities {
  /** Term text/status can be edited. */
  editTerms: boolean;
  /** Relations can be added and removed. */
  editRelations: boolean;
  /** Named markets are available (richer geography panels). */
  markets: boolean;
  /** Observations are available. */
  observations: boolean;
  /** Comments are available. */
  comments: boolean;
  /** A full revision timeline is available. */
  timeline: boolean;
  /** Where-used data is available. */
  whereUsed: boolean;
}

/** Infer capabilities from which optional methods a source implements. */
export function deriveCapabilities(source: ConceptDataSource): ConceptCapabilities {
  // Computed access (not a static method reference) so capability probing does
  // not trip the unbound-method lint.
  const has = (key: keyof ConceptDataSource): boolean => typeof source[key] === "function";
  return {
    editTerms: has("updateTerm") || has("setTermStatus"),
    editRelations: has("addRelation") && has("removeRelation"),
    markets: has("getMarkets"),
    observations: has("getObservations"),
    comments: has("getComments"),
    timeline: has("getTimeline"),
    whereUsed: has("getWhereUsed"),
  };
}

/**
 * The capabilities a component should act on: method-presence derivation with
 * any explicit `source.capabilities` overrides applied on top.
 */
export function resolveCapabilities(source: ConceptDataSource): ConceptCapabilities {
  return { ...deriveCapabilities(source), ...source.capabilities };
}
