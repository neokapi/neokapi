import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AddStepPicker } from "../../components/flow-editor";
import type { FlowTool } from "../../components/flow-editor";

const TOOLS: FlowTool[] = [
  {
    name: "recycle",
    display_name: "Recycle",
    description: "Reuse approved wording from content memory.",
  },
  { name: "translate", display_name: "Translate", description: "Translate with an AI provider." },
  { name: "qa", display_name: "Quality Check", description: "Check the result against the rules." },
  {
    name: "voice-vocab-check",
    display_name: "Voice check",
    description: "Hold the wording to the voice profile.",
  },
  { name: "word-count", display_name: "Word count", description: "Count words and segments." },
];

const meta: Meta<typeof AddStepPicker> = {
  title: "Flow editor/AddStepPicker",
  component: AddStepPicker,
  tags: ["autodocs"],
  args: { tools: TOOLS, onAdd: fn() },
};

export default meta;
type Story = StoryObj<typeof AddStepPicker>;

/** The trigger button in a flow. Click it to open the tool list. */
export const Trigger: Story = {};

/** The dialog open, showing the searchable tool list. */
export const Open: Story = {
  args: { defaultOpen: true },
};
