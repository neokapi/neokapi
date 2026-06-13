import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { GraphPanel } from "./GraphPanel";
import { sampleGraph } from "../../stories/brandHubFixtures";
import { richGraph } from "./graphSample";

const meta: Meta<typeof GraphPanel> = {
  title: "Brand Hub/Concepts/GraphPanel",
  component: GraphPanel,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ height: 520, width: 820, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof GraphPanel>;

export const Default: Story = {
  args: { graph: sampleGraph, onSelectNode: fn() },
};

export const Focused: Story = {
  args: { graph: sampleGraph, focusId: "c-checkout", onSelectNode: fn() },
};

export const RichHierarchy: Story = {
  args: { graph: richGraph, onSelectNode: fn() },
};

export const RichFocused: Story = {
  args: { graph: richGraph, focusId: "c-checkout", onSelectNode: fn() },
};
