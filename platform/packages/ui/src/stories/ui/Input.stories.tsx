import type { Meta, StoryObj } from "@storybook/react-vite";
import { Input } from "../../components/ui/input";
import { Label } from "../../components/ui/label";

const meta: Meta<typeof Input> = {
  title: "Foundations/Input",
  component: Input,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 320, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Input>;

export const Default: Story = {
  args: { placeholder: "Enter text..." },
};

export const WithLabel: Story = {
  render: () => (
    <div className="grid gap-2">
      <Label htmlFor="source-locale">Source Locale</Label>
      <Input id="source-locale" placeholder="en-US" />
    </div>
  ),
};

export const Disabled: Story = {
  args: { placeholder: "Disabled", disabled: true },
};

export const WithValue: Story = {
  args: { defaultValue: "messages.json" },
};
