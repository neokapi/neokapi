// Shared Storybook fixtures + a providers decorator for the Brand hub stories
// (AD-021). Lives under src/stories (excluded from the package typecheck) so the
// brand mock can satisfy just the methods the hub views call. The bowrain
// Storybook still discovers the *.stories.tsx files that import from here.
import type { Decorator } from "@storybook/react";
import type { ApiAdapter } from "../api/adapter";
import type { ConceptInfo, Membership, TermSearchResult } from "../types/api";
import type {
  ConceptRelation,
  Observation,
  Comment as ConceptComment,
  ConceptStory,
  ConceptUsage,
  Market,
  ChangeSet,
  ChangeSetDetail,
  ChangeSetImpact,
} from "../types/brand-graph";
import { createProvidersDecorator } from "./decorators";

const now = "2026-06-13T10:00:00Z";
const earlier = "2026-06-01T09:00:00Z";

export const sampleConcepts: ConceptInfo[] = [
  {
    id: "c-checkout",
    domain: "commerce",
    definition: "The flow where a shopper completes a purchase.",
    terms: [
      { text: "Checkout", locale: "en-US", status: "preferred" },
      { text: "Kasse", locale: "de-DE", status: "approved" },
      { text: "Caisse", locale: "fr-FR", status: "proposed" },
    ],
    created_at: earlier,
    updated_at: now,
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
    created_at: earlier,
    updated_at: earlier,
  },
  {
    id: "c-rival",
    domain: "commerce",
    definition: "A competitor's product name we must not use.",
    terms: [{ text: "QuickPay", locale: "en-US", status: "forbidden" }],
    created_at: earlier,
    updated_at: now,
  },
];

const conceptsResult: TermSearchResult = {
  concepts: sampleConcepts,
  total_count: sampleConcepts.length,
};

const sampleRelations: ConceptRelation[] = [
  {
    id: "r-1",
    source_id: "c-checkout",
    target_id: "c-basket",
    relation_type: "RELATED",
    note: "Checkout follows the cart.",
    created_at: earlier,
  },
  {
    id: "r-2",
    source_id: "c-rival",
    target_id: "c-checkout",
    relation_type: "COMPETITOR",
    created_at: earlier,
  },
];

const sampleObservations: Observation[] = [
  {
    id: "o-1",
    workspace_id: "ws-1",
    concept_id: "c-checkout",
    kind: "competitor",
    quote: "Breeze through QuickPay checkout in one tap.",
    source: "Rival landing page",
    url: "https://example.com",
    market: "us",
    created_by: "alex",
    created_at: earlier,
  },
];

const sampleComments: ConceptComment[] = [
  {
    id: "cm-1",
    workspace_id: "ws-1",
    concept_id: "c-checkout",
    body: "Should ‘Caisse’ be preferred for fr-FR rather than proposed?",
    author: "sam",
    created_at: earlier,
    resolved: false,
  },
  {
    id: "cm-2",
    workspace_id: "ws-1",
    concept_id: "c-checkout",
    parent_id: "cm-1",
    body: "Marketing prefers ‘Paiement’. Let's open an experiment.",
    author: "alex",
    created_at: now,
    resolved: false,
  },
];

const sampleStory: ConceptStory = {
  concept_id: "c-checkout",
  entries: [
    { kind: "revision", at: earlier, actor: "alex", summary: "Created concept with 3 terms." },
    { kind: "observation", at: earlier, actor: "alex", summary: "Recorded a competitor phrasing." },
    { kind: "comment", at: now, actor: "sam", summary: "Asked about the fr-FR status." },
    { kind: "changeset", at: now, actor: "sam", summary: "Opened experiment ‘Prefer Paiement’." },
  ],
};

const sampleConceptUsage: ConceptUsage = {
  concept_id: "c-checkout",
  total_blocks: 1280,
  blocks: 96,
  occurrences: 142,
  words: 410,
  projects: [
    {
      project_id: "p-web",
      project_name: "Marketing Website",
      blocks: 61,
      occurrences: 88,
      words: 250,
      collections: [],
    },
    {
      project_id: "p-app",
      project_name: "Mobile App",
      blocks: 35,
      occurrences: 54,
      words: 160,
      collections: [],
    },
  ],
  samples: [],
};

const sampleMarkets: Market[] = [
  {
    id: "m-dach",
    workspace_id: "ws-1",
    name: "dach",
    description: "German-speaking markets",
    locales: ["de-DE", "de-AT", "de-CH"],
    created_at: earlier,
    updated_at: earlier,
  },
  {
    id: "m-us",
    workspace_id: "ws-1",
    name: "us",
    locales: ["en-US"],
    created_at: earlier,
    updated_at: earlier,
  },
];

export const sampleChangesets: ChangeSet[] = [
  {
    id: "cs-1",
    workspace_id: "ws-1",
    name: "Prefer ‘Paiement’ for fr-FR",
    description: "Promote the marketing-preferred term and retire the proposed one.",
    status: "in_review",
    created_by: "sam",
    created_at: earlier,
    updated_at: now,
    submitted_at: now,
  },
  {
    id: "cs-2",
    workspace_id: "ws-1",
    name: "Ban competitor name in DACH",
    status: "draft",
    created_by: "alex",
    created_at: earlier,
    updated_at: earlier,
  },
  {
    id: "cs-3",
    workspace_id: "ws-1",
    name: "Standardise ‘Cart’ across locales",
    status: "merged",
    created_by: "alex",
    created_at: earlier,
    updated_at: earlier,
    merged_at: earlier,
    merged_by: "sam",
  },
];

const sampleChangesetDetail: ChangeSetDetail = {
  ...sampleChangesets[0],
  governed: true,
  ops: [
    {
      workspace_id: "ws-1",
      changeset_id: "cs-1",
      seq: 1,
      op: "term.status",
      payload: {
        concept_id: "c-checkout",
        locale: "fr-FR",
        text: "Paiement",
        from: "proposed",
        to: "preferred",
      },
      base_rev: 4,
      created_by: "sam",
      created_at: now,
    },
    {
      workspace_id: "ws-1",
      changeset_id: "cs-1",
      seq: 2,
      op: "term.remove",
      payload: { concept_id: "c-checkout", locale: "fr-FR", text: "Caisse" },
      base_rev: 4,
      created_by: "sam",
      created_at: now,
    },
  ],
  reviews: [
    {
      workspace_id: "ws-1",
      changeset_id: "cs-1",
      reviewer: "alex",
      verdict: "approve",
      comment: "Agreed, matches the brand book.",
      created_at: now,
    },
  ],
  pilots: [
    {
      workspace_id: "ws-1",
      changeset_id: "cs-1",
      project_id: "p-web",
      stream: "main",
      created_by: "sam",
      created_at: now,
    },
  ],
};

const sampleImpact: ChangeSetImpact = {
  total_blocks: 1280,
  affected_blocks: 34,
  new_violations: 12,
  resolved: 7,
  words: 210,
  projects: [
    {
      project_id: "p-web",
      project_name: "Marketing Website",
      affected_blocks: 22,
      new_violations: 8,
      resolved: 5,
      words: 140,
      collections: [],
    },
    {
      project_id: "p-app",
      project_name: "Mobile App",
      affected_blocks: 12,
      new_violations: 4,
      resolved: 2,
      words: 70,
      collections: [],
    },
  ],
  samples: [],
};

// The actors that author fixture change-sets, reviews, observations, and
// comments, mapped to display names so the hub renders people, not raw user ids.
const sampleMembers: Membership[] = [
  {
    user_id: "you",
    workspace_id: "ws-1",
    role: "owner",
    user: { id: "you", email: "you@acme.example", name: "You", avatar_url: "" },
  },
  {
    user_id: "sam",
    workspace_id: "ws-1",
    role: "member",
    user: { id: "sam", email: "sam@acme.example", name: "Sam Okafor", avatar_url: "" },
  },
  {
    user_id: "alex",
    workspace_id: "ws-1",
    role: "admin",
    user: { id: "alex", email: "alex@acme.example", name: "Alex Romero", avatar_url: "" },
  },
];

/** Brand-hub method overrides layered onto the base Storybook mock adapter. */
export const brandHubOverrides: Partial<ApiAdapter> = {
  listMembers: async () => sampleMembers,
  listConcepts: async (_ws, params) => {
    let list = sampleConcepts;
    if (params?.status) list = list.filter((c) => c.terms.some((t) => t.status === params.status));
    if (params?.q) {
      const q = params.q.toLowerCase();
      list = list.filter(
        (c) =>
          c.terms.some((t) => t.text.toLowerCase().includes(q)) ||
          c.definition.toLowerCase().includes(q),
      );
    }
    return { concepts: list, total_count: list.length };
  },
  getConcept: async (_ws, id) => sampleConcepts.find((c) => c.id === id) ?? sampleConcepts[0],
  createConcept: async (_ws, req) => ({
    id: "c-new",
    domain: req.domain,
    definition: req.definition,
    terms: req.terms,
    created_at: now,
    updated_at: now,
  }),
  getConceptStory: async () => sampleStory,
  listConceptRelations: async () => sampleRelations,
  addConceptRelation: async (_ws, _id, req) => ({
    id: "r-new",
    source_id: "c-checkout",
    target_id: req.target_id,
    relation_type: req.relation_type,
    note: req.note,
    created_at: now,
  }),
  deleteConceptRelation: async () => {},
  getConceptBlastRadius: async () => sampleConceptUsage,
  listObservations: async () => sampleObservations,
  addObservation: async (_ws, id, req) => ({
    id: "o-new",
    workspace_id: "ws-1",
    concept_id: id,
    created_by: "you",
    created_at: now,
    ...req,
  }),
  deleteObservation: async () => {},
  listConceptComments: async () => sampleComments,
  addConceptComment: async (_ws, id, req) => ({
    id: "cm-new",
    workspace_id: "ws-1",
    concept_id: id,
    body: req.body,
    author: "you",
    created_at: now,
    resolved: false,
  }),
  resolveConceptComment: async () => {},
  deleteConceptComment: async () => {},
  listMarkets: async () => sampleMarkets,
  listChangesets: async (_ws, status) =>
    status ? sampleChangesets.filter((c) => c.status === status) : sampleChangesets,
  getChangeset: async () => sampleChangesetDetail,
  createChangeset: async (_ws, req) => ({
    id: "cs-new",
    workspace_id: "ws-1",
    name: req.name,
    description: req.description,
    status: "draft",
    created_by: "you",
    created_at: now,
    updated_at: now,
  }),
  getChangesetBlastRadius: async () => sampleImpact,
  listBrandProfiles: async () => [],
};

/** Decorator wiring the base mock + brand-hub overrides + a wide container. */
export const withBrandHub: Decorator = createProvidersDecorator(undefined, brandHubOverrides);
