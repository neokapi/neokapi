import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConceptGraphView } from "./ConceptGraphView";
import { createProvidersDecorator } from "../../stories/decorators";
import { brandHubOverrides } from "../../stories/brandHubFixtures";
import { richGraph, largeGraph, richMarkets, richConcepts, conceptById } from "./graphSample";

const richDecorator = createProvidersDecorator(undefined, {
  ...brandHubOverrides,
  getGraph: async () => richGraph,
  listMarkets: async () => richMarkets,
  getConcept: async (_ws: string, id: string) => conceptById(id),
  listConcepts: async () => ({ concepts: richConcepts, total_count: richConcepts.length }),
});

const meta: Meta<typeof ConceptGraphView> = {
  title: "Brand Hub/Concepts/ConceptGraphView",
  component: ConceptGraphView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  decorators: [
    richDecorator,
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ConceptGraphView>;

export const Default: Story = {
  args: { onOpenConcept: fn() },
};

// The graph fetch fails: the canvas shows a distinct error banner with a retry
// affordance instead of silently collapsing into the empty state.
const errorDecorator = createProvidersDecorator(undefined, {
  ...brandHubOverrides,
  getGraph: async () => {
    throw new Error("The brand graph service is unavailable (503).");
  },
  listMarkets: async () => richMarkets,
});

export const LoadError: Story = {
  name: "Load error",
  args: { onOpenConcept: fn() },
  decorators: [errorDecorator],
};

// The workspace has outgrown a single canvas: the server caps the payload and
// flags it truncated, so the wide-open view shows the focus-or-filter guard
// instead of a hairball. Choosing a concept (or setting a filter) renders the
// graph normally.
const largeDecorator = createProvidersDecorator(undefined, {
  ...brandHubOverrides,
  getGraph: async () => largeGraph,
  listMarkets: async () => richMarkets,
  getConcept: async (_ws: string, id: string) => conceptById(id),
  listConcepts: async () => ({ concepts: richConcepts, total_count: richConcepts.length }),
});

export const LargeTruncated: Story = {
  name: "Large / truncated (scale guard)",
  args: { onOpenConcept: fn() },
  decorators: [largeDecorator],
};
