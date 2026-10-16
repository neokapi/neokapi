import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { fn } from "storybook/test";
import { ConceptsSection } from "./ConceptsSection";
import { sampleConcepts, voiceHubOverrides } from "../../stories/voiceHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";
import type { ApiAdapter } from "../../api/adapter";
import type { ConceptInfo } from "../../types/api";
import type { ListConceptsParams } from "../../types/brand-graph";

// The workspace's concepts as a list beside a detail pane, read through the
// REST concept source over the Storybook mock adapter: the same mapping, paging
// and governed-edit handling the web app runs.

const earlier = "2026-09-23T14:00:00Z";
const recent = "2026-10-13T09:30:00Z";

const shoppingCart: ConceptInfo = {
  id: "c-cart",
  domain: "Commerce",
  definition: "The list of items a shopper has chosen and not yet paid for.",
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
  properties: { coordinates: "brand=kapimart,channel=store", profile: "kapimart-store" },
  created_at: earlier,
  updated_at: recent,
};

const payment: ConceptInfo = {
  id: "c-payment",
  domain: "Commerce",
  definition: "Paying for an order at the end of checkout.",
  terms: [
    { text: "payment", locale: "en-US", status: "proposed" },
    { text: "Zahlung", locale: "de-DE", status: "proposed" },
  ],
  created_at: recent,
  updated_at: recent,
};

const storyConcepts: ConceptInfo[] = [shoppingCart, payment, ...sampleConcepts];

const longConcept: ConceptInfo = {
  id: "c-long",
  domain: "Regulatory and compliance communication for financial services",
  definition:
    "The written notice a customer receives before a change to the terms of an account takes effect, including the date the change applies, what it changes, and how the customer can close the account without a fee if they do not accept it.",
  terms: [
    {
      text: "advance notice of a change to the account terms and conditions",
      locale: "en-US",
      status: "preferred",
      note: "Always the full phrase in the first mention; the short form is admitted after it.",
    },
    { text: "change notice", locale: "en-US", status: "admitted" },
    {
      text: "heads-up",
      locale: "en-US",
      status: "forbidden",
      note: "Too casual for a legal notice.",
    },
    ...[
      "de-DE",
      "fr-FR",
      "es-ES",
      "it-IT",
      "nl-NL",
      "sv-SE",
      "da-DK",
      "nb-NO",
      "fi-FI",
      "pl-PL",
      "pt-BR",
      "ja-JP",
    ].map((locale, i) => ({
      text: `Vorankündigung ${i + 1} einer Änderung der Kontobedingungen`,
      locale,
      status: (i % 3 === 0 ? "proposed" : "approved") as "proposed" | "approved",
    })),
  ],
  properties: { coordinates: "brand=northbank,channel=legal,product=current-account" },
  created_at: earlier,
  updated_at: recent,
};

/** A source that answers list reads a page at a time, searched by term text. */
function conceptsSource(concepts: ConceptInfo[]): Partial<ApiAdapter> {
  return {
    listConcepts: async (_ws: string, params?: ListConceptsParams) => {
      let list = concepts;
      if (params?.q) {
        const q = params.q.toLowerCase();
        list = list.filter((c) => c.terms.some((t) => t.text.toLowerCase().includes(q)));
      }
      if (params?.status) {
        list = list.filter((c) => c.terms.some((t) => t.status === params.status));
      }
      const offset = params?.offset ?? 0;
      const limit = params?.limit ?? list.length;
      return { concepts: list.slice(offset, offset + limit), total_count: list.length };
    },
    getConcept: async (_ws: string, id: string) => concepts.find((c) => c.id === id) ?? concepts[0],
    getConceptStatusCounts: async () => ({ total: concepts.length, by_status: {} }),
  };
}

function withConcepts(concepts: ConceptInfo[], extra: Partial<ApiAdapter> = {}): Decorator {
  return createProvidersDecorator(undefined, {
    ...voiceHubOverrides,
    ...conceptsSource(concepts),
    ...extra,
  });
}

const many: ConceptInfo[] = Array.from({ length: 240 }, (_, i) => {
  const n = String(i + 1).padStart(3, "0");
  return {
    id: `c-${n}`,
    domain: "Catalogue",
    definition: `Catalogue concept ${n}.`,
    terms: [
      {
        text: `catalogue term ${n}`,
        locale: "en-US",
        status: i % 7 === 0 ? "proposed" : "preferred",
      },
      { text: `Katalogbegriff ${n}`, locale: "de-DE", status: "approved" },
      ...(i % 2 === 0
        ? [{ text: `terme ${n}`, locale: "fr-FR", status: "approved" as const }]
        : []),
    ],
    created_at: earlier,
    updated_at: recent,
  };
});

const meta: Meta<typeof ConceptsSection> = {
  title: "Context/Concepts/ConceptsSection",
  component: ConceptsSection,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { onSelect: fn(), onOpenChangeSet: fn(), onOpenExperiments: fn() },
  decorators: [
    (Story) => (
      <div style={{ height: 680, padding: 16, display: "flex", flexDirection: "column" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ConceptsSection>;

/** The list with nothing selected, and a change-set waiting above it. */
export const List: Story = {
  decorators: [withConcepts(storyConcepts)],
  globals: { theme: "light" },
};

/** One concept selected: its discouraged forms, languages, provenance and record. */
export const ConceptSelected: Story = {
  args: { defaultSelected: "c-cart" },
  decorators: [withConcepts(storyConcepts)],
  globals: { theme: "light" },
};

export const ConceptSelectedDark: Story = {
  args: { defaultSelected: "c-cart" },
  decorators: [withConcepts(storyConcepts)],
  globals: { theme: "dark" },
};

/** A proposed concept, which offers Approve in its toolbar. */
export const ProposedConcept: Story = {
  args: { defaultSelected: "c-payment" },
  decorators: [withConcepts(storyConcepts)],
  globals: { theme: "light" },
};

/** A workspace holding a single concept. */
export const OneConcept: Story = {
  args: { defaultSelected: "c-cart" },
  decorators: [withConcepts([shoppingCart], { listChangesets: async () => [] })],
  globals: { theme: "light" },
};

/** Hundreds of concepts, read a page at a time with Show more at the foot of the list. */
export const ManyConcepts: Story = {
  args: { defaultSelected: "c-001" },
  decorators: [withConcepts(many, { listChangesets: async () => [] })],
  globals: { theme: "light" },
};

/** A long term, a long definition and a dozen languages. */
export const LongContent: Story = {
  args: { defaultSelected: "c-long" },
  decorators: [withConcepts([longConcept, ...storyConcepts], { listChangesets: async () => [] })],
  globals: { theme: "light" },
};

export const LongContentDark: Story = {
  args: { defaultSelected: "c-long" },
  decorators: [withConcepts([longConcept, ...storyConcepts], { listChangesets: async () => [] })],
  globals: { theme: "dark" },
};

/** A workspace with no concepts and nothing waiting. */
export const Empty: Story = {
  decorators: [withConcepts([], { listChangesets: async () => [] })],
  globals: { theme: "light" },
};

/** Every concept still waiting on a review: the list is empty and the strip names why. */
export const EverythingWaiting: Story = {
  decorators: [withConcepts([])],
  globals: { theme: "light" },
};
