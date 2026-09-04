import type { Meta, StoryObj } from "@storybook/react-vite";
import { FlowDiagramView } from "../FlowDiagramView";
import type { FlowSpec } from "../types";
import { tools, transformerAwareTools, runReviewTrace } from "./fixtures";

const LINEAR: FlowSpec = {
  steps: [{ tool: "recycle" }, { tool: "translate" }, { tool: "qa" }, { tool: "word-count" }],
};

const FAN_OUT: FlowSpec = {
  steps: [
    { tool: "translate" },
    { tool: "", parallel: [{ tool: "qa" }, { tool: "word-count" }, { tool: "term-check" }] },
    { tool: "pseudo-translate" },
  ],
};

// unredact needs the vault redact writes; with nothing upstream producing it,
// the node carries a "needs redaction.secret" warning.
const UNMET: FlowSpec = {
  steps: [{ tool: "unredact" }, { tool: "translate" }],
};

const meta: Meta<typeof FlowDiagramView> = {
  title: "Flow Editor/FlowDiagramView",
  component: FlowDiagramView,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The read-only diagram of a flow. Authoring happens in the linear step editor; the canvas draws the same steps with per-branch fan-out, the typed IO contract and its diagnostics on every node, and, with a recorded run loaded, the replay.",
      },
    },
  },
  args: { tools },
  decorators: [
    (Story) => (
      <div style={{ height: 640 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FlowDiagramView>;

export const Linear: Story = {
  args: { flow: LINEAR },
};

export const ParallelFanOut: Story = {
  name: "Parallel fan-out",
  args: { flow: FAN_OUT },
};

export const UnmetInput: Story = {
  name: "Unmet input diagnostics",
  args: { flow: UNMET, tools: transformerAwareTools },
};

export const WithRun: Story = {
  name: "With a recorded run",
  args: {
    flow: {
      steps: [{ tool: "redact", config: { detectors: ["entities"] } }, { tool: "translate" }],
    },
    tools: transformerAwareTools,
    trace: runReviewTrace,
  },
};

export const Empty: Story = {
  args: { flow: { steps: [] } },
};

export const LinearDark: Story = {
  name: "Linear (dark)",
  globals: { theme: "dark" },
  args: { flow: LINEAR },
};

export const ParallelFanOutDark: Story = {
  name: "Parallel fan-out (dark)",
  globals: { theme: "dark" },
  args: { flow: FAN_OUT },
};

export const WithRunDark: Story = {
  name: "With a recorded run (dark)",
  globals: { theme: "dark" },
  args: {
    flow: {
      steps: [{ tool: "redact", config: { detectors: ["entities"] } }, { tool: "translate" }],
    },
    tools: transformerAwareTools,
    trace: runReviewTrace,
  },
};
