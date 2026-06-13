// Brand knowledge-graph types (AD-021).
//
// These mirror the Go DTOs of the concept + change-set REST surface
// (bowrain/server/handlers_concepts.go, handlers_changesets.go and the
// bowrain/knowledge + termbase + core/graph packages). Field names are the
// snake_case JSON the server emits and accepts. Concept and term DTOs returned
// by the list/get/create concept routes reuse `ConceptInfo` / `TermInfo` /
// `TermSearchResult` from ./api; the types here add the graph, governance, and
// collaboration shapes that hang off a concept.

import type { TermRule } from "../brand/types";

// ---------------------------------------------------------------------------
// Shared atoms (termbase + core/graph)
// ---------------------------------------------------------------------------

/** Term lifecycle status (core/model.TermStatus). */
export type TermStatus =
  | "proposed"
  | "approved"
  | "preferred"
  | "admitted"
  | "deprecated"
  | "forbidden";

/** Where a concept comes from (termbase.TermSource). */
export type TermSource = "terminology" | "brand_vocabulary";

/**
 * The SKOS-aligned relation vocabulary (core/graph labels) used by concept
 * relations. REPLACED_BY is governed (it only reaches the live graph through a
 * change-set).
 */
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

/** All concept relation types, ordered for menus. */
export const RELATION_TYPES: RelationType[] = [
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

/**
 * Temporal + tag scoping on a graph edge or term (core/graph.Validity). A nil
 * validity means "always valid, everywhere". Tags carry the market vocabulary
 * (e.g. `{ market: "dach" }`).
 */
export interface Validity {
  valid_from?: string;
  valid_to?: string;
  tags?: Record<string, string>;
}

/**
 * A full term within a concept (termbase.Term) — the shape op payloads carry.
 * The reduced list/get concept response uses `TermInfo` from ./api instead.
 */
export interface Term {
  text: string;
  locale: string;
  status: TermStatus;
  part_of_speech?: string;
  gender?: string;
  note?: string;
  competitor_term?: boolean;
  validity?: Validity;
}

/**
 * A full concept (termbase.Concept) — the shape an OpConceptCreate payload and a
 * revision snapshot carry. The list/get/create concept routes return
 * `ConceptInfo` from ./api. created_at/updated_at are server-assigned and so
 * optional when building a create payload.
 */
export interface GraphConcept {
  id: string;
  project_id?: string;
  domain?: string;
  definition?: string;
  source?: TermSource;
  terms?: Term[];
  properties?: Record<string, string>;
  created_at?: string;
  updated_at?: string;
}

/**
 * A persisted, typed edge between two concepts (termbase.ConceptRelation). The
 * direct add route (AddConceptRelationRequest) lets the server mint id +
 * created_at; an OpRelationAdd payload must carry a complete relation.
 */
export interface ConceptRelation {
  id: string;
  source_id: string;
  target_id: string;
  relation_type: RelationType;
  note?: string;
  validity?: Validity;
  created_at: string;
}

// ---------------------------------------------------------------------------
// Graph visualization payload (GET /graph)
// ---------------------------------------------------------------------------

/** One concept node in the graph viz payload (server GraphVizNode). */
export interface GraphVizNode {
  id: string;
  label: string;
  domain?: string;
  status?: TermStatus | "";
  source?: TermSource | "";
  term_count: number;
}

/** One relation edge in the graph viz payload (server GraphVizEdge). */
export interface GraphVizEdge {
  id: string;
  source: string;
  target: string;
  type: RelationType;
  note?: string;
}

/** The force-directed graph payload (server GraphVizResponse). */
export interface GraphViz {
  nodes: GraphVizNode[];
  edges: GraphVizEdge[];
}

// ---------------------------------------------------------------------------
// Concept story (GET /concepts/:cid/story)
// ---------------------------------------------------------------------------

/** The source that produced a story entry. */
export type ConceptStoryKind = "revision" | "observation" | "comment" | "changeset";

/** One event on a concept's merged timeline (server ConceptStoryEntry). */
export interface ConceptStoryEntry {
  kind: ConceptStoryKind;
  at: string;
  actor?: string;
  summary?: string;
  ref?: string;
  /** The kind-specific record (ConceptRevision | Observation | Comment | ChangeSet). */
  data?: unknown;
}

/** A concept's merged chronological timeline (server ConceptStoryResponse). */
export interface ConceptStory {
  concept_id: string;
  entries: ConceptStoryEntry[];
}

/** An immutable concept snapshot (knowledge.ConceptRevision). */
export interface ConceptRevision {
  workspace_id: string;
  concept_id: string;
  rev: number;
  /** termbase.Concept + relations delta, JSON-encoded. */
  snapshot: unknown;
  summary?: string;
  actor: string;
  changeset_id?: string;
  created_at: string;
}

// ---------------------------------------------------------------------------
// Observations (GET/POST/DELETE /concepts/:cid/observations)
// ---------------------------------------------------------------------------

/** The kind of external evidence an observation records (knowledge.ObservationKind). */
export type ObservationKind =
  | "competitor"
  | "customer"
  | "style_guide"
  | "regulatory"
  | "web"
  | "internal";

/** All observation kinds, ordered for menus. */
export const OBSERVATION_KINDS: ObservationKind[] = [
  "competitor",
  "customer",
  "style_guide",
  "regulatory",
  "web",
  "internal",
];

/** External evidence attached to a concept (knowledge.Observation). */
export interface Observation {
  id: string;
  workspace_id: string;
  concept_id: string;
  kind: ObservationKind;
  quote: string;
  source: string;
  url?: string;
  locale?: string;
  market?: string;
  note?: string;
  created_by: string;
  created_at: string;
}

// ---------------------------------------------------------------------------
// Comments (GET/POST/DELETE /concepts/:cid/comments, .../resolve)
// ---------------------------------------------------------------------------

/** A threaded discussion entry on a concept or change-set (knowledge.Comment). */
export interface Comment {
  id: string;
  workspace_id: string;
  concept_id: string;
  /** Empty = top-level; set = a reply. */
  parent_id?: string;
  /** Set when the thread belongs to a change-set under review. */
  changeset_id?: string;
  body: string;
  author: string;
  created_at: string;
  resolved: boolean;
}

// ---------------------------------------------------------------------------
// Markets (GET/POST/PUT/DELETE /markets)
// ---------------------------------------------------------------------------

/** A workspace-defined scope: a name plus the locales it covers (knowledge.Market). */
export interface Market {
  id: string;
  workspace_id: string;
  name: string;
  description?: string;
  locales: string[];
  created_at: string;
  updated_at: string;
}

// ---------------------------------------------------------------------------
// Change-set ops (the 11 operations + their payloads)
// ---------------------------------------------------------------------------

/** The eleven change-set op types (knowledge.OpType). */
export type OpType =
  | "concept.create"
  | "concept.update"
  | "concept.delete"
  | "term.add"
  | "term.update"
  | "term.remove"
  | "term.status"
  | "relation.add"
  | "relation.remove"
  | "voice.rule.add"
  | "voice.rule.remove";

/** Which vocabulary list a voice rule joins (knowledge.VoiceRuleList). */
export type VoiceRuleList = "preferred" | "forbidden" | "competitor";

/** A brand vocabulary rule that references its backing concept (core/brand.TermRule). */
export type VoiceRule = TermRule & { concept_id?: string };

export interface ConceptCreatePayload {
  concept: GraphConcept;
}

export interface ConceptUpdatePayload {
  concept_id: string;
  domain?: string;
  definition?: string;
  properties?: Record<string, string>;
}

export interface ConceptDeletePayload {
  concept_id: string;
}

export interface TermAddPayload {
  concept_id: string;
  term: Term;
}

export interface TermUpdatePayload {
  concept_id: string;
  locale: string;
  text: string;
  term: Term;
}

export interface TermRemovePayload {
  concept_id: string;
  locale: string;
  text: string;
}

export interface TermStatusPayload {
  concept_id: string;
  locale: string;
  text: string;
  from: TermStatus;
  to: TermStatus;
  validity?: Validity;
}

export interface RelationAddPayload {
  relation: ConceptRelation;
}

export interface RelationRemovePayload {
  relation_id: string;
}

export interface VoiceRuleAddPayload {
  profile_id: string;
  list: VoiceRuleList;
  rule: VoiceRule;
}

export interface VoiceRuleRemovePayload {
  profile_id: string;
  list: VoiceRuleList;
  term: string;
}

/** The decoded payload of a stored op, narrowable by the op's `op` discriminant. */
export type ChangeSetOpPayload =
  | ConceptCreatePayload
  | ConceptUpdatePayload
  | ConceptDeletePayload
  | TermAddPayload
  | TermUpdatePayload
  | TermRemovePayload
  | TermStatusPayload
  | RelationAddPayload
  | RelationRemovePayload
  | VoiceRuleAddPayload
  | VoiceRuleRemovePayload;

/** One ordered op within a change-set (knowledge.ChangeSetOp). */
export interface ChangeSetOp {
  workspace_id: string;
  changeset_id: string;
  seq: number;
  op: OpType;
  payload: ChangeSetOpPayload;
  base_rev: number;
  created_by: string;
  created_at: string;
}

/**
 * The body of POST /changesets/:id/ops (server AddChangeSetOpRequest). A
 * discriminated union so the payload type follows the chosen op.
 */
export type AddChangeSetOpRequest =
  | { op: "concept.create"; payload: ConceptCreatePayload; base_rev?: number }
  | { op: "concept.update"; payload: ConceptUpdatePayload; base_rev?: number }
  | { op: "concept.delete"; payload: ConceptDeletePayload; base_rev?: number }
  | { op: "term.add"; payload: TermAddPayload; base_rev?: number }
  | { op: "term.update"; payload: TermUpdatePayload; base_rev?: number }
  | { op: "term.remove"; payload: TermRemovePayload; base_rev?: number }
  | { op: "term.status"; payload: TermStatusPayload; base_rev?: number }
  | { op: "relation.add"; payload: RelationAddPayload; base_rev?: number }
  | { op: "relation.remove"; payload: RelationRemovePayload; base_rev?: number }
  | { op: "voice.rule.add"; payload: VoiceRuleAddPayload; base_rev?: number }
  | { op: "voice.rule.remove"; payload: VoiceRuleRemovePayload; base_rev?: number };

// ---------------------------------------------------------------------------
// Change-sets, reviews, pilots
// ---------------------------------------------------------------------------

/** The lifecycle state of a change-set (knowledge.ChangeSetStatus). */
export type ChangeSetStatus = "draft" | "in_review" | "approved" | "merged" | "abandoned";

/** All change-set statuses, in lifecycle order. */
export const CHANGE_SET_STATUSES: ChangeSetStatus[] = [
  "draft",
  "in_review",
  "approved",
  "merged",
  "abandoned",
];

/** The outcome a reviewer records (knowledge.ReviewVerdict). */
export type ReviewVerdict = "approve" | "reject";

/** A named, reviewable draft of graph + voice edits (knowledge.ChangeSet). */
export interface ChangeSet {
  id: string;
  workspace_id: string;
  name: string;
  description?: string;
  status: ChangeSetStatus;
  created_by: string;
  created_at: string;
  updated_at: string;
  submitted_at?: string;
  merged_at?: string;
  merged_by?: string;
}

/** A reviewer's verdict on a change-set (knowledge.ChangeSetReview). */
export interface ChangeSetReview {
  workspace_id: string;
  changeset_id: string;
  reviewer: string;
  verdict: ReviewVerdict;
  comment?: string;
  created_at: string;
}

/** A change-set bound to a project's content stream (knowledge.Pilot). */
export interface Pilot {
  workspace_id: string;
  changeset_id: string;
  project_id: string;
  stream: string;
  created_by: string;
  created_at: string;
}

/**
 * A change-set with its ops, reviews, and pilots, plus whether it carries any
 * governed op (server ChangeSetDetailResponse). Returned by GET /changesets/:id
 * and the lifecycle transitions that re-read it.
 */
export interface ChangeSetDetail extends ChangeSet {
  governed: boolean;
  ops: ChangeSetOp[];
  reviews: ChangeSetReview[];
  pilots: Pilot[];
}

// ---------------------------------------------------------------------------
// Blast radius / where-used (knowledge.ChangeSetImpact, ConceptUsage)
// ---------------------------------------------------------------------------

/** One inspectable affected/matching block (knowledge.BlockSample). */
export interface BlockSample {
  project_id: string;
  stream: string;
  collection_id: string;
  collection_name: string;
  locale: string;
  item_name: string;
  block_id: string;
  text: string;
  new_violations?: number;
  resolved?: number;
  occurrences?: number;
}

/** The per-(stream, locale) leaf of a change-set impact (knowledge.LocaleImpact). */
export interface LocaleImpact {
  stream: string;
  locale: string;
  affected_blocks: number;
  new_violations: number;
  resolved: number;
  words: number;
}

/** The per-collection slice of a change-set impact (knowledge.CollectionImpact). */
export interface CollectionImpact {
  collection_id: string;
  collection_name: string;
  affected_blocks: number;
  new_violations: number;
  resolved: number;
  words: number;
  locales: LocaleImpact[];
}

/** The per-project slice of a change-set impact (knowledge.ProjectImpact). */
export interface ProjectImpact {
  project_id: string;
  project_name: string;
  affected_blocks: number;
  new_violations: number;
  resolved: number;
  words: number;
  collections: CollectionImpact[];
}

/** The blast radius of a change-set over stored content (knowledge.ChangeSetImpact). */
export interface ChangeSetImpact {
  total_blocks: number;
  affected_blocks: number;
  new_violations: number;
  resolved: number;
  words: number;
  projects: ProjectImpact[];
  samples: BlockSample[];
}

/** The per-(stream, locale) leaf of a concept usage report (knowledge.LocaleUsage). */
export interface LocaleUsage {
  stream: string;
  locale: string;
  blocks: number;
  occurrences: number;
  words: number;
}

/** The per-collection slice of a concept usage report (knowledge.CollectionUsage). */
export interface CollectionUsage {
  collection_id: string;
  collection_name: string;
  blocks: number;
  occurrences: number;
  words: number;
  locales: LocaleUsage[];
}

/** The per-project slice of a concept usage report (knowledge.ProjectUsage). */
export interface ProjectUsage {
  project_id: string;
  project_name: string;
  blocks: number;
  occurrences: number;
  words: number;
  collections: CollectionUsage[];
}

/**
 * The where-used footprint of a single concept (knowledge.ConceptUsage) —
 * returned by GET /concepts/:cid/blast-radius.
 */
export interface ConceptUsage {
  concept_id: string;
  total_blocks: number;
  blocks: number;
  occurrences: number;
  words: number;
  projects: ProjectUsage[];
  samples: BlockSample[];
}

// ---------------------------------------------------------------------------
// Merge result (knowledge.MergeResult / MergeEvent / OpConflict)
// ---------------------------------------------------------------------------

/** A stale-draft conflict that blocked a merge (knowledge.OpConflict). */
export interface OpConflict {
  seq: number;
  concept_id: string;
  reason: string;
}

/** A domain event emitted by a merge or pilot transition (knowledge.MergeEvent). */
export interface MergeEvent {
  type: string;
  workspace_id: string;
  changeset_id?: string;
  concept_id?: string;
  profile_id?: string;
  project_id?: string;
  stream?: string;
  actor?: string;
}

/** The outcome of merging a change-set (knowledge.MergeResult). */
export interface MergeResult {
  changeset_id: string;
  conflicts?: OpConflict[];
  applied_ops?: number[];
  revisions_created: number;
  concepts_touched?: string[];
  profiles_touched?: string[];
  pilots_stopped: number;
  events?: MergeEvent[];
}

// ---------------------------------------------------------------------------
// Request bodies + query params
// ---------------------------------------------------------------------------

/** Body of POST /concepts/:cid/relations (server AddConceptRelationRequest). */
export interface AddConceptRelationRequest {
  target_id: string;
  relation_type: RelationType;
  note?: string;
  validity?: Validity;
}

/** Body of POST /concepts/:cid/observations (server AddObservationRequest). */
export interface AddObservationRequest {
  kind: ObservationKind;
  quote: string;
  source: string;
  url?: string;
  locale?: string;
  market?: string;
  note?: string;
}

/** Body of POST /concepts/:cid/comments (server AddCommentRequest). */
export interface AddCommentRequest {
  body: string;
  parent_id?: string;
  changeset_id?: string;
}

/** Body of POST/PUT /markets (server MarketRequest). */
export interface MarketRequest {
  name: string;
  description?: string;
  locales?: string[];
}

/** Body of POST /changesets (server CreateChangeSetRequest). */
export interface CreateChangeSetRequest {
  name: string;
  description?: string;
}

/** Body of PATCH /changesets/:id (server UpdateChangeSetRequest). */
export interface UpdateChangeSetRequest {
  name?: string;
  description?: string;
}

/** Body of POST /changesets/:id/approve|reject (server ReviewRequest). */
export interface ReviewRequest {
  comment?: string;
}

/** Body of POST /changesets/:id/pilots (server StartPilotRequest). */
export interface StartPilotRequest {
  project_id: string;
  stream: string;
}

/** Query params for GET /concepts. */
export interface ListConceptsParams {
  q?: string;
  status?: TermStatus;
  domain?: string;
  market?: string;
  locale?: string;
  source?: TermSource;
  stream?: string;
  project_id?: string;
  offset?: number;
  limit?: number;
}

/** Query params for GET /graph. focus + depth restrict to a concept neighborhood. */
export interface GraphParams {
  as_of?: string;
  market?: string;
  domain?: string;
  status?: TermStatus;
  focus?: string;
  depth?: number;
}

/** Validity scope for relation reads (as_of RFC3339 + market). */
export interface RelationScope {
  as_of?: string;
  market?: string;
}
