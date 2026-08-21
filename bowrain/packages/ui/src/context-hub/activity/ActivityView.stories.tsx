import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { Decorator } from "@storybook/react";
import { ActivityView } from "./ActivityView";
import { brandHubOverrides } from "../../stories/brandHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";

const populated: Decorator = createProvidersDecorator(undefined, brandHubOverrides);

// A feed with nothing in it, and no names to resolve — what a workspace looks
// like before its first governed change.
const empty: Decorator = createProvidersDecorator(undefined, {
  ...brandHubOverrides,
  listActivities: async () => ({ activities: [], next_cursor: "" }),
  listChangesets: async () => [],
  listConcepts: async () => ({ concepts: [], total_count: 0 }),
});

const meta: Meta<typeof ActivityView> = {
  title: "Context/Activity/ActivityView",
  component: ActivityView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { onOpenConcept: fn(), onOpenExperiment: fn() },
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ActivityView>;

export const Default: Story = {
  decorators: [populated],
};

export const Empty: Story = {
  decorators: [empty],
};
