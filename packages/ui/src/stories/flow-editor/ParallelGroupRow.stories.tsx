import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ParallelGroupRow } from "../../components/flow-editor";
import type { FlowTool } from "../../components/flow-editor";

const TOOLS: FlowTool[] = [
  { name: "qa", display_name: "Quality Check", description: "Check the result against the rules." },
  {
    name: "voice-vocab-check",
    display_name: "Voice check",
    description: "Hold the wording to the voice profile.",
  },
  { name: "word-count", display_name: "Word count", description: "Count words and segments." },
];

const meta: Meta<typeof ParallelGroupRow> = {
  title: "Flow editor/ParallelGroupRow",
  component: ParallelGroupRow,
  tags: ["autodocs"],
  args: {
    index: 0,
    count: 1,
    tools: TOOLS,
    onChange: fn(),
    onRemove: fn(),
    onMoveUp: fn(),
    onMoveDown: fn(),
  },
  render: (args) => (
    <ul className="max-w-2xl">
      <ParallelGroupRow {...args} />
    </ul>
  ),
};

export default meta;
type Story = StoryObj<typeof ParallelGroupRow>;

export const TwoBranches: Story = {
  args: {
    step: { tool: "", parallel: [{ tool: "qa" }, { tool: "voice-vocab-check" }] },
  },
};

export const OneBranch: Story = {
  args: {
    step: { tool: "", parallel: [{ tool: "qa" }] },
  },
};

export const Empty: Story = {
  args: {
    step: { tool: "", parallel: [] },
  },
};

export const ReadOnly: Story = {
  args: {
    step: { tool: "", parallel: [{ tool: "qa" }, { tool: "voice-vocab-check" }] },
    readOnly: true,
  },
};
