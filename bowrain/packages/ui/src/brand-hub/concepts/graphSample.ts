// A richer brand-graph sample used by the Concepts stories (graph canvas, graph
// view, full concept story). Kept as plain data so the stories can layer it onto
// the mock adapter without bloating the shared brand-hub fixtures.
import type { ConceptInfo } from "../../types/api";
import type {
  Comment as ConceptComment,
  ConceptRelation,
  ConceptStoryEntry,
  GraphViz,
  Market,
  Observation,
} from "../../types/brand-graph";

export const richMarkets: Market[] = [
  {
    id: "m-dach",
    workspace_id: "ws-1",
    name: "dach",
    description: "German-speaking markets",
    locales: ["de-DE", "de-AT", "de-CH"],
    created_at: "2026-05-01T00:00:00Z",
    updated_at: "2026-05-01T00:00:00Z",
  },
  {
    id: "m-france",
    workspace_id: "ws-1",
    name: "france",
    locales: ["fr-FR"],
    created_at: "2026-05-01T00:00:00Z",
    updated_at: "2026-05-01T00:00:00Z",
  },
  {
    id: "m-us",
    workspace_id: "ws-1",
    name: "us",
    locales: ["en-US"],
    created_at: "2026-05-01T00:00:00Z",
    updated_at: "2026-05-01T00:00:00Z",
  },
];

export const richConcepts: ConceptInfo[] = [
  {
    id: "c-commerce",
    domain: "commerce",
    definition: "The umbrella concept for buying and selling on the platform.",
    terms: [{ text: "Commerce", locale: "en-US", status: "approved" }],
    created_at: "2026-05-02T00:00:00Z",
    updated_at: "2026-05-02T00:00:00Z",
  },
  {
    id: "c-checkout",
    domain: "commerce",
    definition: "The flow where a shopper completes a purchase.",
    terms: [
      { text: "Checkout", locale: "en-US", status: "preferred" },
      { text: "Kasse", locale: "de-DE", status: "approved" },
      { text: "Caisse", locale: "fr-FR", status: "deprecated" },
      { text: "Paiement", locale: "fr-FR", status: "proposed" },
    ],
    created_at: "2026-05-02T00:00:00Z",
    updated_at: "2026-06-10T00:00:00Z",
  },
  {
    id: "c-basket",
    domain: "commerce",
    definition: "The collection of items a shopper intends to buy.",
    terms: [
      { text: "Cart", locale: "en-US", status: "preferred" },
      { text: "Basket", locale: "en-GB", status: "admitted" },
      { text: "Warenkorb", locale: "de-DE", status: "approved" },
    ],
    created_at: "2026-05-02T00:00:00Z",
    updated_at: "2026-05-02T00:00:00Z",
  },
  {
    id: "c-payment",
    domain: "commerce",
    definition: "Taking money for an order.",
    terms: [
      { text: "Payment", locale: "en-US", status: "approved" },
      { text: "Zahlung", locale: "de-DE", status: "approved" },
    ],
    created_at: "2026-05-03T00:00:00Z",
    updated_at: "2026-05-03T00:00:00Z",
  },
  {
    id: "c-wallet",
    domain: "commerce",
    definition: "A stored balance a shopper can pay from.",
    terms: [{ text: "Wallet", locale: "en-US", status: "admitted" }],
    created_at: "2026-05-04T00:00:00Z",
    updated_at: "2026-05-04T00:00:00Z",
  },
  {
    id: "c-coupon",
    domain: "marketing",
    definition: "A discount a shopper applies at checkout.",
    terms: [{ text: "Coupon", locale: "en-US", status: "proposed" }],
    created_at: "2026-05-05T00:00:00Z",
    updated_at: "2026-05-05T00:00:00Z",
  },
  {
    id: "c-quickpay",
    domain: "commerce",
    definition: "A competitor's branded checkout we must not use.",
    terms: [{ text: "QuickPay", locale: "en-US", status: "forbidden" }],
    created_at: "2026-05-06T00:00:00Z",
    updated_at: "2026-06-08T00:00:00Z",
  },
];

export const richGraph: GraphViz = {
  nodes: [
    { id: "c-commerce", label: "Commerce", domain: "commerce", status: "approved", term_count: 1 },
    { id: "c-checkout", label: "Checkout", domain: "commerce", status: "preferred", term_count: 4 },
    { id: "c-basket", label: "Cart", domain: "commerce", status: "preferred", term_count: 3 },
    { id: "c-payment", label: "Payment", domain: "commerce", status: "approved", term_count: 2 },
    { id: "c-wallet", label: "Wallet", domain: "commerce", status: "admitted", term_count: 1 },
    { id: "c-coupon", label: "Coupon", domain: "marketing", status: "proposed", term_count: 1 },
    { id: "c-quickpay", label: "QuickPay", domain: "commerce", status: "forbidden", term_count: 1 },
  ],
  edges: [
    { id: "e1", source: "c-commerce", target: "c-checkout", type: "BROADER" },
    { id: "e2", source: "c-commerce", target: "c-basket", type: "BROADER" },
    { id: "e3", source: "c-commerce", target: "c-payment", type: "BROADER" },
    { id: "e4", source: "c-checkout", target: "c-payment", type: "HAS_PART" },
    { id: "e5", source: "c-payment", target: "c-wallet", type: "HAS_PART" },
    { id: "e6", source: "c-checkout", target: "c-basket", type: "RELATED" },
    { id: "e7", source: "c-basket", target: "c-coupon", type: "RELATED" },
    { id: "e8", source: "c-quickpay", target: "c-checkout", type: "COMPETITOR" },
    { id: "e9", source: "c-wallet", target: "c-payment", type: "CLOSE_MATCH" },
  ],
};

export const richRelations: ConceptRelation[] = [
  {
    id: "r1",
    source_id: "c-commerce",
    target_id: "c-checkout",
    relation_type: "BROADER",
    note: "Checkout is a kind of commerce flow.",
    created_at: "2026-05-02T00:00:00Z",
  },
  {
    id: "r2",
    source_id: "c-checkout",
    target_id: "c-payment",
    relation_type: "HAS_PART",
    created_at: "2026-05-03T00:00:00Z",
  },
  {
    id: "r3",
    source_id: "c-checkout",
    target_id: "c-basket",
    relation_type: "RELATED",
    note: "Checkout follows the cart.",
    created_at: "2026-05-02T00:00:00Z",
  },
  {
    id: "r4",
    source_id: "c-quickpay",
    target_id: "c-checkout",
    relation_type: "COMPETITOR",
    note: "A rival's branded checkout.",
    created_at: "2026-06-08T00:00:00Z",
  },
];

export const richStory: ConceptStoryEntry[] = [
  {
    kind: "revision",
    at: "2026-05-02T09:00:00Z",
    actor: "alex",
    ref: "1",
    summary: "Created concept with terms in en-US, de-DE, fr-FR.",
  },
  {
    kind: "observation",
    at: "2026-05-02T14:00:00Z",
    actor: "alex",
    summary: "competitor observation: ‘Breeze through QuickPay checkout in one tap.’",
  },
  {
    kind: "revision",
    at: "2026-05-20T11:00:00Z",
    actor: "sam",
    ref: "2",
    summary: "Added relation HAS_PART → Payment.",
  },
  {
    kind: "comment",
    at: "2026-06-08T10:00:00Z",
    actor: "sam",
    summary: "Should ‘Caisse’ be retired for fr-FR in favour of ‘Paiement’? @alex",
  },
  {
    kind: "changeset",
    at: "2026-06-10T16:00:00Z",
    actor: "sam",
    summary: "change-set ‘Prefer Paiement for fr-FR’ (in_review)",
  },
];

export const richObservations: Observation[] = [
  {
    id: "o1",
    workspace_id: "ws-1",
    concept_id: "c-checkout",
    kind: "competitor",
    quote: "Breeze through QuickPay checkout in one tap.",
    source: "Rival landing page",
    url: "https://example.com",
    market: "us",
    created_by: "alex",
    created_at: "2026-05-02T14:00:00Z",
  },
  {
    id: "o2",
    workspace_id: "ws-1",
    concept_id: "c-checkout",
    kind: "customer",
    quote: "I couldn't find where to pay.",
    source: "Support ticket #4821",
    locale: "en-US",
    created_by: "sam",
    created_at: "2026-05-18T08:00:00Z",
  },
];

export const richComments: ConceptComment[] = [
  {
    id: "cm1",
    workspace_id: "ws-1",
    concept_id: "c-checkout",
    body: "Should ‘Caisse’ be retired for fr-FR in favour of ‘Paiement’? @alex",
    author: "sam",
    created_at: "2026-06-08T10:00:00Z",
    resolved: false,
  },
  {
    id: "cm2",
    workspace_id: "ws-1",
    concept_id: "c-checkout",
    parent_id: "cm1",
    body: "Agreed — marketing has standardised on ‘Paiement’. Let's open an experiment.",
    author: "alex",
    created_at: "2026-06-08T12:00:00Z",
    resolved: false,
  },
  {
    id: "cm3",
    workspace_id: "ws-1",
    concept_id: "c-checkout",
    body: "Confirmed the de-DE term ‘Kasse’ with the DACH reviewer.",
    author: "alex",
    created_at: "2026-05-21T09:00:00Z",
    resolved: true,
  },
];

export const conceptById = (id: string): ConceptInfo =>
  richConcepts.find((c) => c.id === id) ?? richConcepts[0];
