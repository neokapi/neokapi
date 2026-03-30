import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { FlowTemplateLibrary } from "../FlowTemplateLibrary";

const meta: Meta<typeof FlowTemplateLibrary> = {
  title: "Flow Editor/FlowTemplateLibrary",
  component: FlowTemplateLibrary,
  tags: ["autodocs"],
  args: {
    onSelect: fn(),
    onDismiss: fn(),
  },
  decorators: [
    (Story) => (
      <div style={{ height: 600, overflow: "auto" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FlowTemplateLibrary>;

export const Default: Story = {};

export const WithoutDismiss: Story = {
  args: {
    onDismiss: undefined,
  },
};
