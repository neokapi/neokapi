import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { StepRow } from "../../components/flow-editor";
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

const meta: Meta<typeof StepRow> = {
  title: "Flow editor/StepRow",
  component: StepRow,
  tags: ["autodocs"],
  args: { onRemove: fn(), onMoveUp: fn(), onMoveDown: fn(), onConfigChange: fn() },
  render: (args) => (
    <ul className="max-w-xl">
      <StepRow {...args} />
    </ul>
  ),
};

export default meta;
type Story = StoryObj<typeof StepRow>;

export const Collapsed: Story = {
  args: { step: { tool: "translate" }, tool: TRANSLATE, index: 1, count: 3, schema: SCHEMA },
};

/** The options area open, rendering the tool's own schema form inline. */
export const OptionsExpanded: Story = {
  args: {
    step: { tool: "translate", config: { provider: "anthropic" } },
    tool: TRANSLATE,
    index: 1,
    count: 3,
    schema: SCHEMA,
    defaultOpen: true,
  },
};

export const FirstStep: Story = {
  args: { step: { tool: "translate" }, tool: TRANSLATE, index: 0, count: 3, schema: SCHEMA },
};

export const ReadOnly: Story = {
  args: {
    step: { tool: "translate" },
    tool: TRANSLATE,
    index: 1,
    count: 3,
    schema: SCHEMA,
    readOnly: true,
  },
};
