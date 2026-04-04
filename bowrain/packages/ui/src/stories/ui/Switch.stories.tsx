import { Label, Switch } from "@neokapi/ui-primitives";
import type { Meta, StoryObj } from "@storybook/react-vite";

const meta: Meta<typeof Switch> = {
  title: "Foundations/Switch",
  component: Switch,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof Switch>;

export const Default: Story = {};

export const Checked: Story = {
  args: { defaultChecked: true },
};

export const WithLabel: Story = {
  render: () => (
    <div className="flex items-center gap-2">
      <Switch id="auto-translate" />
      <Label htmlFor="auto-translate">Auto-translate on save</Label>
    </div>
  ),
};

export const Disabled: Story = {
  args: { disabled: true },
};
