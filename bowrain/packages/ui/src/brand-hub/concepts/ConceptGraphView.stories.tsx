import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConceptGraphView } from "./ConceptGraphView";
import { createProvidersDecorator } from "../../stories/decorators";
import { brandHubOverrides } from "../../stories/brandHubFixtures";
import { richGraph, richMarkets, richConcepts, conceptById } from "./graphSample";

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
