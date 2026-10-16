import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoriesPage } from "../components/MemoriesPage";
import { SAMPLE_ENTRIES, makeMemoryAdapter, manyEntries } from "./fixtures/memory";

/**
 * Content memory: one row per pair, the source text and one target language,
 * beside a detail pane with both sides in full and where the entry came from.
 * With no store open, the named stores are listed one row each.
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

/** A pair selected: inline formatting as chips, its other language, its provenance and unit. */
export const PairSelected: Story = {
  render: () => (
    <MemoriesPage
      store={{
        name: "Project Content Memory",
        adapter: makeMemoryAdapter(),
        count: SAMPLE_ENTRIES.length,
        defaultSelected: "2::fr-FR",
      }}
    />
  ),
};

/** The same in the dark theme. */
export const PairSelectedDark: Story = { ...PairSelected, globals: { theme: "dark" } };

/** An entry held in three languages, selected on one of its two targets. */
export const ThreeLanguages: Story = {
  render: () => (
    <MemoriesPage
      store={{
        name: "Project Content Memory",
        adapter: makeMemoryAdapter(),
        count: SAMPLE_ENTRIES.length,
        defaultSelected: "1::de-DE",
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
        defaultSelected: "m0::fr-FR",
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
