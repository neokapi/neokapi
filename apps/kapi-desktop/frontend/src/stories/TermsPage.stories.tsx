import type { Meta, StoryObj } from "@storybook/react-vite";
import { TermsPage } from "../components/TermsPage";
import { SAMPLE_CONCEPTS, manyConcepts, storyConceptSource } from "./fixtures/concepts";

/**
 * Terms: a store's concepts as a list beside a detail pane, with the search at
 * the top of the list. With no store open, the named stores are listed one row
 * each.
 */
const meta: Meta<typeof TermsPage> = {
  title: "Pages/TermsPage",
  component: TermsPage,
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
type Story = StoryObj<typeof TermsPage>;

/** The project's terms, open, with nothing selected yet. */
export const OpenStore: Story = {
  render: () => (
    <TermsPage
      store={{ name: "Project Terms", source: storyConceptSource(), count: SAMPLE_CONCEPTS.length }}
    />
  ),
};

/** A concept selected: its terms by language, what it replaces, where it came from. */
export const ConceptSelected: Story = {
  render: () => (
    <TermsPage
      store={{
        name: "Project Terms",
        source: storyConceptSource(),
        count: SAMPLE_CONCEPTS.length,
        defaultSelected: "c1",
      }}
    />
  ),
};

/** The same in the dark theme. */
export const ConceptSelectedDark: Story = {
  ...ConceptSelected,
  globals: { theme: "dark" },
};

/** A store of a thousand concepts: the list reads a page at a time. */
export const ManyConcepts: Story = {
  render: () => (
    <TermsPage
      store={{
        name: "Project Terms",
        source: storyConceptSource(manyConcepts(1000)),
        count: 1000,
        defaultSelected: "m0",
      }}
    />
  ),
};

/** An open store with nothing in it yet. */
export const EmptyStore: Story = {
  render: () => (
    <TermsPage store={{ name: "Project Terms", source: storyConceptSource([]), count: 0 }} />
  ),
};

/** No store open: the named stores, one row each. */
export const WithResources: Story = {
  render: () => (
    <TermsPage
      resources={[
        {
          name: "my-terms",
          path: "/stores/terms/my-terms.db",
          size: 262144,
          modified: "2026-10-14T10:00:00Z",
        },
        {
          name: "voice-terms",
          path: "/stores/terms/voice-terms.db",
          size: 131072,
          modified: "2026-10-12T10:00:00Z",
        },
      ]}
    />
  ),
};

/** The store list in the dark theme. */
export const WithResourcesDark: Story = { ...WithResources, globals: { theme: "dark" } };

/** No named stores to open. */
export const Empty: Story = {
  render: () => <TermsPage resources={[]} />,
};

/** Loading the named stores. */
export const Loading: Story = {
  render: () => <TermsPage resources={[]} forceLoading />,
};
