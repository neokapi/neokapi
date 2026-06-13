// An in-memory ConceptDataSource for Storybook + manual testing (Apache-2.0).
// A small commerce concept set (the kind of brand vocabulary a kapi project
// carries) with terms across several markets, validity windows, direct
// relations, and the optional rich data. makeMemorySource() can present the
// full rich source or a core-only one, so stories show both the platform
// experience and the framework-only degradation.
import type {
  ConceptCapabilities,
  ConceptDataSource,
  ConceptListResult,
  ConceptQuery,
} from "../adapter";
import type {
  Comment,
  Concept,
  Market,
  Observation,
  Relation,
  RelationInput,
  TermRef,
  TermPatch,
  TermStatus,
  TimelineEvent,
  Validity,
} from "../types";
import { primaryName } from "../concept-meta";

// ── Markets ──────────────────────────────────────────────────────────────────

export const SAMPLE_MARKETS: Market[] = [
  {
    id: "dach",
    name: "DACH",
    description: "Germany, Austria, Switzerland",
    locales: ["de-DE", "de-AT", "de-CH"],
  },
  { id: "france", name: "France", locales: ["fr-FR"] },
  { id: "us", name: "United States", locales: ["en-US"] },
  { id: "uk", name: "United Kingdom", locales: ["en-GB"] },
];

// ── Concepts ─────────────────────────────────────────────────────────────────

export const SAMPLE_CONCEPTS: Concept[] = [
  {
    id: "checkout",
    domain: "commerce",
    source: "brand_vocabulary",
    definition: "The step where a shopper reviews their cart and pays for the order.",
    createdAt: "2025-09-01T09:00:00Z",
    updatedAt: "2026-03-05T14:20:00Z",
    terms: [
      { text: "Checkout", locale: "en-US", status: "preferred" },
      { text: "Checkout", locale: "en-GB", status: "approved" },
      {
        text: "Kasse",
        locale: "de-DE",
        status: "preferred",
        validity: { tags: { market: "dach" } },
      },
      {
        text: "Kassa",
        locale: "de-AT",
        status: "approved",
        validity: { tags: { market: "dach" } },
      },
      {
        text: "Paiement",
        locale: "fr-FR",
        status: "approved",
        validity: { tags: { market: "france" } },
      },
      {
        text: "Validation de commande",
        locale: "fr-FR",
        status: "deprecated",
        note: "Superseded by 'Paiement' in the 2025 voice refresh.",
        validity: { validTo: "2025-11-03T00:00:00Z", tags: { market: "france" } },
      },
    ],
  },
  {
    id: "express-checkout",
    domain: "commerce",
    source: "brand_vocabulary",
    definition: "A one-tap checkout that skips the cart review for returning shoppers.",
    createdAt: "2025-10-20T09:00:00Z",
    updatedAt: "2025-10-20T09:00:00Z",
    terms: [
      { text: "Express checkout", locale: "en-US", status: "preferred" },
      { text: "Schnellkasse", locale: "de-DE", status: "approved" },
    ],
  },
  {
    id: "cart",
    domain: "commerce",
    source: "brand_vocabulary",
    definition: "The collection of items a shopper has chosen but not yet bought.",
    createdAt: "2025-09-01T09:05:00Z",
    updatedAt: "2026-01-12T11:00:00Z",
    terms: [
      { text: "Cart", locale: "en-US", status: "preferred" },
      {
        text: "Basket",
        locale: "en-GB",
        status: "preferred",
        validity: { tags: { market: "uk" } },
      },
      {
        text: "Warenkorb",
        locale: "de-DE",
        status: "preferred",
        validity: { tags: { market: "dach" } },
      },
      {
        text: "Panier",
        locale: "fr-FR",
        status: "preferred",
        validity: { tags: { market: "france" } },
      },
    ],
  },
  {
    id: "coupon",
    domain: "promotions",
    source: "brand_vocabulary",
    definition: "A code a shopper enters at checkout for a discount.",
    createdAt: "2025-09-14T09:00:00Z",
    updatedAt: "2026-02-01T08:30:00Z",
    terms: [
      { text: "Coupon", locale: "en-US", status: "preferred" },
      {
        text: "Voucher",
        locale: "en-US",
        status: "forbidden",
        note: "Competitor term — never use.",
      },
      {
        text: "Gutschein",
        locale: "de-DE",
        status: "preferred",
        validity: { tags: { market: "dach" } },
      },
      {
        text: "Code promo",
        locale: "fr-FR",
        status: "preferred",
        validity: { tags: { market: "france" } },
      },
      {
        text: "Bon de réduction",
        locale: "fr-FR",
        status: "admitted",
        note: "Seasonal campaign term.",
        validity: {
          validFrom: "2025-11-01T00:00:00Z",
          validTo: "2026-01-15T00:00:00Z",
          tags: { market: "france" },
        },
      },
    ],
  },
  {
    id: "promo-code",
    domain: "promotions",
    source: "terminology",
    definition: "An alphanumeric code that unlocks a promotion.",
    createdAt: "2025-09-14T09:10:00Z",
    updatedAt: "2025-09-14T09:10:00Z",
    terms: [
      { text: "Promo code", locale: "en-US", status: "approved" },
      { text: "Aktionscode", locale: "de-DE", status: "approved" },
    ],
  },
  {
    id: "shipping",
    domain: "fulfilment",
    source: "terminology",
    definition: "Delivery of an order to the shopper.",
    createdAt: "2025-09-02T09:00:00Z",
    updatedAt: "2025-12-03T16:00:00Z",
    terms: [
      { text: "Shipping", locale: "en-US", status: "preferred" },
      {
        text: "Delivery",
        locale: "en-GB",
        status: "approved",
        validity: { tags: { market: "uk" } },
      },
      {
        text: "Versand",
        locale: "de-DE",
        status: "preferred",
        validity: { tags: { market: "dach" } },
      },
      {
        text: "Livraison",
        locale: "fr-FR",
        status: "preferred",
        validity: { tags: { market: "france" } },
      },
    ],
  },
  {
    id: "payment-method",
    domain: "commerce",
    source: "terminology",
    definition: "How a shopper pays — card, wallet, or invoice.",
    createdAt: "2025-09-02T09:05:00Z",
    updatedAt: "2025-09-02T09:05:00Z",
    terms: [
      { text: "Payment method", locale: "en-US", status: "preferred" },
      { text: "Zahlungsart", locale: "de-DE", status: "preferred" },
      { text: "Moyen de paiement", locale: "fr-FR", status: "approved" },
    ],
  },
  {
    id: "order-summary",
    domain: "commerce",
    source: "terminology",
    definition: "The review panel listing items, totals, and taxes before payment.",
    createdAt: "2025-09-02T09:08:00Z",
    updatedAt: "2025-09-02T09:08:00Z",
    terms: [
      { text: "Order summary", locale: "en-US", status: "preferred" },
      { text: "Bestellübersicht", locale: "de-DE", status: "approved" },
    ],
  },
  {
    id: "address",
    domain: "commerce",
    source: "terminology",
    definition: "The shipping or billing address on an order.",
    createdAt: "2025-09-02T09:12:00Z",
    updatedAt: "2025-09-02T09:12:00Z",
    terms: [
      { text: "Address", locale: "en-US", status: "preferred" },
      { text: "Adresse", locale: "de-DE", status: "preferred" },
    ],
  },
  {
    id: "wishlist",
    domain: "commerce",
    source: "brand_vocabulary",
    definition: "A saved list of items a shopper may buy later.",
    createdAt: "2025-09-20T09:00:00Z",
    updatedAt: "2025-11-18T10:00:00Z",
    terms: [
      { text: "Wishlist", locale: "en-US", status: "preferred" },
      {
        text: "Merkliste",
        locale: "de-DE",
        status: "preferred",
        validity: { tags: { market: "dach" } },
      },
      {
        text: "Wunschliste",
        locale: "de-DE",
        status: "admitted",
        validity: { tags: { market: "dach" } },
      },
      {
        text: "Liste de souhaits",
        locale: "fr-FR",
        status: "approved",
        validity: { tags: { market: "france" } },
      },
    ],
  },
];

// ── Relations ────────────────────────────────────────────────────────────────

export const SAMPLE_RELATIONS: Relation[] = [
  { id: "r1", sourceId: "checkout", targetId: "express-checkout", type: "BROADER" },
  { id: "r2", sourceId: "checkout", targetId: "order-summary", type: "HAS_PART" },
  { id: "r3", sourceId: "checkout", targetId: "cart", type: "RELATED" },
  { id: "r4", sourceId: "checkout", targetId: "coupon", type: "RELATED" },
  { id: "r5", sourceId: "checkout", targetId: "shipping", type: "RELATED" },
  { id: "r6", sourceId: "checkout", targetId: "payment-method", type: "RELATED" },
  { id: "r7", sourceId: "checkout", targetId: "address", type: "RELATED" },
  { id: "r8", sourceId: "coupon", targetId: "promo-code", type: "USE_INSTEAD" },
  { id: "r9", sourceId: "cart", targetId: "wishlist", type: "RELATED" },
];

// ── Optional rich data ───────────────────────────────────────────────────────

const SAMPLE_OBSERVATIONS: Record<string, Observation[]> = {
  checkout: [
    {
      id: "o1",
      kind: "competitor",
      quote: "Proceed to secure checkout",
      source: "competitor-store.example",
      url: "https://competitor-store.example/cart",
      locale: "en-US",
      actor: "Priya N.",
      at: "2026-02-10T13:00:00Z",
      note: "They lead with 'secure' — worth A/B testing trust language.",
    },
    {
      id: "o2",
      kind: "style_guide",
      quote: "Always lowercase 'checkout' mid-sentence.",
      source: "Brand voice guide v4",
      locale: "en-US",
      actor: "Style council",
      at: "2025-12-01T09:00:00Z",
    },
  ],
};

const SAMPLE_COMMENTS: Record<string, Comment[]> = {
  checkout: [
    {
      id: "c1",
      body: "Should 'Validation de commande' stay listed once deprecated?",
      author: "Léa M.",
      at: "2026-03-04T10:00:00Z",
      resolved: true,
    },
    {
      id: "c2",
      parentId: "c1",
      body: "Yes — keep it visible so writers know what replaced it.",
      author: "Priya N.",
      at: "2026-03-05T14:20:00Z",
    },
  ],
};

const SAMPLE_TIMELINE: Record<string, TimelineEvent[]> = {
  checkout: [
    {
      id: "t1",
      kind: "create",
      at: "2025-09-01T09:00:00Z",
      actor: "Priya N.",
      summary: "Created concept",
      ref: "1",
    },
    {
      id: "t2",
      kind: "revision",
      at: "2025-10-12T11:30:00Z",
      actor: "Léa M.",
      summary: "Added French and German terms",
      ref: "4",
    },
    {
      id: "t3",
      kind: "status",
      at: "2025-11-03T08:00:00Z",
      actor: "Léa M.",
      summary: "Deprecated ‘Validation de commande’ (fr-FR)",
      ref: "5",
    },
    {
      id: "t4",
      kind: "observation",
      at: "2026-02-10T13:00:00Z",
      actor: "Priya N.",
      summary: "Logged a competitor observation",
    },
    {
      id: "t5",
      kind: "comment",
      at: "2026-03-05T14:20:00Z",
      actor: "Priya N.",
      summary: "Replied in discussion",
      detail: "Yes — keep it visible so writers know what replaced it.",
    },
  ],
};

// ── Source factory ───────────────────────────────────────────────────────────

export interface MemorySourceOptions {
  /** Expose markets/observations/comments/timeline/where-used. Default true. */
  rich?: boolean;
  /** Expose the editable-core mutations. Default true. */
  editable?: boolean;
  /** Override the derived capabilities. */
  capabilities?: Partial<ConceptCapabilities>;
}

function localesForMarket(name: string): string[] {
  const m = SAMPLE_MARKETS.find((x) => x.name === name || x.id === name);
  return m?.locales ?? [];
}

function matchesQuery(concept: Concept, q: ConceptQuery): boolean {
  if (q.source && concept.source !== q.source) return false;
  if (q.domain && concept.domain !== q.domain) return false;
  if (q.status && !concept.terms.some((t) => t.status === q.status)) return false;
  if (q.locale && !concept.terms.some((t) => t.locale === q.locale)) return false;
  if (q.market) {
    const locales = localesForMarket(q.market);
    if (!concept.terms.some((t) => locales.includes(t.locale))) return false;
  }
  if (q.text) {
    const needle = q.text.toLowerCase();
    const hay = [
      primaryName(concept),
      concept.domain ?? "",
      concept.definition ?? "",
      ...concept.terms.map((t) => t.text),
    ]
      .join(" ")
      .toLowerCase();
    if (!hay.includes(needle)) return false;
  }
  return true;
}

/**
 * Build an in-memory ConceptDataSource over a (cloned) copy of the sample set.
 * Mutations affect only this instance, so each story starts clean.
 */
export function makeMemorySource(opts: MemorySourceOptions = {}): ConceptDataSource {
  const { rich = true, editable = true, capabilities } = opts;
  const concepts: Concept[] = structuredClone(SAMPLE_CONCEPTS);
  const relations: Relation[] = structuredClone(SAMPLE_RELATIONS);
  let relSeq = relations.length;

  const find = (id: string) => concepts.find((c) => c.id === id) ?? null;

  const source: ConceptDataSource = {
    listConcepts(query: ConceptQuery): ConceptListResult {
      const matched = concepts.filter((c) => matchesQuery(c, query));
      return { concepts: matched, total: matched.length };
    },
    getConcept: (id) => find(id),
    getRelations: (conceptId) =>
      relations.filter((r) => r.sourceId === conceptId || r.targetId === conceptId),
    getConceptSummary: (id) => find(id),
  };

  if (rich) {
    source.getMarkets = () => structuredClone(SAMPLE_MARKETS);
    source.getObservations = (id) => structuredClone(SAMPLE_OBSERVATIONS[id] ?? []);
    source.getComments = (id) => structuredClone(SAMPLE_COMMENTS[id] ?? []);
    source.getTimeline = (id) => structuredClone(SAMPLE_TIMELINE[id] ?? []);
    source.getWhereUsed = (id) => {
      const c = find(id);
      if (!c) return null;
      const factor = c.terms.length;
      return { conceptId: id, blocks: factor * 6, occurrences: factor * 11, words: factor * 140 };
    };
  }

  if (editable) {
    source.addRelation = (conceptId: string, input: RelationInput): Relation => {
      const rel: Relation = {
        id: `r${++relSeq}`,
        sourceId: conceptId,
        targetId: input.targetId,
        type: input.type,
        note: input.note,
        validity: input.validity,
      };
      relations.push(rel);
      return rel;
    };
    source.removeRelation = (relationId: string) => {
      const i = relations.findIndex((r) => r.id === relationId);
      if (i >= 0) relations.splice(i, 1);
    };
    source.updateTerm = (conceptId: string, ref: TermRef, patch: TermPatch) => {
      const term = find(conceptId)?.terms.find(
        (t) => t.locale === ref.locale && t.text === ref.text,
      );
      if (!term) return;
      if (patch.text !== undefined) term.text = patch.text;
      if (patch.status !== undefined) term.status = patch.status;
      if (patch.note !== undefined) term.note = patch.note;
      if (patch.validity !== undefined) term.validity = patch.validity;
    };
    source.setTermStatus = (
      conceptId: string,
      ref: TermRef,
      status: TermStatus,
      validity?: Validity,
    ) => {
      const term = find(conceptId)?.terms.find(
        (t) => t.locale === ref.locale && t.text === ref.text,
      );
      if (!term) return;
      term.status = status;
      if (validity) term.validity = validity;
    };
  }

  if (capabilities) source.capabilities = capabilities;
  return source;
}
