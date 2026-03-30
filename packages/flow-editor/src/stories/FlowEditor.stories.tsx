import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { FlowEditor } from "../FlowEditor";

const SAMPLE_TOOLS = [
  { name: "ai-translate", description: "Translate with AI", category: "translation" },
  { name: "pseudo-translate", description: "Pseudo-translate for testing", category: "translation" },
  { name: "qa-check", description: "Run quality checks", category: "quality" },
  { name: "ai-qa", description: "AI-powered QA", category: "quality" },
  { name: "tm-leverage", description: "Pre-fill from TM", category: "translation" },
];

const meta: Meta<typeof FlowEditor> = {
  title: "Flow Editor/FlowEditor",
  component: FlowEditor,
  tags: ["autodocs"],
  args: {
    flow: { steps: [{ tool: "ai-translate" }, { tool: "qa-check" }] },
    tools: SAMPLE_TOOLS,
    onChange: fn(),
    onRun: fn(),
  },
  parameters: {
    layout: "fullscreen",
  },
  decorators: [
    (Story) => (
      <div style={{ height: 500 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FlowEditor>;

export const SingleStep: Story = {
  args: {
    flow: {
      steps: [{ tool: "ai-translate", config: { provider: "anthropic" } }],
    },
  },
};

export const MultiStep: Story = {
  args: {
    flow: {
      steps: [
        { tool: "ai-translate", label: "Translate" },
        { tool: "qa-check", label: "QA" },
      ],
    },
  },
};

export const FullPipeline: Story = {
  args: {
    flow: {
      steps: [
        { tool: "tm-leverage" },
        { tool: "ai-translate", config: { provider: "anthropic", model: "claude-sonnet-4-5-20241022" } },
        { tool: "qa-check" },
        { tool: "ai-qa" },
      ],
    },
  },
};

export const ReadOnly: Story = {
  args: {
    flow: {
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
    },
    readOnly: true,
    onRun: undefined,
  },
};

export const Empty: Story = {
  args: {
    flow: { steps: [] },
  },
};
