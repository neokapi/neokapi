import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { FlowCardItem } from "./FlowCard";
import { FlowCard } from "./FlowCard";

const meta: Meta<typeof FlowCard> = {
  title: "Flows/FlowCard",
  component: FlowCard,
  tags: ["autodocs"],
  render: (args) => (
    <div className="max-w-md">
      <FlowCard {...args} />
    </div>
  ),
};

export default meta;
type Story = StoryObj<typeof FlowCard>;

const base = {
  onClick: fn(),
};

const shipFlow: FlowCardItem = {
  id: "ship",
  name: "Ship translations",
  description: "Translate with guardrails: memory reuse, AI translate, then deterministic checks.",
  steps: ["Memory Reuse", "Translate", "Quality Check"],
  stepCount: 3,
  source: "project",
  isDefault: true,
};

export const DefaultFlow: Story = {
  name: "Default flow",
  args: { ...base, item: shipFlow },
};

export const MultiStep: Story = {
  name: "Multi-step (built-in)",
  args: {
    ...base,
    item: {
      id: "translate-qa",
      name: "Translate and check",
      description: "Translate content then run an LLM-judged quality check.",
      steps: ["Translate", "Quality Check"],
      stepCount: 2,
      source: "built-in",
    },
    onCopy: fn(),
  },
};

export const SingleStep: Story = {
  name: "Single-step",
  args: {
    ...base,
    item: {
      id: "pseudo",
      name: "Pseudo translate",
      description: "Generate pseudo-translations for testing.",
      steps: ["Pseudo Translate"],
      stepCount: 1,
      source: "built-in",
    },
    onCopy: fn(),
  },
};

export const NoSteps: Story = {
  name: "No steps yet",
  args: {
    ...base,
    item: {
      id: "empty",
      name: "empty-flow",
      steps: [],
      stepCount: 0,
      source: "user",
    },
    onDelete: fn(),
  },
};

export const Loading: Story = {
  name: "Loading",
  args: { loading: true },
};
