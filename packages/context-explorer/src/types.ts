// The answers the explorer renders (Apache-2.0). Each shape mirrors what the
// engine already produces, so an adapter maps fields rather than re-modelling:
//
//   GovernanceAnswer  ← the by-location resolution (host.ContextAnswer) at kapi,
//                       the profile aggregation (GET /:ws/profiles) at Bowrain
//   SearchAnswer      ← host.ContextSearchResult / context_search
//   RelationsAnswer   ← core/occurrence + the context graph's Uses queries
//
// Notes are carried on every answer because a retrieval result must never be
// ambiguous between "no answer here" and "nowhere to look": the staleness note
// the engine stamps when the graph moved under the reader rides this field.

import type { ScopeTuple } from "./scope";

/** How much of the graph an answer could have come from. */
export type AnswerScope = "project" | "profile" | "workspace";

/** The point a question resolved to. */
export interface ContextPoint {
  path?: string;
  profile?: string;
  channel?: string;
  collection?: string;
  /** The recipe key or binding the resolution came from. */
  ref?: string;
  /** True when nothing finer than the project default governs here. */
  default: boolean;
}

/** The voice profile in force at a point. */
export interface ContextVoice {
  name: string;
  /** Where it was loaded from — a path, `pack:<name>` or `store:<name>`. */
  source?: string;
  /** The recipe key that bound it. */
  field?: string;
  guide?: string;
}

/** A term decision that applies at the point. */
export interface TermHit {
  concept_id: string;
  term: string;
  locale?: string;
  status?: string;
  definition?: string;
  domain?: string;
  replacement?: string;
  discouraged: boolean;
  valid_from?: string;
  valid_to?: string;
  uses?: number;
  use_blocks?: number;
  top_uses?: TermUse[];
}

/** One place a term is used, and the point that place sits at. */
export interface TermUse {
  document?: string;
  block_id?: string;
  locale?: string;
  snippet?: string;
  /** The coordinate the document resolves to, written `profile/channel`. */
  point?: string;
}

/** Wording the project has already approved. */
export interface PrecedentHit {
  entry_id: string;
  text: string;
  locale?: string;
  note?: string;
  discouraged?: string[];
}

/** A governance profile with a validity window, and where that window stands. */
export interface ProfileWindow {
  name: string;
  valid_from?: string;
  valid_to?: string;
  /** "active" | "upcoming" | "expired". */
  state: string;
}

/** A rule bound at the point, with its window. */
export interface RuleBinding {
  id: string;
  label: string;
  /** The gate it runs in — voice, terms, dnt, placeholder, custom. */
  gate?: string;
  severity?: string;
  valid_from?: string;
  valid_to?: string;
  state?: string;
}

/** What governs at one point. */
export interface GovernanceAnswer {
  point: ContextPoint;
  scope: AnswerScope;
  voice?: ContextVoice;
  terms: TermHit[];
  /** Total terms scoped here when `terms` is capped. */
  terms_total?: number;
  profiles: ProfileWindow[];
  rules?: RuleBinding[];
  /** Redaction clearance in force here, when the surface knows it. */
  clearance?: string;
  notes: string[];
}

/** One collection sitting at or under the point. */
export interface CollectionAtPoint {
  id: string;
  name: string;
  project?: string;
  stream?: string;
  /** The coordinate it binds to, written `profile/channel`. */
  point?: string;
  items?: number;
}

/** One content item at the point. */
export interface ContentItem {
  id: string;
  /** The document or unit this item belongs to. */
  document?: string;
  collection?: string;
  locale?: string;
  text?: string;
  /** "governed" | "ai-shippable" | "pending", when the surface derives it. */
  ship_state?: string;
  /** True when the decision that blessed this item cited a superseded source. */
  stale?: boolean;
}

/** Per-locale coverage at the point. */
export interface LocaleCoverage {
  locale: string;
  units: number;
  covered: number;
  /** "governed" | "ai-shippable" | "pending". */
  ship_state?: string;
  stale?: number;
}

/** What lives at one point. */
export interface ContentAnswer {
  point: ContextPoint;
  scope: AnswerScope;
  collections: CollectionAtPoint[];
  items: ContentItem[];
  coverage: LocaleCoverage[];
  /** Total items at the point when `items` is capped. */
  items_total?: number;
  notes: string[];
}

/** What a relations question is about. */
export interface RelationSubject {
  kind: "concept" | "block" | "collection";
  id: string;
  label?: string;
}

/** One occurrence of a concept in content. */
export interface OccurrenceRow {
  concept_id: string;
  term: string;
  term_locale?: string;
  block_id?: string;
  document?: string;
  collection?: string;
  locale: string;
  matched: string;
  snippet: string;
  /** The project the occurrence sits in; absent at a project-reach answer. */
  project?: string;
  stream?: string;
}

/** One of a concept's terms as one project uses it, and how it stands there. */
export interface ProjectTermUse {
  term: string;
  locale?: string;
  status?: string;
  discouraged?: boolean;
  occurrences?: number;
}

/**
 * A project's share of a concept's use.
 *
 * Naming the projects is only half the answer. "Is it discouraged here" is a
 * question about the concept AT A COORDINATE — the same word can be admitted on
 * one surface and discouraged on another — so a project's row carries how the
 * concept stands in that project rather than borrowing a workspace-global flag
 * that is the same wherever the reader is standing.
 */
export interface ProjectUse {
  project: string;
  project_name?: string;
  stream?: string;
  blocks?: number;
  occurrences?: number;
  collections?: string[];
  /** The lifecycle status the project's own usage gives the concept. */
  status?: string;
  /** True when the project reached for a spelling a writer must stop using. */
  discouraged?: boolean;
  /** What to say instead, when it is discouraged here. */
  replacement?: string;
  /** The spellings this project reached for. */
  terms?: ProjectTermUse[];
}

/** A decision that blessed a unit, and the source it cited. */
export interface BlessingRow {
  unit: string;
  variant?: string;
  status?: string;
  review_state?: string;
  /** The source hash the decision blessed — its basis. */
  target_hash?: string;
  project?: string;
  stale?: boolean;
}

/** How a selection relates to the rest of the graph. */
export interface RelationsAnswer {
  subject: RelationSubject;
  /**
   * How far this answer could see. A project-reach answer inside a workspace is
   * a smaller answer, not a wrong one — the pane says so rather than implying
   * the workspace holds nothing.
   */
  reach: "project" | "workspace";
  occurrences: OccurrenceRow[];
  occurrences_total?: number;
  projects: ProjectUse[];
  blessings: BlessingRow[];
  notes: string[];
}

/** One kind of by-content result. Kinds are never merged into one ranking. */
export type SearchGroup =
  | { kind: "terms"; hits: TermHit[] }
  | { kind: "precedent"; hits: PrecedentHit[] }
  | { kind: "profiles"; hits: ProfileWindow[] }
  | { kind: "content"; hits: ContentItem[] };

/** The kinds a group can carry, in the order the panel renders them. */
export const SEARCH_GROUP_ORDER: readonly SearchGroup["kind"][] = [
  "terms",
  "precedent",
  "content",
  "profiles",
] as const;

/** The by-content answer. */
export interface SearchAnswer {
  query: string;
  scope: AnswerScope;
  groups: SearchGroup[];
  notes: string[];
}

/** An option for one dimension of the filter bar, and the next ladder rung. */
export interface RungOption {
  value: string;
  label?: string;
  /** Items at this option, when the surface counts them. */
  count?: number;
}

/** A question asked at a point, with the tuple that scopes it. */
export interface ScopedQuery {
  scope: ScopeTuple;
  /** Cap on each result group. */
  limit?: number;
}
