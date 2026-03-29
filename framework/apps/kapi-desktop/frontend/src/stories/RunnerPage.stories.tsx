import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { RunnerPage } from "../components/RunnerPage";

const meta: Meta<typeof RunnerPage> = {
  title: "Pages/RunnerPage",
  component: RunnerPage,
  tags: ["autodocs"],
  args: {
    onClose: fn(),
  },
};

export default meta;
type Story = StoryObj<typeof RunnerPage>;

export const SingleStep: Story = {
  args: {
    flowName: "translate",
    flow: {
      steps: [{ tool: "ai-translate" }],
    },
  },
};

export const MultiStep: Story = {
  args: {
    flowName: "translate-and-qa",
    flow: {
      steps: [
        { tool: "ai-translate", config: { provider: "anthropic" } },
        { tool: "qa-check" },
      ],
    },
  },
};

export const ThreeSteps: Story = {
  args: {
    flowName: "full-pipeline",
    flow: {
      steps: [
        { tool: "ai-translate" },
        { tool: "qa-check" },
        { tool: "pseudo-translate" },
      ],
    },
  },
};
