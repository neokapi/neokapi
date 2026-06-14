// LocalConceptDataSource — drives @neokapi/concept-ui against a LOCAL SQLite
// termbase through the Wails backend bindings. This is the desktop replacement
// for the deleted CLI relation commands: the framework concept UI browses,
// relates, and re-statuses concepts against the author's own local copy.
//
// The source is bound to one open termbase handle and supplies the required
// CORE reads (concepts, terms, relations) plus the editable-core mutations
// (addRelation/removeRelation/setTermStatus/updateTerm). It supplies NONE of the
// rich reads (markets/observations/comments/timeline/whereUsed) — a local copy
// has no server-side evidence/discussion/revision log — so deriveCapabilities
// yields editRelations + editTerms TRUE and every rich flag FALSE, and the
// dashboard degrades to terms + relations + tag-derived geography + constraints
// + a synthesized timeline. Governance is the server's job, applied on push;
// there is no governance gate here.

import type {
  Concept,
  ConceptDataSource,
  ConceptListResult,
  ConceptQuery,
  ConceptSummary,
  Relation,
  RelationInput,
  Term,
  TermPatch,
  TermRef,
  TermStatus,
  TermSource,
  Validity,
} from "@neokapi/concept-ui";
import { call } from "../hooks/useApi";

// The local termbase Search supports free-text + paging only (no server-side
// status/domain/market facets), and the concept list has no pager, so list reads
// fetch a generous first page; users narrow with the search box.
const DEFAULT_LIST_LIMIT = 200;

// ── Wire DTOs (snake_case) — mirror apps/kapi-desktop/backend ──────────────────

/** Temporal/tag scoping on a term or relation (mirrors backend ValidityDTO). */
export interface ValidityDTO {
  valid_from?: string;
  valid_to?: string;
  tags?: Record<string, string>;
}

/** A term within a concept (mirrors backend TermDTO, incl. validity). */
export interface TermDTO {
  text: string;
  locale: string;
  status: string;
  part_of_speech?: string;
  gender?: string;
  note?: string;
  competitor_term?: boolean;
  validity?: ValidityDTO;
}

/** A termbase concept (mirrors backend ConceptDTO). */
export interface ConceptDTO {
  id: string;
  project_id: string;
  domain: string;
  definition: string;
  source: string;
  terms: TermDTO[];
  properties?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

/** Paginated search result (mirrors backend TermSearchResult). */
export interface TermSearchResult {
  concepts: ConceptDTO[];
  total_count: number;
}

/** A typed edge between two concepts (mirrors backend RelationDTO). */
export interface RelationDTO {
  id: string;
  source_id: string;
  target_id: string;
  type: string;
  note?: string;
  validity?: ValidityDTO;
}

/** Create-relation request (mirrors backend AddRelationRequest). */
export interface AddRelationRequest {
  source_id: string;
  target_id: string;
  type: string;
  note?: string;
  valid_from?: string;
  valid_to?: string;
  tags?: Record<string, string>;
}

/** Set-term-status request (mirrors backend SetTermStatusRequest). */
export interface SetTermStatusRequest {
  concept_id: string;
  locale: string;
  text: string;
  status: string;
  valid_from?: string;
  valid_to?: string;
  tags?: Record<string, string>;
}

/** Whole-concept update request (mirrors backend UpdateConceptRequest). */
export interface UpdateConceptRequest {
  concept_id: string;
  project_id?: string;
  domain: string;
  definition: string;
  terms: TermDTO[];
}

// ── Backend port ───────────────────────────────────────────────────────────────

/**
 * The minimal slice of the Wails backend the concept source needs. Injectable so
 * the mapping logic is unit-testable with a fake; the default implementation
 * dispatches to the generated bindings via `call` (which returns null outside a
 * Wails runtime, e.g. Storybook/vitest).
 */
export interface ConceptBackend {
  searchTerms(
    handle: string,
    query: string,
    srcLocale: string,
    tgtLocale: string,
    offset: number,
    limit: number,
  ): Promise<TermSearchResult | null>;
  getConceptForView(handle: string, conceptID: string): Promise<ConceptDTO | null>;
  getRelations(handle: string, conceptID: string): Promise<RelationDTO[] | null>;
  addRelation(handle: string, req: AddRelationRequest): Promise<RelationDTO | null>;
  removeRelation(handle: string, relationID: string): Promise<void>;
  setTermStatus(handle: string, req: SetTermStatusRequest): Promise<void>;
  updateConcept(handle: string, req: UpdateConceptRequest): Promise<void>;
}

/** The production backend: dispatches each method to its Wails binding. */
export const wailsConceptBackend: ConceptBackend = {
  searchTerms: (handle, query, srcLocale, tgtLocale, offset, limit) =>
    call<TermSearchResult>("SearchTerms", handle, query, srcLocale, tgtLocale, offset, limit),
  getConceptForView: (handle, conceptID) =>
    call<ConceptDTO>("GetConceptForView", handle, conceptID),
  getRelations: (handle, conceptID) => call<RelationDTO[]>("GetRelations", handle, conceptID),
  addRelation: (handle, req) => call<RelationDTO>("AddRelation", handle, req),
  removeRelation: async (handle, relationID) => {
    await call<void>("RemoveRelation", handle, relationID);
  },
  setTermStatus: async (handle, req) => {
    await call<void>("SetTermStatus", handle, req);
  },
  updateConcept: async (handle, req) => {
    await call<void>("UpdateConcept", handle, req);
  },
};

// ── DTO ↔ concept-ui mapping ────────────────────────────────────────────────

/** Map a wire validity to the concept-ui Validity, or undefined when absent. */
export function validityFromDTO(dto?: ValidityDTO | null): Validity | undefined {
  if (!dto) return undefined;
  const v: Validity = {};
  if (dto.valid_from) v.validFrom = dto.valid_from;
  if (dto.valid_to) v.validTo = dto.valid_to;
  if (dto.tags) v.tags = dto.tags;
  return v;
}

/** Map a concept-ui Validity to its wire DTO, or undefined when absent. */
export function validityToDTO(v?: Validity): ValidityDTO | undefined {
  if (!v) return undefined;
  const dto: ValidityDTO = {};
  if (v.validFrom) dto.valid_from = v.validFrom;
  if (v.validTo) dto.valid_to = v.validTo;
  if (v.tags) dto.tags = v.tags;
  return dto;
}

function termFromDTO(t: TermDTO): Term {
  return {
    text: t.text,
    locale: t.locale,
    status: t.status as TermStatus,
    partOfSpeech: t.part_of_speech,
    gender: t.gender,
    note: t.note,
    validity: validityFromDTO(t.validity),
  };
}

function conceptSummaryFromDTO(c: ConceptDTO): ConceptSummary {
  return {
    id: c.id,
    domain: c.domain || undefined,
    definition: c.definition || undefined,
    source: c.source ? (c.source as TermSource) : undefined,
    terms: c.terms.map(termFromDTO),
  };
}

function conceptFromDTO(c: ConceptDTO): Concept {
  return {
    ...conceptSummaryFromDTO(c),
    properties: c.properties,
    createdAt: c.created_at || undefined,
    updatedAt: c.updated_at || undefined,
  };
}

function relationFromDTO(r: RelationDTO): Relation {
  return {
    id: r.id,
    sourceId: r.source_id,
    targetId: r.target_id,
    type: r.type as Relation["type"],
    note: r.note,
    validity: validityFromDTO(r.validity),
  };
}

// ── Source factory ─────────────────────────────────────────────────────────────

/**
 * Build a {@link ConceptDataSource} bound to one open termbase `handle`. The
 * supplied edit methods make deriveCapabilities report editRelations + editTerms;
 * the absence of any rich read keeps every rich flag false.
 */
export function createLocalConceptSource(
  handle: string,
  backend: ConceptBackend = wailsConceptBackend,
): ConceptDataSource {
  return {
    async listConcepts(query: ConceptQuery): Promise<ConceptListResult> {
      const result = await backend.searchTerms(
        handle,
        query.text ?? "",
        "",
        "",
        query.offset ?? 0,
        query.limit ?? DEFAULT_LIST_LIMIT,
      );
      if (!result) return { concepts: [], total: 0 };
      return {
        concepts: result.concepts.map(conceptSummaryFromDTO),
        total: result.total_count,
      };
    },

    async getConcept(id: string): Promise<Concept | null> {
      const dto = await backend.getConceptForView(handle, id);
      return dto ? conceptFromDTO(dto) : null;
    },

    async getRelations(conceptId: string): Promise<Relation[]> {
      const rels = await backend.getRelations(handle, conceptId);
      return (rels ?? []).map(relationFromDTO);
    },

    async addRelation(conceptId: string, input: RelationInput): Promise<Relation> {
      const rel = await backend.addRelation(handle, {
        source_id: conceptId,
        target_id: input.targetId,
        type: input.type,
        note: input.note,
        valid_from: input.validity?.validFrom,
        valid_to: input.validity?.validTo,
        tags: input.validity?.tags,
      });
      if (!rel) throw new Error("addRelation: backend unavailable");
      return relationFromDTO(rel);
    },

    async removeRelation(relationId: string): Promise<void> {
      await backend.removeRelation(handle, relationId);
    },

    async setTermStatus(
      conceptId: string,
      ref: TermRef,
      status: TermStatus,
      validity?: Validity,
    ): Promise<void> {
      await backend.setTermStatus(handle, {
        concept_id: conceptId,
        locale: ref.locale,
        text: ref.text,
        status,
        valid_from: validity?.validFrom,
        valid_to: validity?.validTo,
        tags: validity?.tags,
      });
    },

    // updateTerm edits text/note/validity (and optionally status). The backend
    // has no per-term patch, so load the concept, patch the matching term DTO in
    // place, and re-persist the whole term list via UpdateConcept (TermDTO now
    // carries validity, so it survives the wholesale round-trip).
    async updateTerm(conceptId: string, ref: TermRef, patch: TermPatch): Promise<void> {
      const concept = await backend.getConceptForView(handle, conceptId);
      if (!concept) return;
      const term = concept.terms.find((t) => t.locale === ref.locale && t.text === ref.text);
      if (!term) return;
      if (patch.text !== undefined) term.text = patch.text;
      if (patch.status !== undefined) term.status = patch.status;
      if (patch.note !== undefined) term.note = patch.note;
      if (patch.validity !== undefined) term.validity = validityToDTO(patch.validity);
      await backend.updateConcept(handle, {
        concept_id: concept.id,
        project_id: concept.project_id,
        domain: concept.domain,
        definition: concept.definition,
        terms: concept.terms,
      });
    },
  };
}
