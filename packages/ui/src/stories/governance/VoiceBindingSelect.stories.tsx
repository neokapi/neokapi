import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { useState } from "react";
import { VoiceBindingSelect, type VoiceBindingOption } from "../../components/governance";

const recipeOptions: VoiceBindingOption[] = [
  { value: "file:.kapi/voice.yaml", label: ".kapi/voice.yaml", group: "Files" },
  {
    value: "file:.kapi/profiles/support/voice.yaml",
    label: ".kapi/profiles/support/voice.yaml",
    group: "Files",
  },
  {
    value: "pack:technical-docs",
    label: "technical-docs",
    group: "Starter packs",
    hint: "read-only",
  },
  { value: "pack:friendly-dtc", label: "friendly-dtc", group: "Starter packs", hint: "read-only" },
];

const workspaceOptions: VoiceBindingOption[] = [
  { value: "vp-1", label: "Northsea support" },
  { value: "vp-2", label: "Northsea campaigns" },
];

const meta: Meta<typeof VoiceBindingSelect> = {
  title: "Governance/VoiceBindingSelect",
  component: VoiceBindingSelect,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "The voice profile governing content at a point, as one picker. A recipe offers its files and starter packs; a workspace offers its stored profiles. The inherit row says what governs when nothing is bound here.",
      },
    },
  },
  args: { onChange: fn() },
  render: function Render(args) {
    const [value, setValue] = useState(args.value);
    return (
      <div className="max-w-md">
        <VoiceBindingSelect
          {...args}
          value={value}
          onChange={(next) => {
            setValue(next);
            args.onChange(next);
          }}
        />
      </div>
    );
  },
};

export default meta;
type Story = StoryObj<typeof VoiceBindingSelect>;

export const Empty: Story = {
  name: "Nothing bound (recipe)",
  args: { value: undefined, options: recipeOptions, inheritLabel: "None bound" },
};

export const Populated: Story = {
  name: "A file bound, with packs offered",
  args: {
    value: "file:.kapi/voice.yaml",
    options: recipeOptions,
    inheritLabel: "None bound",
    help: "A file is edited on the Voice page. A starter pack is read-only.",
  },
};

export const WorkspaceProfiles: Story = {
  name: "Workspace profiles (platform)",
  args: {
    value: "vp-2",
    options: workspaceOptions,
    inheritLabel: "Workspace default",
    help: "Governs checks and scoring for this project. Streams and collections can override it.",
  },
};

export const Disabled: Story = {
  args: {
    value: "vp-1",
    options: workspaceOptions,
    inheritLabel: "Workspace default",
    disabled: true,
  },
};

export const NotFound: Story = {
  name: "Bound to a profile no option names",
  args: {
    value: "file:.kapi/removed.yaml",
    options: recipeOptions,
    inheritLabel: "None bound",
    help: "The bound file is gone from the tree; clear or rebind it.",
  },
};

export const Dark: Story = {
  globals: { theme: "dark" },
  args: {
    value: "pack:technical-docs",
    options: recipeOptions,
    inheritLabel: "None bound",
    help: "A file is edited on the Voice page. A starter pack is read-only.",
  },
};
