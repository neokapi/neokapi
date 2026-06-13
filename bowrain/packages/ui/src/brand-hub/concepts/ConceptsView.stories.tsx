import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConceptsView } from "./ConceptsView";
import { withBrandHub } from "../../stories/brandHubFixtures";

const meta: Meta<typeof ConceptsView> = {
  title: "Brand Hub/Concepts/ConceptsView",
  component: ConceptsView,
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
type Story = StoryObj<typeof ConceptsView>;

export const Default: Story = {
  args: { onOpenConcept: fn() },
};
