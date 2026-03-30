import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { FlowPage } from "../components/FlowPage";

const meta: Meta<typeof FlowPage> = {
  title: "Pages/FlowPage",
  component: FlowPage,
  tags: ["autodocs"],
  args: {
    onUpdate: fn(),
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
    project: {
      version: "v1",
      name: "Test",
      flows: {
        translate: {
          steps: [{ tool: "ai-translate", config: { provider: "anthropic" } }],
        },
        "translate-and-qa": {
          steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
        },
        pseudo: {
          steps: [{ tool: "pseudo-translate", config: { expansion_rate: 1.3 } }],
        },
      },
    },
  },
};

export const Empty: Story = {
  args: {
    project: {
      version: "v1",
      name: "Empty",
      flows: {},
    },
  },
};
