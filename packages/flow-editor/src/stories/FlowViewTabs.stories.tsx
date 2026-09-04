import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn } from "storybook/test";
import { FlowViewTabs, type FlowViewTabsProps } from "../FlowViewTabs";
import { FlowTemplateLibrary } from "../FlowTemplateLibrary";
import type { ComponentSchema, FlowSpec } from "../types";
import { tools, transformerAwareTools, runReviewTrace } from "./fixtures";

const TRANSLATE_SCHEMA: ComponentSchema = {
  title: "Translate options",
  type: "object",
  properties: {
    provider: {
      type: "string",
      title: "Provider",
      description: "Which provider handles the translation.",
      options: [
        { value: "openai", label: "OpenAI" },
        { value: "anthropic", label: "Anthropic" },
      ],
    },
    temperature: { type: "number", title: "Temperature", minimum: 0, maximum: 1, default: 0.2 },
  },
};

const getSchema = (name: string) => (name === "translate" ? TRANSLATE_SCHEMA : null);

const FLOW: FlowSpec = {
  description: "Reuse approved wording, translate the rest, and check the result.",
  steps: [
    { tool: "recycle" },
    { tool: "translate", config: { provider: "anthropic" } },
    { tool: "", parallel: [{ tool: "qa" }, { tool: "word-count" }] },
  ],
};

/** Keeps the flow in state so edits on the Steps view show up on the Diagram. */
function Stateful(props: FlowViewTabsProps) {
  const [flow, setFlow] = useState(props.flow);
  return (
    <FlowViewTabs
      {...props}
      flow={flow}
      onChange={(next) => {
        setFlow(next);
        props.onChange(next);
      }}
    />
  );
}

const meta: Meta<typeof FlowViewTabs> = {
  title: "Flow Editor/FlowViewTabs",
  component: FlowViewTabs,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The flow workbench: Steps (the shared linear step editor, where authoring happens), Diagram (the same steps as a read-only canvas with the IO diagnostics) and, once a run of the flow is loaded, Run (the replay). Both kapi desktop and the platform render this composition.",
      },
    },
  },
  args: {
    flowName: "translate-and-check",
    flow: FLOW,
    tools,
    onChange: fn(),
    onGetSchema: getSchema,
    onRun: fn(),
    isDefault: true,
    onToggleDefault: fn(),
    onRename: fn(),
    templateLibrary: <FlowTemplateLibrary onSelect={fn()} />,
  },
  render: (args) => (
    <div style={{ height: 640 }}>
      <Stateful {...args} />
    </div>
  ),
};

export default meta;
type Story = StoryObj<typeof FlowViewTabs>;

export const Steps: Story = {};

export const Diagram: Story = {
  args: { defaultView: "diagram" },
};

export const WithRun: Story = {
  name: "With a recorded run",
  args: {
    flowName: "secure-translate",
    flow: {
      steps: [{ tool: "redact", config: { detectors: ["entities"] } }, { tool: "translate" }],
    },
    tools: transformerAwareTools,
    trace: runReviewTrace,
    defaultView: "run",
    isDefault: false,
  },
};

export const ReadOnly: Story = {
  name: "Read-only (built-in flow)",
  args: { readOnly: true, isDefault: false, onToggleDefault: undefined, onRename: undefined },
};

export const EmptyFlow: Story = {
  args: { flowName: "new-flow", flow: { steps: [] }, isDefault: false },
};

export const StepsDark: Story = {
  name: "Steps (dark)",
  globals: { theme: "dark" },
};

export const DiagramDark: Story = {
  name: "Diagram (dark)",
  globals: { theme: "dark" },
  args: { defaultView: "diagram" },
};
