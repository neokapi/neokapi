import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { LinearFlowEditor } from "../../components/flow-editor";
import type { FlowTool } from "../../components/flow-editor";
import type { ComponentSchema } from "../../components/schema-form";
import { Card } from "../../components/ui/card";

const TOOLS: FlowTool[] = [
  {
    name: "recycle",
    display_name: "Recycle",
    description: "Reuse approved wording from content memory.",
    has_schema: false,
  },
  {
    name: "translate",
    display_name: "Translate",
    description: "Translate with an AI provider.",
    has_schema: true,
  },
  {
    name: "qa",
    display_name: "Quality Check",
    description: "Check the result against the rules.",
    has_schema: false,
  },
  {
    name: "voice-vocab-check",
    display_name: "Voice check",
    description: "Hold the wording to the voice profile.",
    has_schema: false,
  },
  {
    name: "word-count",
    display_name: "Word count",
    description: "Count words and segments.",
    has_schema: false,
  },
];

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

const meta: Meta<typeof LinearFlowEditor> = {
  title: "Flow editor/LinearFlowEditor",
  component: LinearFlowEditor,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The shared linear flow editor: a flow is an ordered pipeline of tool steps, edited as a reorderable list rather than a node canvas. It is surface-agnostic, so kapi and bowrain pass their own flow objects and tool lists.",
      },
    },
  },
  args: { onChange: fn(), tools: TOOLS },
  render: (args) => (
    <div style={{ height: 640 }}>
      <LinearFlowEditor {...args} />
    </div>
  ),
};

export default meta;
type Story = StoryObj<typeof LinearFlowEditor>;

/** A new flow: the template slot plus a blank-start "Add step". */
export const Empty: Story = {
  args: {
    flowName: "new-flow",
    flow: { steps: [] },
    templateLibrary: (
      <Card className="p-4 text-sm text-muted-foreground">
        Template library (provided by the host)
      </Card>
    ),
  },
};

export const SingleStep: Story = {
  args: {
    flowName: "word-count",
    flow: { description: "Count the words in the source.", steps: [{ tool: "word-count" }] },
  },
};

export const MultiStep: Story = {
  args: {
    flowName: "translate-and-qa",
    flow: {
      description: "Recycle, translate with AI, then run the quality checks.",
      steps: [{ tool: "recycle" }, { tool: "translate" }, { tool: "qa" }],
    },
    onGetSchema: getSchema,
    onRun: fn(),
  },
};

/** The Default toggle on, with the name editable and Run wired. */
export const DefaultOn: Story = {
  args: {
    flowName: "translate-and-qa",
    flow: {
      description: "The project's default flow.",
      steps: [{ tool: "recycle" }, { tool: "translate" }, { tool: "qa" }],
    },
    isDefault: true,
    onToggleDefault: fn(),
    onRename: fn(),
    onGetSchema: getSchema,
    onRun: fn(),
  },
};

/** A step whose tool has options: the row gains an Options control to expand the form. */
export const StepWithOptions: Story = {
  args: {
    flowName: "translate-and-qa",
    flow: { steps: [{ tool: "translate", config: { provider: "anthropic" } }, { tool: "qa" }] },
    onGetSchema: getSchema,
  },
};

/** A built-in flow: read-only, no add/remove/reorder. */
export const ReadOnly: Story = {
  args: {
    flowName: "built-in-convert",
    flow: {
      description: "A built-in flow. Copy it to edit.",
      steps: [{ tool: "recycle" }, { tool: "translate" }],
    },
    readOnly: true,
    onRun: fn(),
  },
};
