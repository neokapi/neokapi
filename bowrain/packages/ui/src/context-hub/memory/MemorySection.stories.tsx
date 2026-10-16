import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { MemorySection } from "./MemorySection";
import { voiceHubOverrides } from "../../stories/voiceHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";
import type { MemoryEntryInfo, ProjectInfo } from "../../types/api";

// The workspace's content memory as pairs beside a detail pane, over the
// Storybook mock adapter. The server reads each entry as a pair into one target
// language, so the fixture answers the same way: a search, a target language,
// and a page at a time.

const projects: ProjectInfo[] = [
  {
    id: "p-web",
    name: "Marketing Website",
    default_source_language: "en-US",
    target_languages: ["de-DE", "fr-FR", "nb-NO"],
    created_at: "2026-06-01T00:00:00Z",
    modified_at: "2026-06-01T00:00:00Z",
  },
  {
    id: "p-app",
    name: "Mobile App",
    default_source_language: "en-US",
    target_languages: ["de-DE"],
    created_at: "2026-06-01T00:00:00Z",
    modified_at: "2026-06-01T00:00:00Z",
  },
];

const recent = "2026-10-13T09:30:00Z";

const PAIRS: Array<[string, Record<string, string>, string?]> = [
  [
    "Proceed to checkout",
    { "de-DE": "Weiter zur Kasse", "fr-FR": "Passer à la caisse", "nb-NO": "Gå til kassen" },
    "p-web",
  ],
  [
    "Your cart is empty",
    {
      "de-DE": "Ihr Warenkorb ist leer",
      "fr-FR": "Votre panier est vide",
      "nb-NO": "Handlekurven er tom",
    },
  ],
  [
    "Sign in to see your orders",
    { "de-DE": "Melden Sie sich an, um Ihre Bestellungen zu sehen" },
    "p-app",
  ],
  [
    "Free returns within 30 days",
    {
      "de-DE": "Kostenlose Rücksendung innerhalb von 30 Tagen",
      "fr-FR": "Retours gratuits sous 30 jours",
    },
  ],
];

function entriesFrom(pairs: typeof PAIRS): MemoryEntryInfo[] {
  return pairs.flatMap(([source, targets, project], i) =>
    Object.entries(targets).map(([locale, target]) => ({
      id: `e-${i + 1}`,
      source,
      target,
      source_language: "en-US",
      target_language: locale,
      project_id: project,
      stream: i === 3 ? "release-2026-q4" : undefined,
      updated_at: recent,
    })),
  );
}

/** A memory that answers the way the server does: a search into one target language, paged. */
function withMemory(
  entries: MemoryEntryInfo[],
  extra: { projects?: ProjectInfo[] } = {},
): Decorator {
  return createProvidersDecorator(undefined, {
    ...voiceHubOverrides,
    listProjects: async () => extra.projects ?? projects,
    getMemoryCount: async () => new Set(entries.map((e) => e.id)).size,
    getMemoryEntries: async (_ws, query, _source, target, offset, limit) => {
      const q = query.toLowerCase();
      const list = entries.filter(
        (e) =>
          (!target || e.target_language === target) &&
          (!q || e.source.toLowerCase().includes(q) || e.target.toLowerCase().includes(q)),
      );
      return { entries: list.slice(offset, offset + limit), total_count: list.length };
    },
  });
}

const sample = entriesFrom(PAIRS);

const many: MemoryEntryInfo[] = Array.from({ length: 320 }, (_, i) => ({
  id: `e-many-${i}`,
  source: `Catalogue line ${i + 1}: the product description a shopper reads first`,
  target: `Katalogzeile ${i + 1}: die Produktbeschreibung, die ein Käufer zuerst liest`,
  source_language: "en-US",
  target_language: "de-DE",
  project_id: i % 3 === 0 ? "p-web" : undefined,
  updated_at: recent,
}));

const long: MemoryEntryInfo[] = [
  {
    id: "e-long",
    source:
      "Before a change to the terms of your account takes effect, we write to you at least sixty days ahead with the date it applies, what changes, and how you can close the account without a fee if you do not accept the new terms.",
    target:
      "Bevor eine Änderung der Bedingungen Ihres Kontos wirksam wird, schreiben wir Ihnen mindestens sechzig Tage im Voraus mit dem Datum, ab dem sie gilt, was sich ändert und wie Sie das Konto ohne Gebühr schließen können, wenn Sie die neuen Bedingungen nicht annehmen.",
    source_language: "en-US",
    target_language: "de-DE",
    project_id: "p-web",
    stream: "legal-review-2026-with-an-unusually-long-stream-name",
    updated_at: recent,
  },
  ...sample,
];

const meta: Meta<typeof MemorySection> = {
  title: "Context/Memory/MemorySection",
  component: MemorySection,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div style={{ height: 640, padding: 16, display: "flex", flexDirection: "column" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof MemorySection>;

/** The pairs into German, the language most of the workspace translates into. */
export const Pairs: Story = {
  decorators: [withMemory(sample)],
  globals: { theme: "light" },
};

/** One pair selected: both sides in full, and the project it is held for. */
export const PairSelected: Story = {
  args: { defaultSelected: "e-1::de-DE" },
  decorators: [withMemory(sample)],
  globals: { theme: "light" },
};

export const PairSelectedDark: Story = {
  args: { defaultSelected: "e-1::de-DE" },
  decorators: [withMemory(sample)],
  globals: { theme: "dark" },
};

/** Two entries marked: the pane holds what can be done to both. */
export const Marked: Story = {
  args: { defaultMarked: ["e-1", "e-2"] },
  decorators: [withMemory(sample)],
  globals: { theme: "light" },
};

/** A workspace that translates into one language, so the picker is not offered. */
export const OneEntry: Story = {
  args: { defaultSelected: "e-1::de-DE" },
  decorators: [
    withMemory(
      sample.filter((e) => e.id === "e-1" && e.target_language === "de-DE"),
      {
        projects: [projects[1]],
      },
    ),
  ],
  globals: { theme: "light" },
};

/** Hundreds of pairs, read a page at a time with Show more at the foot of the list. */
export const ManyEntries: Story = {
  decorators: [withMemory(many)],
  globals: { theme: "light" },
};

/** A long sentence on both sides and a long stream name. */
export const LongContent: Story = {
  args: { defaultSelected: "e-long::de-DE" },
  decorators: [withMemory(long)],
  globals: { theme: "light" },
};

/** Nothing approved into the language yet: the empty state offers Add entry. */
export const Empty: Story = {
  decorators: [withMemory([])],
  globals: { theme: "light" },
};
