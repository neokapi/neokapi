import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoriesPage } from "../components/MemoriesPage";
import { SAMPLE_ENTRIES, SIX_LANGUAGES, makeMemoryAdapter, manyEntries } from "./fixtures/memory";

/**
 * Content memory: one row per source wording, with its languages as pills,
 * beside a detail pane with each translation in full and where the entry came
 * from. With no store open, the named stores are listed one row each.
 */
const meta: Meta<typeof MemoriesPage> = {
  title: "Pages/MemoriesPage",
  component: MemoriesPage,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div className="h-screen bg-background">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof MemoriesPage>;

/** The project's content memory, open, with nothing selected yet. */
export const OpenStore: Story = {
  render: () => (
    <MemoriesPage
      store={{
        name: "Project Content Memory",
        adapter: makeMemoryAdapter(),
        count: SAMPLE_ENTRIES.length,
      }}
    />
  ),
};

/** An entry selected: inline formatting as chips, its provenance and unit. */
export const PairSelected: Story = {
  render: () => (
    <MemoriesPage
      store={{
        name: "Project Content Memory",
        adapter: makeMemoryAdapter(),
        count: SAMPLE_ENTRIES.length,
        defaultSelected: "2",
      }}
    />
  ),
};

/** The same in the dark theme. */
export const PairSelectedDark: Story = { ...PairSelected, globals: { theme: "dark" } };

/** An entry held in three languages: one row, and each translation in the detail. */
export const ThreeLanguages: Story = {
  render: () => (
    <MemoriesPage
      store={{
        name: "Project Content Memory",
        adapter: makeMemoryAdapter(),
        count: SAMPLE_ENTRIES.length,
        defaultSelected: "1",
      }}
    />
  ),
};

/** A wording in six languages: three pills on the row, the rest counted. */
export const ManyLanguages: Story = {
  render: () => (
    <MemoriesPage
      store={{
        name: "Project Content Memory",
        adapter: makeMemoryAdapter([SIX_LANGUAGES, ...SAMPLE_ENTRIES]),
        count: SAMPLE_ENTRIES.length + 1,
        defaultSelected: SIX_LANGUAGES.id,
      }}
    />
  ),
};

/** Narrowed by a facet from the filter bar: the entries with inline codes. */
export const FilteredByFacet: Story = {
  render: () => (
    <MemoriesPage
      store={{
        name: "Project Content Memory",
        adapter: makeMemoryAdapter(),
        count: SAMPLE_ENTRIES.length,
        initial: { tokens: [{ key: "codes", value: "with" }] },
      }}
    />
  ),
};

/** Two entries marked: the detail pane holds what can be done to both. */
export const EntriesMarked: Story = {
  render: () => (
    <MemoriesPage
      store={{
        name: "Project Content Memory",
        adapter: makeMemoryAdapter(),
        count: SAMPLE_ENTRIES.length,
        initial: { marked: ["1", "2"] },
      }}
    />
  ),
};

/** The same in the dark theme. */
export const EntriesMarkedDark: Story = { ...EntriesMarked, globals: { theme: "dark" } };

/** Find similar: the matches for a sentence, best first, with their scores. */
export const FindSimilar: Story = {
  render: () => (
    <MemoriesPage
      store={{
        name: "Project Content Memory",
        adapter: makeMemoryAdapter(),
        count: SAMPLE_ENTRIES.length,
        initial: { lookup: "Proceed to the checkout" },
      }}
    />
  ),
};

/** A store of five hundred entries: the list reads a page at a time. */
export const ManyEntries: Story = {
  render: () => (
    <MemoriesPage
      store={{
        name: "Project Content Memory",
        adapter: makeMemoryAdapter(manyEntries(500)),
        count: 500,
        defaultSelected: "m0",
      }}
    />
  ),
};

/** An open store with nothing approved in it yet. */
export const EmptyStore: Story = {
  render: () => (
    <MemoriesPage
      store={{ name: "Project Content Memory", adapter: makeMemoryAdapter([]), count: 0 }}
    />
  ),
};

/** No store open: the named stores, one row each. */
export const WithResources: Story = {
  render: () => (
    <MemoriesPage
      resources={[
        {
          name: "my-project",
          path: "/stores/memory/my-project.db",
          size: 524288,
          modified: "2026-10-14T10:00:00Z",
        },
        {
          name: "shared-corpus",
          path: "/stores/memory/shared-corpus.db",
          size: 2097152,
          modified: "2026-10-09T10:00:00Z",
        },
      ]}
    />
  ),
};

/** No named stores to open. */
export const Empty: Story = {
  render: () => <MemoriesPage resources={[]} />,
};

/** Loading the named stores. */
export const Loading: Story = {
  render: () => <MemoriesPage resources={[]} forceLoading />,
};
