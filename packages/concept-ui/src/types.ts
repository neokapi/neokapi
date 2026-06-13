// Framework-level concept model for the concept UI (Apache-2.0). These are the
// camelCase view types the components consume — deliberately decoupled from any
// platform's wire DTOs. A data source (see ./adapter) maps its own storage
// (a local SQLite termbase, a REST API, …) onto these shapes.
//
// The model has a small required CORE (concepts, terms, relations, validity,
// status) that any source must provide, and OPTIONAL rich types (named markets,
// observations, comments, a revision timeline) that only platforms with that
// data supply. Components render the rich parts only when the data is present
// and degrade gracefully to the core otherwise.

// ── Term status & source ─────────────────────────────────────────────────────

/** Term lifecycle status (mirrors core/model.TermStatus, SKOS-aligned). */
export type TermStatus =
  | "proposed"
  | "approved"
  | "preferred"
  | "admitted"
  | "deprecated"
  | "forbidden";

/** All term statuses, ordered most-blessed → most-restricted for menus. */
export const TERM_STATUSES: readonly TermStatus[] = [
  "preferred",
  "approved",
  "admitted",
  "proposed",
  "deprecated",
  "forbidden",
];

/** Where a concept originates (mirrors termbase.TermSource). */
export type TermSource = "terminology" | "brand_vocabulary";

// ── Relations (SKOS labels) ──────────────────────────────────────────────────

/** The SKOS-aligned relation vocabulary used between concepts (core/graph). */
export type RelationType =
  | "BROADER"
  | "NARROWER"
  | "PART_OF"
  | "HAS_PART"
  | "RELATED"
  | "REPLACED_BY"
  | "USE_INSTEAD"
  | "EXACT_MATCH"
  | "CLOSE_MATCH"
  | "COMPETITOR";

/** All relation types, in canonical menu/group order. */
export const RELATION_TYPES: readonly RelationType[] = [
  "BROADER",
  "NARROWER",
  "PART_OF",
  "HAS_PART",
  "RELATED",
  "REPLACED_BY",
  "USE_INSTEAD",
  "EXACT_MATCH",
  "CLOSE_MATCH",
  "COMPETITOR",
];

// ── Validity (temporal + tag scoping) ────────────────────────────────────────

/**
 * Temporal + tag scoping on a term or relation (mirrors core/graph.Validity).
 * An absent validity means "always valid, everywhere". `tags` carries market
 * vocabulary, e.g. `{ market: "dach" }` — the framework-only geography axis is
 * derived from these when a source has no named markets.
 */
export interface Validity {
  /** ISO-8601 instant the term/relation becomes valid (inclusive). */
  validFrom?: string;
  /** ISO-8601 instant the term/relation stops being valid (exclusive). */
  validTo?: string;
  /** Free-form scoping tags, e.g. `{ market: "dach", channel: "web" }`. */
  tags?: Record<string, string>;
}

// ── Term & concept (core) ────────────────────────────────────────────────────

/** One term within a concept — what the concept is called in a locale. */
export interface Term {
  text: string;
  locale: string;
  status: TermStatus;
  partOfSpeech?: string;
  gender?: string;
  note?: string;
  /** When/where this term applies (constraints axis + derived geography). */
  validity?: Validity;
}

/** The reduced concept shape used in lists and as a relation endpoint. */
export interface ConceptSummary {
  id: string;
  domain?: string;
  definition?: string;
  source?: TermSource;
  terms: Term[];
}

/** A full concept — the language-neutral unit of brand language. */
export interface Concept extends ConceptSummary {
  properties?: Record<string, string>;
  /** ISO-8601 creation instant, when the source tracks it. */
  createdAt?: string;
  /** ISO-8601 last-update instant, when the source tracks it. */
  updatedAt?: string;
}

/** A typed edge between two concepts (direct, 1-hop). */
export interface Relation {
  id: string;
  sourceId: string;
  targetId: string;
  type: RelationType;
  note?: string;
  /** When/where the relation holds. */
  validity?: Validity;
}

// ── Optional rich types ──────────────────────────────────────────────────────

/**
 * A named market: a label plus the locales it covers. Platforms supply these
 * via the adapter; the framework-only path derives anonymous markets from term
 * validity tags instead (see deriveMarketsFromTerms).
 */
export interface Market {
  /** Stable id when the source tracks one; optional for derived markets. */
  id?: string;
  name: string;
  description?: string;
  locales: string[];
}

/** The kind of external evidence an observation records. */
export type ObservationKind =
  | "competitor"
  | "customer"
  | "style_guide"
  | "regulatory"
  | "web"
  | "internal";

/** All observation kinds, ordered for menus. */
export const OBSERVATION_KINDS: readonly ObservationKind[] = [
  "competitor",
  "customer",
  "style_guide",
  "regulatory",
  "web",
  "internal",
];

/** External evidence attached to a concept (optional, platform-supplied). */
export interface Observation {
  id: string;
  kind: ObservationKind;
  quote: string;
  source: string;
  url?: string;
  locale?: string;
  market?: string;
  note?: string;
  actor?: string;
  /** ISO-8601 instant the observation was recorded. */
  at: string;
}

/** A threaded discussion entry on a concept (optional, platform-supplied). */
export interface Comment {
  id: string;
  /** Empty = top-level; set = a reply. */
  parentId?: string;
  body: string;
  author: string;
  /** ISO-8601 instant the comment was posted. */
  at: string;
  resolved?: boolean;
}

/** What produced a timeline event. */
export type TimelineKind =
  | "create"
  | "revision"
  | "status"
  | "relation"
  | "observation"
  | "comment"
  | "changeset";

/**
 * One event on a concept's evolution timeline. The core path can synthesise a
 * minimal timeline from createdAt/updatedAt + validity windows; a platform with
 * a revision log supplies the full history via the adapter.
 */
export interface TimelineEvent {
  id?: string;
  kind: TimelineKind;
  /** ISO-8601 instant the event occurred. */
  at: string;
  actor?: string;
  /** One-line headline. */
  summary: string;
  /** Optional opaque reference (e.g. a revision number or change-set id). */
  ref?: string;
  /** Optional longer body (e.g. a comment's text). */
  detail?: string;
}

/**
 * The footprint of a concept over stored content (optional, platform-supplied
 * via getWhereUsed). Deliberately shallow — platforms layer richer drill-downs
 * around it.
 */
export interface WhereUsed {
  conceptId: string;
  blocks: number;
  occurrences: number;
  words: number;
}

// ── Mutation inputs (editable core) ──────────────────────────────────────────

/** Identifies a term within its concept for updates. */
export interface TermRef {
  locale: string;
  text: string;
}

/** A partial term update. Only the provided fields change. */
export interface TermPatch {
  text?: string;
  status?: TermStatus;
  note?: string;
  validity?: Validity;
}

/** The fields needed to create a relation from the subject concept. */
export interface RelationInput {
  targetId: string;
  type: RelationType;
  note?: string;
  validity?: Validity;
}
