import type { ComponentProps } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { StepRow, useStepReorder } from "../../components/flow-editor";
import type { FlowTool } from "../../components/flow-editor";
import type { ComponentSchema } from "../../components/schema-form";

const TRANSLATE: FlowTool = {
  name: "translate",
  display_name: "Translate",
  description: "Translate with an AI provider.",
  has_schema: true,
};

const SCHEMA: ComponentSchema = {
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

/** A row in a three-step flow, so its grip drags and answers the arrow keys. */
function Reorderable(props: ComponentProps<typeof StepRow>) {
  const reorder = useStepReorder(3, fn());
  return (
    <ul className="max-w-xl">
      <StepRow {...props} reorder={props.readOnly ? undefined : reorder(props.index)} />
    </ul>
  );
}

const meta: Meta<typeof StepRow> = {
  title: "Flow editor/StepRow",
  component: StepRow,
  tags: ["autodocs"],
  args: { onRemove: fn(), onConfigChange: fn() },
  render: (args) => <Reorderable {...args} />,
};

export default meta;
type Story = StoryObj<typeof StepRow>;

export const Collapsed: Story = {
  args: { step: { tool: "translate" }, tool: TRANSLATE, index: 1, schema: SCHEMA },
};

/** The options area open, rendering the tool's own schema form inline. */
export const OptionsExpanded: Story = {
  args: {
    step: { tool: "translate", config: { provider: "anthropic" } },
    tool: TRANSLATE,
    index: 1,
    schema: SCHEMA,
    defaultOpen: true,
  },
};

export const ReadOnly: Story = {
  args: {
    step: { tool: "translate" },
    tool: TRANSLATE,
    index: 1,
    schema: SCHEMA,
    readOnly: true,
  },
};
