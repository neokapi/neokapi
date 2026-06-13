// Self-contained fixtures + a focused harness for the ConceptTimeline /
// ConstraintsPanel stories (Apache-2.0). Kept independent of the shared
// fixtures/barrel so these stories render a single panel in isolation against a
// hand-tuned concept with several dated transitions and market-specific bans —
// and against a core-only source to show the framework-only degradation.
import { useMemo } from "react";
import type { ReactNode } from "react";
import { Skeleton } from "@neokapi/ui-primitives";
import type { ConceptDataSource, ConceptListResult, ConceptQuery } from "../adapter";
import { resolveCapabilities } from "../adapter";
import type { ConceptSectionProps } from "../ConceptView";
import { useResource } from "../useResource";
import type { Comment, Concept, Market, Observation, Relation, TimelineEvent } from "../types";

// ── Markets ──────────────────────────────────────────────────────────────────

export const MARKETS: Market[] = [
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

const CHECKOUT: Concept = {
  id: "checkout",
  domain: "commerce",
  source: "brand_vocabulary",
  definition: "The step where a shopper reviews their cart and pays for the order.",
  createdAt: "2025-09-01T09:00:00Z",
  updatedAt: "2026-03-06T16:00:00Z",
  terms: [
    { text: "Checkout", locale: "en-US", status: "preferred" },
    { text: "Checkout", locale: "en-GB", status: "approved", validity: { tags: { market: "uk" } } },
    { text: "Voucher", locale: "en-US", status: "forbidden", note: "Competitor term — never use." },
    {
      text: "Kasse",
      locale: "de-DE",
      status: "preferred",
      validity: { validFrom: "2026-01-01T00:00:00Z", tags: { market: "dach" } },
    },
    {
      text: "Kassa",
      locale: "de-AT",
      status: "approved",
      validity: { validFrom: "2026-01-01T00:00:00Z", tags: { market: "dach" } },
    },
    {
      text: "Paiement",
      locale: "fr-FR",
      status: "preferred",
      validity: { validFrom: "2025-11-03T00:00:00Z", tags: { market: "france" } },
    },
    {
      text: "Validation de commande",
      locale: "fr-FR",
      status: "deprecated",
      note: "Superseded by ‘Paiement’ in the 2025 voice refresh.",
      validity: {
        validFrom: "2025-09-01T00:00:00Z",
        validTo: "2025-11-03T00:00:00Z",
        tags: { market: "france" },
      },
    },
    {
      text: "Offre de paiement",
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
};

const NEIGHBOURS: Concept[] = [
  {
    id: "cart",
    domain: "commerce",
    terms: [{ text: "Cart", locale: "en-US", status: "preferred" }],
  },
  {
    id: "order-summary",
    domain: "commerce",
    terms: [{ text: "Order summary", locale: "en-US", status: "preferred" }],
  },
  {
    id: "promo-code",
    domain: "promotions",
    terms: [{ text: "Promo code", locale: "en-US", status: "approved" }],
  },
];

/** A minimal, undated concept — the floor case (no windows, no markets). */
const PLAIN: Concept = {
  id: "wishlist",
  domain: "commerce",
  source: "brand_vocabulary",
  definition: "A saved list of items a shopper may buy later.",
  terms: [
    { text: "Wishlist", locale: "en-US", status: "preferred" },
    { text: "Saved items", locale: "en-US", status: "forbidden", note: "Legacy label." },
    { text: "Merkliste", locale: "de-DE", status: "preferred" },
  ],
};

const ALL_CONCEPTS = [CHECKOUT, ...NEIGHBOURS, PLAIN];

// ── Relations (one dated, the rest plain) ────────────────────────────────────

const RELATIONS: Relation[] = [
  { id: "r1", sourceId: "checkout", targetId: "cart", type: "RELATED" },
  { id: "r2", sourceId: "checkout", targetId: "order-summary", type: "HAS_PART" },
  {
    id: "r3",
    sourceId: "checkout",
    targetId: "promo-code",
    type: "USE_INSTEAD",
    validity: { validFrom: "2026-01-20T00:00:00Z" },
  },
];

// ── Rich revision log (getTimeline) ──────────────────────────────────────────

const TIMELINE: TimelineEvent[] = [
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
    at: "2025-09-12T11:30:00Z",
    actor: "Léa M.",
    summary: "Added German and French terms",
    ref: "4",
  },
  {
    id: "t3",
    kind: "status",
    at: "2025-11-03T08:00:00Z",
    actor: "Léa M.",
    summary: "Deprecated ‘Validation de commande’ (fr-FR)",
    ref: "7",
  },
  {
    id: "t4",
    kind: "status",
    at: "2025-11-03T08:05:00Z",
    actor: "Léa M.",
    summary: "‘Paiement’ became preferred (fr-FR)",
    ref: "6",
  },
  {
    id: "t5",
    kind: "relation",
    at: "2026-01-20T10:00:00Z",
    actor: "Priya N.",
    summary: "Linked use instead Promo code",
    ref: "r3",
  },
  {
    id: "t6",
    kind: "observation",
    at: "2026-02-10T13:00:00Z",
    actor: "Priya N.",
    summary: "Logged a competitor observation",
    detail: "Proceed to secure checkout",
  },
  {
    id: "t7",
    kind: "comment",
    at: "2026-03-05T14:20:00Z",
    actor: "Priya N.",
    summary: "Priya N. commented",
    detail: "Keep it visible so writers know what replaced it.",
  },
  {
    id: "t8",
    kind: "changeset",
    at: "2026-03-06T16:00:00Z",
    actor: "Release bot",
    summary: "Shipped voice refresh v4",
    ref: "CS-42",
  },
];

const OBSERVATIONS: Observation[] = [
  {
    id: "o1",
    kind: "competitor",
    quote: "Proceed to secure checkout",
    source: "competitor-store.example",
    actor: "Priya N.",
    at: "2026-02-10T13:00:00Z",
  },
];

const COMMENTS: Comment[] = [
  {
    id: "c1",
    body: "Should ‘Validation de commande’ stay listed once deprecated?",
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
];

// ── Source factory ───────────────────────────────────────────────────────────

export interface PanelSourceOptions {
  /** Expose markets/observations/comments/timeline (the platform path). */
  rich?: boolean;
}

/** An in-memory source over the panel fixtures. `rich:false` is the core path. */
export function makePanelSource(opts: PanelSourceOptions = {}): ConceptDataSource {
  const { rich = true } = opts;
  const concepts = structuredClone(ALL_CONCEPTS);
  const relations = structuredClone(RELATIONS);
  const find = (id: string) => concepts.find((c) => c.id === id) ?? null;

  const source: ConceptDataSource = {
    listConcepts: (_query: ConceptQuery): ConceptListResult => ({
      concepts,
      total: concepts.length,
    }),
    getConcept: (id) => find(id),
    getRelations: (conceptId) =>
      relations.filter((r) => r.sourceId === conceptId || r.targetId === conceptId),
    getConceptSummary: (id) => find(id),
  };

  if (rich) {
    source.getMarkets = () => structuredClone(MARKETS);
    source.getObservations = (id) => (id === "checkout" ? structuredClone(OBSERVATIONS) : []);
    source.getComments = (id) => (id === "checkout" ? structuredClone(COMMENTS) : []);
    source.getTimeline = (id) => (id === "checkout" ? structuredClone(TIMELINE) : []);
  }
  return source;
}

// ── Harness: render a single panel against a loaded concept ───────────────────

export function PanelHarness({
  source,
  conceptId,
  render,
}: {
  source: ConceptDataSource;
  conceptId: string;
  render: (ctx: ConceptSectionProps) => ReactNode;
}) {
  const caps = useMemo(() => resolveCapabilities(source), [source]);
  const { data: concept, loading } = useResource(
    () => source.getConcept(conceptId),
    [source, conceptId],
  );

  return (
    <div className="mx-auto max-w-xl p-6">
      {concept ? (
        render({ concept, source, capabilities: caps, onNavigate: () => {} })
      ) : loading ? (
        <Skeleton className="h-64 w-full rounded-xl" />
      ) : (
        <p className="text-sm text-muted-foreground">Concept not found.</p>
      )}
    </div>
  );
}
