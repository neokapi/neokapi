// Concepts for the Terms stories and tests, in the snake_case wire shape the
// Wails backend returns, behind an in-memory backend. Stories drive the same
// local concept source the desktop uses, so selecting a concept, relating it
// and removing a relation behave as they do against a real terms store.

import type { ConceptDataSource } from "@neokapi/concept-ui";
import {
  createLocalConceptSource,
  type ConceptBackend,
  type ConceptDTO,
  type RelationDTO,
} from "../../lib/localConceptSource";

const DAY = 86_400_000;
const NOW = Date.UTC(2026, 9, 14, 12);

export const SAMPLE_CONCEPTS: ConceptDTO[] = [
  {
    id: "c1",
    project_id: "",
    domain: "Commerce",
    definition: "The list of items a shopper has chosen and not yet paid for.",
    source: "terminology",
    terms: [
      { text: "shopping cart", locale: "en-US", status: "preferred", forms: ["shopping carts"] },
      {
        text: "basket",
        locale: "en-US",
        status: "deprecated",
        note: "UK usage; the store copy says cart.",
      },
      { text: "trolley", locale: "en-US", status: "forbidden" },
      { text: "Warenkorb", locale: "de-DE", status: "preferred", gender: "masculine" },
      { text: "panier", locale: "fr-FR", status: "preferred" },
      { text: "handlekurv", locale: "nb-NO", status: "approved" },
    ],
    properties: { profile: "kapimart-store", coordinates: "brand=kapimart,product=store" },
    created_at: new Date(NOW - 21 * DAY).toISOString(),
    updated_at: new Date(NOW - 2 * DAY).toISOString(),
  },
  {
    id: "c2",
    project_id: "",
    domain: "Brand",
    definition: "The company and its store, written as one word.",
    source: "brand_vocabulary",
    terms: [
      { text: "KapiMart", locale: "en-US", status: "preferred" },
      { text: "Kapi Mart", locale: "en-US", status: "forbidden" },
      {
        text: "KapiMart",
        locale: "de-DE",
        status: "approved",
        validity: { tags: { market: "dach" } },
      },
    ],
    created_at: new Date(NOW - 14 * DAY).toISOString(),
    updated_at: new Date(NOW - 14 * DAY).toISOString(),
  },
  {
    id: "c3",
    project_id: "",
    domain: "Commerce",
    definition: "Paying for what is in the shopping cart.",
    source: "terminology",
    terms: [
      { text: "checkout", locale: "en-US", status: "preferred" },
      { text: "Kasse", locale: "de-DE", status: "preferred" },
      { text: "paiement", locale: "fr-FR", status: "admitted" },
      {
        text: "Bezahlung",
        locale: "de-DE",
        status: "proposed",
        validity: { valid_from: new Date(NOW + 30 * DAY).toISOString() },
      },
    ],
    created_at: new Date(NOW - 9 * DAY).toISOString(),
    updated_at: new Date(NOW - DAY).toISOString(),
  },
  {
    id: "c4",
    project_id: "",
    domain: "Legal",
    definition: "A competitor's store.",
    source: "terminology",
    terms: [{ text: "Nimbus Market", locale: "en-US", status: "forbidden", competitor_term: true }],
    created_at: new Date(NOW - 4 * DAY).toISOString(),
    updated_at: new Date(NOW - 3 * DAY).toISOString(),
  },
];

export const SAMPLE_RELATIONS: RelationDTO[] = [
  { id: "r1", source_id: "c3", target_id: "c1", type: "RELATED" },
  { id: "r2", source_id: "c2", target_id: "c4", type: "COMPETITOR", note: "Watch positioning." },
];

/** A store of `count` concepts, for the long-list stories. */
export function manyConcepts(count: number): ConceptDTO[] {
  const locales = ["en-US", "de-DE", "fr-FR", "nb-NO", "ja-JP"];
  return Array.from({ length: count }, (_, i) => ({
    id: `m${i}`,
    project_id: "",
    domain: i % 3 === 0 ? "Commerce" : "Support",
    definition: `Concept number ${i + 1} in a large store.`,
    source: "terminology",
    terms: locales.slice(0, 1 + (i % locales.length)).map((locale, j) => ({
      text: j === 0 ? `term ${String(i + 1).padStart(4, "0")}` : `${locale} ${i + 1}`,
      locale,
      status: j === 0 && i % 7 === 0 ? "approved" : "preferred",
    })),
    created_at: new Date(NOW - (i + 1) * DAY).toISOString(),
    updated_at: new Date(NOW - i * DAY).toISOString(),
  }));
}

/** A mutable in-memory backend, so the relation editing is live in a story. */
export function makeConceptBackend(
  seed: ConceptDTO[] = SAMPLE_CONCEPTS,
  seedRelations: RelationDTO[] = SAMPLE_RELATIONS,
): ConceptBackend {
  const concepts = structuredClone(seed);
  let relations = structuredClone(seedRelations);
  let seq = relations.length;
  const find = (id: string) => concepts.find((c) => c.id === id) ?? null;

  return {
    async searchTerms(_handle, query, _src, _tgt, offset, limit) {
      const q = query.trim().toLowerCase();
      const matched = q
        ? concepts.filter(
            (c) =>
              c.domain.toLowerCase().includes(q) ||
              c.definition.toLowerCase().includes(q) ||
              c.terms.some((t) => t.text.toLowerCase().includes(q)),
          )
        : concepts;
      return { concepts: matched.slice(offset, offset + limit), total_count: matched.length };
    },
    async getConceptForView(_handle, id) {
      const c = find(id);
      return c ? structuredClone(c) : null;
    },
    async getRelations(_handle, conceptId) {
      return relations.filter((r) => r.source_id === conceptId || r.target_id === conceptId);
    },
    async addRelation(_handle, req) {
      const rel: RelationDTO = {
        id: `r${++seq}`,
        source_id: req.source_id,
        target_id: req.target_id,
        type: req.type,
        note: req.note,
      };
      relations.push(rel);
      return rel;
    },
    async removeRelation(_handle, relationID) {
      relations = relations.filter((r) => r.id !== relationID);
    },
    async setTermStatus(_handle, req) {
      const term = find(req.concept_id)?.terms.find(
        (t) => t.locale === req.locale && t.text === req.text,
      );
      if (term) term.status = req.status;
    },
    async updateConcept(_handle, req) {
      const c = find(req.concept_id);
      if (c) {
        c.domain = req.domain;
        c.definition = req.definition;
        c.terms = req.terms;
      }
    },
  };
}

/** The local concept source over an in-memory store. */
export function storyConceptSource(seed?: ConceptDTO[]): ConceptDataSource {
  return createLocalConceptSource("story", makeConceptBackend(seed, seed ? [] : SAMPLE_RELATIONS));
}
