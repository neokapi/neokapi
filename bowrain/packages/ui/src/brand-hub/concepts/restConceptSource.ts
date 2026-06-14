// RestConceptDataSource — drives the framework concept UI (@neokapi/concept-ui)
// against bowrain's REST surface through an ApiAdapter (AD-021). This is the
// bowrain replacement for the deleted whole-graph view: the SAME components that
// kapi-desktop runs against a local termbase here get the FULL feature set —
// every rich read (named markets, observations, comments, a revision timeline,
// where-used) and governance-aware editing.
//
// One factory builds the source for both workspace adapters (web RestApiAdapter,
// desktop WailsApiAdapter — both implement ApiAdapter), so the source maps the
// snake_case wire DTOs onto the camelCase concept-ui view types and threads
// bowrain's governance through the edit methods: a governed transition (a term
// status to/from forbidden/preferred, un-forbidding a term, or a REPLACED_BY
// relation) is refused by the server with a 409 and a change-set hint. The source
// catches that and surfaces it as a typed {@link GovernedEditError} (and an
// optional onGovernedEdit callback) so the consuming view can offer "open it as
// an experiment" instead of failing silently. Ordinary edits apply directly.

import type {
  Comment,
  Concept,
  ConceptDataSource,
  ConceptListResult,
  ConceptQuery,
  ConceptSummary,
  Market,
  Observation,
  Relation,
  RelationInput,
  Term,
  TermPatch,
  TermRef,
  TermStatus,
  TimelineEvent,
  TimelineKind,
  Validity,
  WhereUsed,
} from "@neokapi/concept-ui";
import type { ApiAdapter } from "../../api/adapter";
import type { ConceptInfo, TermInfo, UpdateConceptRequest } from "../../types/api";
import type {
  AddConceptRelationRequest,
  Comment as GraphComment,
  ConceptRelation,
  ConceptStoryEntry,
  ConceptStoryKind,
  ConceptUsage,
  ListConceptsParams,
  Market as GraphMarket,
  Observation as GraphObservation,
  Validity as GraphValidity,
} from "../../types/brand-graph";

// ── Governed-edit error ──────────────────────────────────────────────────────

/**
 * Thrown by the edit methods when bowrain refuses a direct change because it is
 * governed and must travel through a reviewed change-set (the server answers the
 * direct path with a 409 + change-set hint). The consuming view catches this to
 * route the user to the Experiments section instead of failing silently.
 */
export class GovernedEditError extends Error {
  /** The concept the refused edit targeted. */
  readonly conceptId: string;
  /** The server's human-readable description of what is governed, when present. */
  readonly detail?: string;
  /** The server's hint on how to proceed (open a change-set …), when present. */
  readonly hint?: string;

  constructor(args: { message: string; conceptId: string; detail?: string; hint?: string }) {
    super(args.message);
    this.name = "GovernedEditError";
    this.conceptId = args.conceptId;
    this.detail = args.detail;
    this.hint = args.hint;
    // Restore the prototype chain so `instanceof` holds after transpilation.
    Object.setPrototypeOf(this, GovernedEditError.prototype);
  }
}

/** Narrow an unknown caught value to a {@link GovernedEditError}. */
export function isGovernedEditError(value: unknown): value is GovernedEditError {
  return value instanceof GovernedEditError;
}

/**
 * Recognise the server's governed-conflict response in a thrown adapter error
 * and translate it to a {@link GovernedEditError}; returns null for any other
 * error. Both transports embed the HTTP status and the server's governed-conflict
 * JSON body (conceptGovernedConflict → `{ error, detail, hint }`):
 *   • web RestApiAdapter — `"409: {…}"`
 *   • desktop REST proxy — `"POST …: server returned 409: {…}"`
 */
export function asGovernedEditError(error: unknown, conceptId: string): GovernedEditError | null {
  const message = error instanceof Error ? error.message : String(error);
  if (!/\b409\b/.test(message)) return null;

  let governed = message.includes("governed change requires a change-set");
  let detail: string | undefined;
  let hint: string | undefined;

  const brace = message.indexOf("{");
  if (brace >= 0) {
    try {
      const body = JSON.parse(message.slice(brace)) as {
        error?: string;
        detail?: string;
        hint?: string;
      };
      if (typeof body.error === "string" && body.error.toLowerCase().includes("governed")) {
        governed = true;
        detail = body.detail;
        hint = body.hint;
      }
    } catch {
      // The 409 body was not the JSON we expected; rely on the phrase check.
    }
  }

  if (!governed) return null;
  return new GovernedEditError({
    conceptId,
    detail,
    hint,
    message: detail
      ? `This change needs review — ${detail}. Open it as an experiment to submit it for review.`
      : "This change needs review. Open it as an experiment to submit it for review.",
  });
}

// ── DTO ↔ concept-ui mapping ──────────────────────────────────────────────────

/** Map a wire validity (snake_case) to the concept-ui Validity, or undefined. */
export function validityFromWire(v?: GraphValidity): Validity | undefined {
  if (!v) return undefined;
  const out: Validity = {};
  if (v.valid_from) out.validFrom = v.valid_from;
  if (v.valid_to) out.validTo = v.valid_to;
  if (v.tags) out.tags = v.tags;
  return Object.keys(out).length > 0 ? out : undefined;
}

/** Map a concept-ui Validity to its wire DTO (snake_case), or undefined. */
export function validityToWire(v?: Validity): GraphValidity | undefined {
  if (!v) return undefined;
  const out: GraphValidity = {};
  if (v.validFrom) out.valid_from = v.validFrom;
  if (v.validTo) out.valid_to = v.validTo;
  if (v.tags) out.tags = v.tags;
  return Object.keys(out).length > 0 ? out : undefined;
}

/** Map a list/get-concept term (TermInfo) to a concept-ui Term. */
export function termFromInfo(term: TermInfo): Term {
  return {
    text: term.text,
    locale: term.locale,
    status: (term.status || "proposed") as TermStatus,
    partOfSpeech: term.part_of_speech || undefined,
    gender: term.gender || undefined,
    note: term.note || undefined,
  };
}

/** Map a ConceptInfo to the reduced concept-ui ConceptSummary. */
export function conceptSummaryFromInfo(concept: ConceptInfo): ConceptSummary {
  return {
    id: concept.id,
    domain: concept.domain || undefined,
    definition: concept.definition || undefined,
    terms: concept.terms.map(termFromInfo),
  };
}

/** Map a ConceptInfo to the full concept-ui Concept. */
export function conceptFromInfo(concept: ConceptInfo): Concept {
  return {
    ...conceptSummaryFromInfo(concept),
    properties: concept.properties,
    createdAt: concept.created_at || undefined,
    updatedAt: concept.updated_at || undefined,
  };
}

/** Map a persisted relation DTO to the concept-ui Relation. */
export function relationFromDTO(relation: ConceptRelation): Relation {
  return {
    id: relation.id,
    sourceId: relation.source_id,
    targetId: relation.target_id,
    type: relation.relation_type,
    note: relation.note || undefined,
    validity: validityFromWire(relation.validity),
  };
}

/** Map a market DTO to the concept-ui Market. */
export function marketFromDTO(market: GraphMarket): Market {
  return {
    id: market.id,
    name: market.name,
    description: market.description || undefined,
    locales: market.locales ?? [],
  };
}

/** Map an observation DTO to the concept-ui Observation. */
export function observationFromDTO(observation: GraphObservation): Observation {
  return {
    id: observation.id,
    kind: observation.kind,
    quote: observation.quote,
    source: observation.source,
    url: observation.url || undefined,
    locale: observation.locale || undefined,
    market: observation.market || undefined,
    note: observation.note || undefined,
    actor: observation.created_by || undefined,
    at: observation.created_at,
  };
}

/** Map a comment DTO to the concept-ui Comment. */
export function commentFromDTO(comment: GraphComment): Comment {
  return {
    id: comment.id,
    parentId: comment.parent_id || undefined,
    body: comment.body,
    author: comment.author,
    at: comment.created_at,
    resolved: comment.resolved,
  };
}

function defaultTimelineSummary(kind: ConceptStoryKind): string {
  switch (kind) {
    case "revision":
      return "Concept revised";
    case "observation":
      return "Observation added";
    case "comment":
      return "Comment posted";
    case "changeset":
      return "Change-set";
    default:
      return "Event";
  }
}

function timelineDetail(entry: ConceptStoryEntry): string | undefined {
  if (!entry.data || typeof entry.data !== "object") return undefined;
  const data = entry.data as Record<string, unknown>;
  if (entry.kind === "comment" && typeof data.body === "string") return data.body;
  return undefined;
}

/** Map a story entry (the server's merged timeline) to a concept-ui TimelineEvent. */
export function timelineEventFromStory(entry: ConceptStoryEntry): TimelineEvent {
  const event: TimelineEvent = {
    // ConceptStoryKind is a subset of TimelineKind, so the kind carries across.
    kind: entry.kind as TimelineKind,
    at: entry.at,
    actor: entry.actor || undefined,
    summary: entry.summary || defaultTimelineSummary(entry.kind),
    ref: entry.ref || undefined,
  };
  const detail = timelineDetail(entry);
  if (detail) event.detail = detail;
  return event;
}

/** Map a concept's blast-radius usage to the concept-ui WhereUsed footprint. */
export function whereUsedFromUsage(usage: ConceptUsage): WhereUsed {
  return {
    conceptId: usage.concept_id,
    // `blocks` is the count of blocks that actually contain a term of the
    // concept (the concept-hit count). `total_blocks` is the workspace scan
    // denominator — (block, locale) rows scanned — and must NOT be shown here.
    blocks: usage.blocks,
    occurrences: usage.occurrences,
    words: usage.words,
  };
}

function paramsFromQuery(query: ConceptQuery): ListConceptsParams {
  const params: ListConceptsParams = {};
  if (query.text) params.q = query.text;
  if (query.status) params.status = query.status;
  if (query.domain) params.domain = query.domain;
  if (query.source) params.source = query.source;
  if (query.market) params.market = query.market;
  if (query.locale) params.locale = query.locale;
  if (query.offset !== undefined) params.offset = query.offset;
  if (query.limit !== undefined) params.limit = query.limit;
  return params;
}

function isNotFound(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error);
  return /\b404\b/.test(message) || /not found/i.test(message);
}

// ── Factory ───────────────────────────────────────────────────────────────────

/** Options for {@link createRestConceptSource}. */
export interface RestConceptSourceOptions {
  /**
   * Invoked when an edit is refused as governed (before the typed error is
   * thrown). The consuming view uses this to surface a "open it as an experiment"
   * affordance rather than letting the rejection fail silently.
   */
  onGovernedEdit?: (error: GovernedEditError) => void;
}

/**
 * Build a {@link ConceptDataSource} over a bowrain {@link ApiAdapter} for one
 * workspace. Every capability is on: the core reads, the rich reads (markets,
 * observations, comments, timeline, where-used), and the governance-aware edit
 * mutations. The same factory serves the web (RestApiAdapter) and desktop
 * (WailsApiAdapter) surfaces.
 */
export function createRestConceptSource(
  adapter: ApiAdapter,
  workspaceSlug: string,
  options: RestConceptSourceOptions = {},
): ConceptDataSource {
  // The concept-ui relations widget removes a relation by id alone, but the
  // server route is /concepts/:cid/relations/:rid — so remember which concept a
  // relation was last listed/created under to address the delete.
  const relationOwner = new Map<string, string>();

  const governedOrError = (error: unknown, conceptId: string): Error => {
    const governed = asGovernedEditError(error, conceptId);
    if (governed) {
      options.onGovernedEdit?.(governed);
      return governed;
    }
    return error instanceof Error ? error : new Error(String(error));
  };

  const persistConcept = async (
    base: ConceptInfo,
    terms: TermInfo[],
    conceptId: string,
  ): Promise<void> => {
    const request: UpdateConceptRequest = {
      project_id: base.project_id ?? "",
      concept_id: base.id,
      domain: base.domain,
      definition: base.definition,
      terms,
    };
    try {
      await adapter.updateConcept(workspaceSlug, request);
    } catch (error) {
      throw governedOrError(error, conceptId);
    }
  };

  return {
    // Every method is implemented, so deriveCapabilities already yields all-true;
    // the explicit flags make the full feature set unambiguous.
    capabilities: {
      editTerms: true,
      editRelations: true,
      markets: true,
      observations: true,
      comments: true,
      timeline: true,
      whereUsed: true,
    },

    // ── Core reads ────────────────────────────────────────────────────────────

    async listConcepts(query: ConceptQuery): Promise<ConceptListResult> {
      const result = await adapter.listConcepts(workspaceSlug, paramsFromQuery(query));
      return {
        concepts: result.concepts.map(conceptSummaryFromInfo),
        total: result.total_count,
      };
    },

    async getConcept(id: string): Promise<Concept | null> {
      try {
        return conceptFromInfo(await adapter.getConcept(workspaceSlug, id));
      } catch (error) {
        if (isNotFound(error)) return null;
        throw error;
      }
    },

    async getRelations(conceptId: string): Promise<Relation[]> {
      const relations = await adapter.listConceptRelations(workspaceSlug, conceptId);
      for (const relation of relations) relationOwner.set(relation.id, conceptId);
      return relations.map(relationFromDTO);
    },

    // ── Rich reads ────────────────────────────────────────────────────────────

    async getConceptSummary(id: string): Promise<ConceptSummary | null> {
      // The relations/constraints/timeline widgets label a neighbour by id; this
      // lightweight read resolves the name without the consumer falling back to a
      // full getConcept. Missing neighbours map to null so the widget shows the id.
      try {
        return conceptSummaryFromInfo(await adapter.getConcept(workspaceSlug, id));
      } catch (error) {
        if (isNotFound(error)) return null;
        throw error;
      }
    },

    async getMarkets(): Promise<Market[]> {
      const markets = await adapter.listMarkets(workspaceSlug);
      return markets.map(marketFromDTO);
    },

    async getObservations(conceptId: string): Promise<Observation[]> {
      const observations = await adapter.listObservations(workspaceSlug, conceptId);
      return observations.map(observationFromDTO);
    },

    async getComments(conceptId: string): Promise<Comment[]> {
      const comments = await adapter.listConceptComments(workspaceSlug, conceptId);
      return comments.map(commentFromDTO);
    },

    async getTimeline(conceptId: string): Promise<TimelineEvent[]> {
      const story = await adapter.getConceptStory(workspaceSlug, conceptId);
      return story.entries.map(timelineEventFromStory);
    },

    async getWhereUsed(conceptId: string): Promise<WhereUsed | null> {
      const usage = await adapter.getConceptBlastRadius(workspaceSlug, conceptId);
      return whereUsedFromUsage(usage);
    },

    // ── Governance-aware edits ────────────────────────────────────────────────

    async addRelation(conceptId: string, input: RelationInput): Promise<Relation> {
      const request: AddConceptRelationRequest = {
        target_id: input.targetId,
        relation_type: input.type,
        note: input.note,
        validity: validityToWire(input.validity),
      };
      try {
        const relation = await adapter.addConceptRelation(workspaceSlug, conceptId, request);
        relationOwner.set(relation.id, conceptId);
        return relationFromDTO(relation);
      } catch (error) {
        throw governedOrError(error, conceptId);
      }
    },

    async removeRelation(relationId: string): Promise<void> {
      const conceptId = relationOwner.get(relationId);
      if (!conceptId) {
        throw new Error(
          `removeRelation: relation ${relationId} has no known owning concept; load its relations first`,
        );
      }
      await adapter.deleteConceptRelation(workspaceSlug, conceptId, relationId);
      relationOwner.delete(relationId);
    },

    async setTermStatus(conceptId: string, ref: TermRef, status: TermStatus): Promise<void> {
      const concept = await adapter.getConcept(workspaceSlug, conceptId);
      const terms = concept.terms.map((term) =>
        term.locale === ref.locale && term.text === ref.text ? { ...term, status } : term,
      );
      await persistConcept(concept, terms, conceptId);
    },

    async updateTerm(conceptId: string, ref: TermRef, patch: TermPatch): Promise<void> {
      const concept = await adapter.getConcept(workspaceSlug, conceptId);
      const terms = concept.terms.map((term) => {
        if (term.locale !== ref.locale || term.text !== ref.text) return term;
        const next: TermInfo = { ...term };
        if (patch.text !== undefined) next.text = patch.text;
        if (patch.status !== undefined) next.status = patch.status;
        if (patch.note !== undefined) next.note = patch.note;
        // TermInfo carries no validity on the REST surface, so patch.validity is
        // intentionally not round-tripped — it is not part of the REST model.
        return next;
      });
      await persistConcept(concept, terms, conceptId);
    },
  };
}
