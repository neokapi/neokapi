import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { useState } from "react";
import { CoordinatesEditor, type CoordinateAxisOption } from "../../components/governance";

const recipeAxes: CoordinateAxisOption[] = [
  {
    axis: "product",
    declarable: false,
    refusal:
      "recipe: \"product\" is derived from a collection's channel, not declared: remove it from the point or set the collection's channel instead",
  },
  {
    axis: "channel",
    declarable: false,
    refusal:
      "recipe: \"channel\" is derived from a collection's channel, not declared: remove it from the point or set the collection's channel instead",
  },
  { axis: "brand" },
  { axis: "mode", values: ["tutorial", "how-to", "reference", "explanation"] },
];

const meta: Meta<typeof CoordinatesEditor> = {
  title: "Governance/CoordinatesEditor",
  component: CoordinatesEditor,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "A point, or a region of one, as rows of axis and value. The consumer passes the axes it knows: which may be declared, which are refused and why, and which carry a closed value set. A recipe may mint an axis (free text); a collection or a membership picks from the vocabulary.",
      },
    },
  },
  args: { onChange: fn() },
  render: function Render(args) {
    const [value, setValue] = useState<Record<string, string>>(args.value);
    return (
      <div className="max-w-lg">
        <CoordinatesEditor
          {...args}
          value={value}
          onChange={(next) => {
            setValue(next ?? {});
            args.onChange(next);
          }}
        />
      </div>
    );
  },
};

export default meta;
type Story = StoryObj<typeof CoordinatesEditor>;

export const Empty: Story = {
  name: "Empty (the project's own point)",
  args: {
    value: {},
    axes: recipeAxes,
    allowNewAxis: true,
    label: "Default point",
    emptyText: "Every collection sits at the project's own point.",
    note: "product and channel come from a collection's channel.",
  },
};

export const Populated: Story = {
  name: "A declared point with a closed value set",
  args: {
    value: { brand: "northsea", mode: "reference" },
    axes: recipeAxes,
    allowNewAxis: true,
    label: "Default point",
    note: "product and channel come from a collection's channel.",
  },
};

export const DeclarableOnly: Story = {
  name: "Picks from the declared axes (collection)",
  args: {
    value: { brand: "northsea" },
    axes: recipeAxes,
    label: "Coordinates here",
    emptyText: "Inherits the project's declared axes.",
  },
};

export const Validation: Story = {
  name: "A region refusing a blank value (membership)",
  args: {
    value: { brand: "acme", channel: "" },
    allowNewAxis: true,
    requireValues: true,
    label: "Governs",
    emptyText: "The whole space.",
  },
};

export const Disabled: Story = {
  args: {
    value: { brand: "northsea", mode: "tutorial" },
    axes: recipeAxes,
    allowNewAxis: true,
    label: "Default point",
    disabled: true,
  },
};

export const Dark: Story = {
  globals: { theme: "dark" },
  args: {
    value: { brand: "northsea", mode: "reference" },
    axes: recipeAxes,
    allowNewAxis: true,
    label: "Default point",
    note: "product and channel come from a collection's channel.",
  },
};
