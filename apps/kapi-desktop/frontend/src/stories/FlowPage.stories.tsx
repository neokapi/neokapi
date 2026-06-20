import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { FlowPage } from "../components/FlowPage";

const meta: Meta<typeof FlowPage> = {
  title: "Pages/FlowPage",
  component: FlowPage,
  tags: ["autodocs"],
  args: {
    onChange: fn(),
    onRun: fn(),
  },
  parameters: {
    layout: "fullscreen",
  },
  decorators: [
    (Story) => (
      <div style={{ height: 600 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FlowPage>;

export const WithFlows: Story = {
  args: {
    flowName: "translate",
    flow: {
      steps: [{ tool: "translate", config: { provider: "anthropic" } }],
    },
  },
};

export const Empty: Story = {
  args: {
    flowName: "new-flow",
    flow: {
      steps: [],
    },
  },
};
