import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConceptStoryView } from "./ConceptStoryView";
import { withBrandHub, brandHubOverrides } from "../../stories/brandHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";
import {
  conceptById,
  richMarkets,
  richStory,
  richRelations,
  richObservations,
  richComments,
} from "./graphSample";

const meta: Meta<typeof ConceptStoryView> = {
  title: "Brand Hub/Concepts/ConceptStoryView",
  component: ConceptStoryView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  decorators: [
    withBrandHub,
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ConceptStoryView>;

export const Default: Story = {
  args: { conceptId: "c-checkout", onBack: fn(), onOpenConcept: fn() },
};

// A fully populated story: terms across three markets, a typed relations
// explorer, observations, a multi-day timeline, and a threaded discussion.
const fullDecorator = createProvidersDecorator(undefined, {
  ...brandHubOverrides,
  getConcept: async (_ws: string, id: string) => conceptById(id),
  listMarkets: async () => richMarkets,
  getConceptStory: async () => ({ concept_id: "c-checkout", entries: richStory }),
  listConceptRelations: async () => richRelations,
  listObservations: async () => richObservations,
  listConceptComments: async () => richComments,
  listConcepts: async () => ({
    concepts: [conceptById("c-commerce"), conceptById("c-payment"), conceptById("c-basket")],
    total_count: 3,
  }),
});

export const FullTimeline: Story = {
  args: { conceptId: "c-checkout", onBack: fn(), onOpenConcept: fn() },
  decorators: [fullDecorator],
};
