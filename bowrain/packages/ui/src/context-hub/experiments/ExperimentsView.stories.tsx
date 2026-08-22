import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { fn } from "storybook/test";
import { ExperimentsView } from "./ExperimentsView";
import { withContextHub, voiceHubOverrides } from "../../stories/voiceHubFixtures";
import { createProvidersDecorator } from "../../stories/decorators";

const pad: Decorator = (Story) => (
  <div style={{ padding: 24 }}>
    <Story />
  </div>
);

const meta: Meta<typeof ExperimentsView> = {
  title: "Context/Experiments/ExperimentsView",
  component: ExperimentsView,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
  args: { onOpenExperiment: fn() },
};

export default meta;
type Story = StoryObj<typeof ExperimentsView>;

/** Change-sets grouped by lifecycle status. */
export const Default: Story = {
  decorators: [withContextHub, pad],
};

/** Empty workspace — invites composing the first experiment. */
export const Empty: Story = {
  decorators: [
    createProvidersDecorator(undefined, { ...voiceHubOverrides, listChangesets: async () => [] }),
    pad,
  ],
};
