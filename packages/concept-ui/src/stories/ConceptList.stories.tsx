import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConceptList } from "../ConceptList";
import type { ConceptDataSource } from "../adapter";
import type { Concept } from "../types";
import { makeMemorySource } from "./fixtures";

const meta: Meta<typeof ConceptList> = {
  title: "Concept UI/ConceptList",
  component: ConceptList,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { onOpen: fn() },
  decorators: [
    (Story) => (
      <div className="mx-auto max-w-4xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ConceptList>;

/** Full source: status, domain, source, and market filters all available. */
export const Default: Story = {
  args: { source: makeMemorySource() },
};

/** A core-only source (no named markets): the market filter is hidden. */
export const CoreOnly: Story = {
  args: { source: makeMemorySource({ rich: false, editable: false }) },
};

/** Opened on a starting filter. */
export const FilteredToPromotions: Story = {
  args: { source: makeMemorySource(), initialQuery: { domain: "promotions" } },
};

// ── Naming across locales ────────────────────────────────────────────────────

/**
 * Terms listed Arabic first, each preferred in its own locale — the shape a
 * pushed multilingual store arrives in. Which term heads the card is the whole
 * point of these stories.
 */
const MULTILINGUAL: Concept[] = [
  {
    id: "warehouse",
    domain: "fulfilment",
    definition: "Where stock sits between arriving and shipping.",
    terms: [
      { text: "مستودع", locale: "ar", status: "preferred" },
      { text: "Lager", locale: "de-DE", status: "preferred" },
      { text: "Warehouse", locale: "en-US", status: "preferred" },
      { text: "Entrepôt", locale: "fr-FR", status: "preferred" },
    ],
  },
  {
    id: "escalation",
    domain: "support",
    definition: "Handing a case to someone with more authority to settle it.",
    terms: [
      { text: "تصعيد", locale: "ar", status: "preferred" },
      { text: "Eskalation", locale: "de-DE", status: "preferred" },
      { text: "Escalation", locale: "en-US", status: "preferred" },
      { text: "Escalade", locale: "fr-FR", status: "preferred" },
    ],
  },
];

const multilingualSource = (): ConceptDataSource =>
  ({
    listConcepts: () => ({ concepts: MULTILINGUAL, total: MULTILINGUAL.length }),
    getConcept: (id: string) => MULTILINGUAL.find((c) => c.id === id) ?? null,
    getConceptSummary: (id: string) => MULTILINGUAL.find((c) => c.id === id) ?? null,
  }) as unknown as ConceptDataSource;

/** An English-source workspace: each card is headed by its English term. */
export const MultilingualEnglishSource: Story = {
  args: { source: multilingualSource(), naming: { sourceLocale: "en-US" } },
};

export const MultilingualEnglishSourceDark: Story = {
  args: MultilingualEnglishSource.args,
  globals: { theme: "dark" },
};

/** A German-source workspace reads the same concepts under their German terms. */
export const MultilingualGermanSource: Story = {
  args: { source: multilingualSource(), naming: { sourceLocale: "de-DE" } },
};

export const MultilingualGermanSourceDark: Story = {
  args: MultilingualGermanSource.args,
  globals: { theme: "dark" },
};

/** No source locale: the viewer's own language names the concepts. */
export const MultilingualViewerLocale: Story = {
  args: { source: multilingualSource(), naming: { uiLocale: "fr-FR" } },
};

/** No hints at all: English, then a fixed order over the terms. */
export const MultilingualNoHints: Story = {
  args: { source: multilingualSource() },
};
